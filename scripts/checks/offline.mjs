// Headless Chrome check of the installable, offline page: manifest, icons,
// link preview tags and the service worker. Offline is real: the HTTP server is
// killed, so nothing but the service worker's cache can answer.
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
const HTTP_PORT = 8812;
const CDP_PORT = 9313;
const BASE = `http://127.0.0.1:${HTTP_PORT}`;

mkdirSync(SHOTS, { recursive: true });
const profile = mkdtempSync(join(OUT, "chrome-"));

let server = spawn("python3", ["-m", "http.server", String(HTTP_PORT), "-d", ASSETS, "--bind", "127.0.0.1"], { stdio: "ignore" });
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
    if (msg.method === "Log.entryAdded" && msg.params.entry.level === "error") {
        consoleProblems.push(msg.params.entry.text + " " + (msg.params.entry.url || ""));
    }
});

await send("Runtime.enable");
await send("Log.enable");
await send("Page.enable");
await send("Emulation.setDeviceMetricsOverride", { width: 390, height: 844, deviceScaleFactor: 2, mobile: true });

async function load(url) {
    await send("Page.navigate", { url });
    await waitFor(() => evaluate(`document.readyState === "complete" && document.body.dataset.state === "ready"`), 20000, "wasm ready at " + url);
    await sleep(300);
}

try {
    // Online: tags, manifest, icons, service worker.
    await load(`${BASE}/`);

    const head = await evaluate(`(() => {
        const attr = (selector, name) => { const el = document.querySelector(selector); return el && el.getAttribute(name); };
        return {
            manifest: attr('link[rel="manifest"]', "href"),
            icon: attr('link[rel="icon"]', "href"),
            touch: attr('link[rel="apple-touch-icon"]', "href"),
            ogType: attr('meta[property="og:type"]', "content"),
            ogTitle: attr('meta[property="og:title"]', "content"),
            ogDescription: attr('meta[property="og:description"]', "content"),
            ogImage: attr('meta[property="og:image"]', "content"),
            card: attr('meta[name="twitter:card"]', "content")
        };
    })()`);
    check("page links a manifest", head.manifest, head);
    check("page links a favicon and a touch icon", head.icon && head.touch, head);
    check("page carries link preview tags", head.ogType && head.ogTitle && head.ogDescription && head.ogImage && head.card, head);

    const manifest = await send("Page.getAppManifest");
    check("manifest parses without errors", manifest.data && manifest.errors.length === 0, manifest.errors);
    const data = manifest.data ? JSON.parse(manifest.data) : {};
    check("manifest is installable", data.name && data.start_url && data.display === "standalone"
        && (data.icons || []).some((i) => i.sizes === "512x512") && (data.icons || []).some((i) => i.purpose === "maskable"), data);

    const linked = [head.icon, head.touch, head.ogImage, ...(data.icons || []).map((i) => i.src)].filter(Boolean);
    const fetched = await evaluate(`Promise.all(${JSON.stringify(linked)}.map(async (src) => {
        const response = await fetch(src);
        return {src, status: response.status, type: response.headers.get("content-type")};
    }))`);
    check("every icon loads as an image", fetched.every((f) => f.status === 200 && /^image\//.test(f.type)), fetched);

    const sw = await evaluate(`(async () => {
        if (!("serviceWorker" in navigator)) return {supported: false};
        const registration = await Promise.race([
            navigator.serviceWorker.ready,
            new Promise((resolve) => setTimeout(() => resolve(null), 8000))
        ]);
        return {supported: true, active: registration && registration.active && registration.active.state, scope: registration && registration.scope};
    })()`);
    check("service worker activates for the whole site", sw.active === "activated" && sw.scope === `${BASE}/`, sw);
    check("console clean online", consoleProblems.length === 0, consoleProblems);

    // A reload puts the page under the worker's control.
    await send("Page.reload");
    await waitFor(() => evaluate(`document.readyState === "complete" && document.body.dataset.state === "ready"`), 20000, "reload");
    check("page is controlled after a reload", await evaluate(`Boolean(navigator.serviceWorker && navigator.serviceWorker.controller)`));

    // Offline: kill the server, open another calculation.
    server.kill("SIGKILL");
    await waitFor(async () => { try { await fetch(`${BASE}/index.html`); return false; } catch { return true; } }, 5000, "server down");
    consoleProblems = [];
    await load(`${BASE}/?d=42195&t=3:44:20`);
    const offline = await evaluate(`({plan: document.getElementById("planLine").textContent, pace: document.getElementById("paceMinuteInput").value + ":" + document.getElementById("paceSecondInput").value})`);
    check("offline: a shared link opens from the cache and calculates", offline.plan.includes("3:44:20") && offline.pace === "5:19", offline);
    check("console clean offline", consoleProblems.length === 0, consoleProblems);

    const { data: shot } = await send("Page.captureScreenshot", { format: "png" });
    writeFileSync(join(SHOTS, "offline-390.png"), Buffer.from(shot, "base64"));
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
