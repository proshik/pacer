// Headless Chrome check of validation messages: each one sits next to the
// field it is about, says what is wrong, and is wired to the field for
// screen readers.
import { spawn } from "node:child_process";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const ROOT = fileURLToPath(new URL("../../", import.meta.url));
const ASSETS = process.env.ASSETS || join(ROOT, "assets");
const CHROME = process.env.CHROME || "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
// Screenshots and the throwaway Chrome profile; set CHECK_OUT to keep them.
const OUT = process.env.CHECK_OUT || mkdtempSync(join(tmpdir(), "pacer-checks-"));
const SHOTS = join(OUT, "shots");
const HTTP_PORT = 8813;
const CDP_PORT = 9314;
const BASE = `http://127.0.0.1:${HTTP_PORT}`;

mkdirSync(SHOTS, { recursive: true });
const profile = mkdtempSync(join(OUT, "chrome-"));
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
    results.push({ name, ok: Boolean(ok) });
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

async function input(id, value) {
    await evaluate(`(() => { const el = document.getElementById(${JSON.stringify(id)}); el.value = ${JSON.stringify(value)}; el.dispatchEvent(new Event("input", {bubbles: true})); return true; })()`);
    await sleep(150);
}

// Messages, where they sit and how the fields point at them.
const STATE = `(() => {
    const groups = {distance: ["distanceError", ".distance", ["distInput"]],
        time: ["timeError", "#timeReadout", ["timeHourInput", "timeMinuteInput", "timeSecondInput"]],
        pace: ["paceError", "#paceReadout", ["paceMinuteInput", "paceSecondInput"]]};
    const out = {};
    for (const [name, [id, container, inputs]] of Object.entries(groups)) {
        const message = document.getElementById(id);
        out[name] = {
            text: message ? message.textContent : null,
            inside: Boolean(message && document.querySelector(container).contains(message)),
            visible: Boolean(message && message.textContent && message.getBoundingClientRect().height > 0),
            described: inputs.every((i) => (document.getElementById(i).getAttribute("aria-describedby") || "").split(" ").includes(id)),
            invalid: inputs.filter((i) => document.getElementById(i).getAttribute("aria-invalid") === "true")
        };
    }
    out.legacy = Boolean(document.getElementById("errorMessage"));
    return out;
})()`;

try {
    await send("Page.navigate", { url: `${BASE}/` });
    await waitFor(() => evaluate(`document.readyState === "complete" && document.body.dataset.state === "ready"`), 20000, "ready");
    await sleep(300);

    let s = await evaluate(STATE);
    check("no messages on a valid page", !s.distance.text && !s.time.text && !s.pace.text, s);
    check("each field points at its own message", s.distance.described && s.time.described && s.pace.described, s);
    check("messages live next to their fields", s.distance.inside && s.time.inside && s.pace.inside, s);
    check("the single bottom line is gone", !s.legacy, s.legacy);

    await input("timeMinuteInput", "75");
    s = await evaluate(STATE);
    check("minutes out of range: message under the time", s.time.visible && /59/.test(s.time.text), s.time);
    check("minutes out of range: only minutes are invalid", JSON.stringify(s.time.invalid) === '["timeMinuteInput"]', s.time.invalid);
    check("minutes out of range: nothing under pace or distance", !s.pace.text && !s.distance.text, s);
    await send("Page.captureScreenshot", { format: "png" }).then(({ data }) => writeFileSync(join(SHOTS, "error-time-390.png"), Buffer.from(data, "base64")));

    await input("timeMinuteInput", "25");
    s = await evaluate(STATE);
    check("fixing the field clears its message and state", !s.time.text && s.time.invalid.length === 0, s.time);

    await input("timeHourInput", "0");
    await input("timeMinuteInput", "0");
    await input("timeSecondInput", "0");
    s = await evaluate(STATE);
    check("zero time is refused under the time", s.time.visible && s.time.invalid.length === 3, s.time);
    await input("timeHourInput", "1");
    await input("timeMinuteInput", "25");
    await input("timeSecondInput", "39");

    await input("distInput", "0");
    s = await evaluate(STATE);
    check("zero distance: message under the distance", s.distance.visible && JSON.stringify(s.distance.invalid) === '["distInput"]', s.distance);
    check("zero distance: nothing under time", !s.time.text, s.time);
    await input("distInput", "21097");

    await input("paceSecondInput", "75");
    s = await evaluate(STATE);
    check("pace seconds out of range: message under the pace", s.pace.visible && /59/.test(s.pace.text)
        && JSON.stringify(s.pace.invalid) === '["paceSecondInput"]', s.pace);

    await input("paceMinuteInput", "0");
    await input("paceSecondInput", "0");
    s = await evaluate(STATE);
    check("zero pace: says the pace cannot be 0:00", s.pace.visible && /0:00/.test(s.pace.text) && s.pace.invalid.length === 2, s.pace);
    await send("Page.captureScreenshot", { format: "png", captureBeyondViewport: true }).then(({ data }) => writeFileSync(join(SHOTS, "error-pace-390.png"), Buffer.from(data, "base64")));

    await input("paceMinuteInput", "4");
    await input("paceSecondInput", "04");
    s = await evaluate(STATE);
    check("valid pace again: all clear", !s.distance.text && !s.time.text && !s.pace.text, s);
    // Desktop: time and pace stand side by side; a message must not widen the
    // time column and push the pace aside.
    await send("Emulation.setDeviceMetricsOverride", { width: 1280, height: 900, deviceScaleFactor: 1, mobile: false });
    await sleep(200);
    const paceLeft = () => evaluate(`document.getElementById("paceReadout").getBoundingClientRect().left`);
    const before = await paceLeft();
    await input("timeMinuteInput", "75");
    const after = await paceLeft();
    check("desktop: a message does not push the pace column", Math.abs(before - after) < 0.5, { before, after });
    await send("Page.captureScreenshot", { format: "png" }).then(({ data }) => writeFileSync(join(SHOTS, "error-time-1280.png"), Buffer.from(data, "base64")));
    await input("timeMinuteInput", "25");
    await send("Emulation.setDeviceMetricsOverride", { width: 390, height: 844, deviceScaleFactor: 2, mobile: true });
    await sleep(200);

    check("no horizontal scroll", !(await evaluate(`document.documentElement.scrollWidth > document.documentElement.clientWidth`)));
    check("console clean", consoleProblems.length === 0, consoleProblems);
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
