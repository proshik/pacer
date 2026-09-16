// Headless Chrome check of Telegram start parameters: a t.me link with
// ?startapp=d<metres>_t<seconds> opens the page on that plan.
import { spawn } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const ROOT = fileURLToPath(new URL("../../", import.meta.url));
const ASSETS = process.env.ASSETS || join(ROOT, "assets");
const CHROME = process.env.CHROME || "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
const HTTP_PORT = 8819;
const CDP_PORT = 9320;
const BASE = `http://127.0.0.1:${HTTP_PORT}`;

const profile = mkdtempSync(join(tmpdir(), "pacer-checks-"));
const server = spawn("python3", ["-m", "http.server", String(HTTP_PORT), "-d", ASSETS, "--bind", "127.0.0.1"], { stdio: "ignore" });
const chrome = spawn(CHROME, [
    "--headless=new", `--remote-debugging-port=${CDP_PORT}`, "--use-mock-keychain", "--password-store=basic",
    `--user-data-dir=${profile}`, "--no-first-run", "--no-default-browser-check", "--disable-extensions", "about:blank"
], { stdio: "ignore" });
function cleanup() {
    try { chrome.kill("SIGKILL"); } catch {}
    try { server.kill("SIGKILL"); } catch {}
    try { rmSync(profile, { recursive: true, force: true }); } catch {}
}
const hardStop = setTimeout(() => { console.error("HARD TIMEOUT"); cleanup(); process.exit(3); }, 120000);
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
async function waitFor(fn, timeoutMs, what) {
    const deadline = Date.now() + timeoutMs;
    let last;
    while (Date.now() < deadline) {
        try { last = await fn(); if (last) return last; } catch (e) { last = e; }
        await sleep(100);
    }
    throw new Error(`timed out waiting for ${what}: ${last}`);
}
const results = [];
function check(name, ok, detail) {
    results.push({ ok: Boolean(ok) });
    console.log(`${ok ? "PASS" : "FAIL"} ${name}${ok ? "" : " — " + JSON.stringify(detail)}`);
}

await waitFor(async () => (await fetch(`http://127.0.0.1:${CDP_PORT}/json/version`)).ok, 15000, "chrome");
await waitFor(async () => (await fetch(`${BASE}/index.html`)).ok, 15000, "http server");
const version = await (await fetch(`http://127.0.0.1:${CDP_PORT}/json/version`)).json();
const targets = await (await fetch(`http://127.0.0.1:${CDP_PORT}/json/list`)).json();
const ws = new WebSocket(targets.find((t) => t.type === "page").webSocketDebuggerUrl);
await new Promise((resolve, reject) => { ws.onopen = resolve; ws.onerror = reject; });
let nextId = 0;
const pending = new Map();
const listeners = [];
ws.onmessage = (event) => {
    const msg = JSON.parse(event.data);
    if (msg.id) {
        const p = pending.get(msg.id);
        pending.delete(msg.id);
        msg.error ? p.reject(new Error(JSON.stringify(msg.error))) : p.resolve(msg.result);
    } else {
        listeners.forEach((l) => l(msg));
    }
};
const send = (method, params = {}) => new Promise((resolve, reject) => {
    const id = ++nextId;
    pending.set(id, { resolve, reject });
    ws.send(JSON.stringify({ id, method, params }));
});
async function evaluate(expression) {
    const { result, exceptionDetails } = await send("Runtime.evaluate", { expression, returnByValue: true, awaitPromise: true });
    if (exceptionDetails) throw new Error(exceptionDetails.exception?.description || exceptionDetails.text);
    return result.value;
}
let consoleProblems = [];
listeners.push((msg) => {
    if (msg.method === "Runtime.consoleAPICalled" && ["error", "warning", "assert"].includes(msg.params.type)) {
        consoleProblems.push(msg.params.args.map((a) => a.value ?? a.description).join(" "));
    }
    if (msg.method === "Runtime.exceptionThrown") {
        consoleProblems.push(msg.params.exceptionDetails.exception?.description || msg.params.exceptionDetails.text);
    }
});

await send("Runtime.enable");
await send("Page.enable");
await send("Emulation.setUserAgentOverride", { userAgent: version["User-Agent"], acceptLanguage: "ru-RU,ru" });
await send("Emulation.setDeviceMetricsOverride", { width: 390, height: 844, deviceScaleFactor: 2, mobile: true });

const STATE = `(() => ({
    distance: document.getElementById("distInput").value,
    time: ["timeHourInput", "timeMinuteInput", "timeSecondInput"].map((id) => document.getElementById(id).value).join(":"),
    pace: ["paceMinuteInput", "paceSecondInput"].map((id) => document.getElementById(id).value).join(":"),
    plan: document.getElementById("planLine").textContent,
    search: location.search
}))()`;

async function open(path) {
    consoleProblems = [];
    await send("Page.navigate", { url: `${BASE}${path}` });
    await waitFor(() => evaluate(`document.body.dataset.state === "ready"`), 20000, `ready at ${path}`);
    await sleep(300);
    return evaluate(STATE);
}

try {
    const plain = await open("/");

    // A link such as t.me/PacerGoBot?startapp=d42195_t12600 reaches the page as
    // tgWebAppStartParam: the distance in metres and the time in seconds.
    let s = await open("/?tgWebAppStartParam=d42195_t12600");
    check("startapp opens the plan it names", s.distance === "42195" && s.time === "3:30:00" && s.pace === "4:59", s);
    check("the heading shows that plan", s.plan.includes("Марафон за 3:30:00"), s);
    check("the address becomes an ordinary share link", s.search.includes("d=42195") && s.search.includes("t=3%3A30%3A00") && !s.search.includes("tgWebAppStartParam"), s);
    check("console clean with startapp", consoleProblems.length === 0, consoleProblems);

    s = await open("/#tgWebAppStartParam=d10000_t2900");
    check("startapp is read from the hash too", s.distance === "10000" && s.time === "0:48:20", s);

    s = await open("/?d=5000&t=25:00&tgWebAppStartParam=d42195_t12600");
    check("d and t in the address win over startapp", s.distance === "5000" && s.time === "0:25:00", s);

    for (const bad of ["d0_t100", "d42195_t0", "d42195", "junk", "d42195_t12600x", "d99999999_t100"]) {
        s = await open(`/?tgWebAppStartParam=${bad}`);
        check(`broken startapp ${bad} leaves the default plan`, s.distance === plain.distance && s.time === plain.time, { s, plain });
        check(`console clean with ${bad}`, consoleProblems.length === 0, consoleProblems);
    }
} catch (error) {
    check("runner: no exception", false, String(error && error.stack || error));
} finally {
    clearTimeout(hardStop);
    ws.close();
    cleanup();
}

const failed = results.filter((r) => !r.ok).length;
console.log(`\n${results.length - failed}/${results.length} checks passed`);
process.exit(failed ? 1 : 0);
