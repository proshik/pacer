// Headless Chrome check of saved runs: in this browser when there is no server
// history, on the server inside the Mini App, and gone when storage is blocked.
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
const HTTP_PORT = 8814;
const CDP_PORT = 9315;
const BASE = `http://127.0.0.1:${HTTP_PORT}`;
const RUNS_KEY = "pacer.runs";

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
const hardStop = setTimeout(() => { console.error("HARD TIMEOUT"); cleanup(); process.exit(3); }, 150000);
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
function check(scenario, name, ok, detail) {
    results.push({ ok: Boolean(ok) });
    console.log(`${ok ? "PASS" : "FAIL"} [${scenario}] ${name}${ok ? "" : " — " + JSON.stringify(detail)}`);
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

// Fake API: history either answers with serverRuns or is not configured (503).
let serverMode = "runs";
let serverPosts = 0;
const serverRuns = [{ id: 9, distance: 42195, time: { text: "3h10m49s", seconds: 11449 }, saved_at: "2026-09-01T08:00:00Z" }];
listeners.push(async (msg) => {
    if (msg.method !== "Fetch.requestPaused") return;
    const { requestId, request } = msg.params;
    const reply = (status, body, type = "application/json") => send("Fetch.fulfillRequest", {
        requestId, responseCode: status, responseHeaders: [{ name: "Content-Type", value: type }],
        body: Buffer.from(body).toString("base64")
    }).catch(() => {});
    if (request.url.includes("telegram.org")) return reply(200, "", "application/javascript");
    if (request.url.endsWith("/api/v1/me")) return reply(200, JSON.stringify({ user: { id: 7, first_name: "Ann" } }));
    if (request.url.includes("/api/v1/runs")) {
        if (request.method === "POST") serverPosts++;
        if (serverMode === "unconfigured") return reply(503, JSON.stringify({ error: "history is not configured" }));
        if (request.method === "GET") return reply(200, JSON.stringify({ runs: serverRuns }));
        return reply(201, "{}");
    }
    return reply(404, "{}");
});

await send("Runtime.enable");
await send("Page.enable");
await send("Fetch.enable", { patterns: [{ urlPattern: "*/api/v1/*" }, { urlPattern: "*telegram.org*" }] });
await send("Emulation.setUserAgentOverride", { userAgent: version["User-Agent"], acceptLanguage: "ru-RU,ru" });
await send("Emulation.setDeviceMetricsOverride", { width: 390, height: 844, deviceScaleFactor: 2, mobile: true });

function telegramHash() {
    const user = JSON.stringify({ id: 7, first_name: "Ann", language_code: "ru" });
    const initData = new URLSearchParams({ query_id: "AAE", user, auth_date: "1757500000", hash: "0".repeat(64) }).toString();
    return "#" + new URLSearchParams({ tgWebAppData: initData, tgWebAppVersion: "8.0" }).toString();
}

let prep = 0;
async function open({ hash = "", runs = null }) {
    await send("Page.navigate", { url: `${BASE}/index.html?prep=${++prep}` });
    await waitFor(() => evaluate(`document.readyState === "complete"`), 15000, "prep");
    await evaluate(`localStorage.clear(); ${runs ? `localStorage.setItem(${JSON.stringify(RUNS_KEY)}, ${JSON.stringify(JSON.stringify(runs))});` : ""} true`);
    consoleProblems = [];
    await send("Page.navigate", { url: `${BASE}/${hash}` });
    await waitReady();
}
async function waitReady() {
    await waitFor(() => evaluate(`document.readyState === "complete" && document.body.dataset.state === "ready"`), 20000, "ready");
    await sleep(400);
}

const STATE = `(() => ({
    section: !document.getElementById("savedSection").hidden,
    empty: !document.getElementById("savedEmpty").hidden,
    save: !document.getElementById("saveButton").hidden,
    shareSecondary: document.getElementById("shareButton").classList.contains("is-secondary"),
    items: [...document.querySelectorAll("#savedList li .saved-open")].map((b) => b.innerText.replace(/\\n/g, " | ")),
    note: document.getElementById("shareNote").textContent,
    stored: JSON.parse(localStorage.getItem(${JSON.stringify(RUNS_KEY)}) || "null"),
    plan: document.getElementById("planLine").textContent
}))()`;

async function click(selector) {
    await evaluate(`document.querySelector(${JSON.stringify(selector)}).click(), true`);
    await sleep(300);
}
async function input(id, value) {
    await evaluate(`(() => { const el = document.getElementById(${JSON.stringify(id)}); el.value = ${JSON.stringify(value)}; el.dispatchEvent(new Event("input", {bubbles: true})); return true; })()`);
    await sleep(150);
}

try {
    // A. Plain browser: saved in this browser.
    {
        const s = "browser";
        await open({});
        let r = await evaluate(STATE);
        check(s, "saved runs are offered outside Telegram", r.section && r.empty && r.save && r.shareSecondary, r);

        await click("#saveButton");
        r = await evaluate(STATE);
        check(s, "save lists the run", r.items.length === 1 && r.items[0].startsWith("Полумарафон, 1:25:39 | темп 4:04 /км, "), r.items);
        check(s, "save confirms", r.note === "Расчёт сохранён.", r.note);
        check(s, "save keeps it in this browser", r.stored && r.stored.length === 1 && r.stored[0].distance === 21097 && r.stored[0].time.seconds === 5139, r.stored);

        await click("#b10km");
        await click("#saveButton");
        r = await evaluate(STATE);
        // the page keeps "10 км" together with a non-breaking space
        check(s, "newest first", r.items.length === 2 && r.items[0].startsWith("10 км, 1:25:39"), r.items);

        await send("Page.reload");
        await waitReady();
        r = await evaluate(STATE);
        check(s, "runs survive a reload", r.items.length === 2 && r.items[0].startsWith("10 км") && r.items[1].startsWith("Полумарафон"), r.items);

        await click("#savedList li:nth-child(2) .saved-open");
        const opened = await evaluate(`[document.getElementById("distInput").value, document.getElementById("timeHourInput").value, document.getElementById("timeMinuteInput").value, document.getElementById("timeSecondInput").value].join(" ")`);
        check(s, "opening a run restores it", opened === "21097 1 25 39", opened);

        await click("#savedList li:nth-child(1) .saved-delete");
        r = await evaluate(STATE);
        check(s, "delete removes it from the list and the browser", r.items.length === 1 && r.stored.length === 1 && r.stored[0].distance === 21097, r);

        await input("timeMinuteInput", "75");
        await click("#saveButton");
        r = await evaluate(STATE);
        check(s, "an invalid field saves the plan on screen, not a recomputed time", r.items[0].startsWith("Полумарафон, 1:25:39"), r.items);
        await input("timeMinuteInput", "25");

        check(s, "no horizontal scroll", !(await evaluate(`document.documentElement.scrollWidth > document.documentElement.clientWidth`)));
        check(s, "console clean", consoleProblems.length === 0, consoleProblems);
        const { data } = await send("Page.captureScreenshot", { format: "png", captureBeyondViewport: true });
        writeFileSync(join(SHOTS, "saved-local-390.png"), Buffer.from(data, "base64"));
    }

    // B. At most 50, like the server: the oldest goes.
    {
        const s = "cap";
        const seeded = Array.from({ length: 50 }, (_, i) => ({ id: 50 - i, distance: 5000, time: { seconds: 1200 + i }, saved_at: "2026-09-01T08:00:00Z" }));
        await open({ runs: seeded });
        await click("#saveButton");
        const r = await evaluate(STATE);
        check(s, "keeps 50, newest first, drops the oldest", r.stored.length === 50 && r.stored[0].distance === 21097
            && !r.stored.some((run) => run.id === 1), { length: r.stored.length, first: r.stored[0], ids: r.stored.map((x) => x.id).slice(-3) });
    }

    // C. Storage blocked: no section, the calculator still works.
    {
        const s = "blocked storage";
        const { identifier } = await send("Page.addScriptToEvaluateOnNewDocument", {
            source: `Storage.prototype.setItem = function () { throw new DOMException("blocked", "SecurityError"); };`
        });
        await open({});
        const r = await evaluate(STATE);
        check(s, "no saved runs section without storage", !r.section && !r.save && !r.shareSecondary, r);
        check(s, "calculator still works", r.plan.includes("4:04"), r.plan);
        check(s, "console clean", consoleProblems.length === 0, consoleProblems);
        await send("Page.removeScriptToEvaluateOnNewDocument", { identifier });
    }

    // D. Mini App with server history: the server's list, nothing in this browser.
    {
        const s = "mini app, server";
        serverMode = "runs";
        await open({ hash: telegramHash() });
        await waitFor(() => evaluate(`!document.getElementById("savedSection").hidden`), 5000, "saved section");
        const r = await evaluate(STATE);
        check(s, "server runs are shown", r.items.length === 1 && r.items[0].startsWith("Марафон, 3:10:49"), r.items);
        check(s, "nothing is kept in this browser", r.stored === null, r.stored);
    }

    // E. Mini App without server history: this browser instead.
    {
        const s = "mini app, no server history";
        serverMode = "unconfigured";
        serverPosts = 0;
        await open({ hash: telegramHash() });
        await waitFor(() => evaluate(`!document.getElementById("savedSection").hidden`), 5000, "saved section");
        await click("#saveButton");
        const r = await evaluate(STATE);
        check(s, "falls back to this browser", r.items.length === 1 && r.stored && r.stored.length === 1 && serverPosts === 0, { r, serverPosts });
        check(s, "console clean", consoleProblems.length === 0, consoleProblems);
    }
} catch (error) {
    check("runner", "no exception", false, String(error && error.stack || error));
} finally {
    clearTimeout(hardStop);
    ws.close();
    cleanup();
}

const failed = results.filter((r) => !r.ok).length;
console.log(`\n${results.length - failed}/${results.length} checks passed`);
process.exit(failed ? 1 : 0);
