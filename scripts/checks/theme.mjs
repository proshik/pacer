// Headless Chrome check of the light/dark switch: it follows the system until
// chosen, flips every token (the band and the road included) against the
// system theme, remembers the choice, applies it before the page script runs,
// works before WASM loads and stays out of the Telegram Mini App.
// Usage: node scripts/checks/theme.mjs
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
const HTTP_PORT = 8819;
const CDP_PORT = 9319;
const BASE = `http://127.0.0.1:${HTTP_PORT}`;
const STORAGE_KEY = "pacer.theme";
const COLORS = { light: "#F6F9FC", dark: "#07101C" };

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
function check(scenario, name, ok, detail) {
    results.push({ scenario, name, ok: Boolean(ok) });
    console.log(`${ok ? "PASS" : "FAIL"} [${scenario}] ${name}${ok ? "" : " — " + JSON.stringify(detail)}`);
}

// ---------- CDP ----------
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
        if (msg.error) p.reject(new Error(JSON.stringify(msg.error)));
        else p.resolve(msg.result);
    } else {
        listeners.forEach((listener) => listener(msg));
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

// Requests the scenario wants to hold: the page script (wasm_exec.js is the
// first script after the theme is set) and the WASM module.
let hold = { wasmExec: null, wasm: null };
const held = [];
listeners.push(async (msg) => {
    if (msg.method !== "Fetch.requestPaused") return;
    const { requestId, request } = msg.params;
    const reply = (status, body, type) => send("Fetch.fulfillRequest", {
        requestId, responseCode: status,
        responseHeaders: [{ name: "Content-Type", value: type }],
        body: Buffer.from(body).toString("base64")
    }).catch(() => {});
    if (request.url.includes("telegram.org")) return reply(200, "", "application/javascript");
    if (request.url.endsWith("/api/v1/me")) return reply(200, JSON.stringify({ user: { id: 7, first_name: "Ann" } }), "application/json");
    if (request.url.includes("/api/v1/")) return reply(503, "{}", "application/json");
    const kind = request.url.endsWith("/wasm_exec.js") ? "wasmExec" : request.url.endsWith("/json.wasm") ? "wasm" : null;
    if (kind && hold[kind]) {
        held.push(requestId);
        return hold[kind]();
    }
    return send("Fetch.continueRequest", { requestId }).catch(() => {});
});
const release = () => Promise.all(held.splice(0).map((requestId) => send("Fetch.continueRequest", { requestId }).catch(() => {})));

await send("Runtime.enable");
await send("Page.enable");
await send("Network.enable");
// A worker answering from its cache would bypass the held requests.
await send("Network.setBypassServiceWorker", { bypass: true });
await send("Fetch.enable", { patterns: [{ urlPattern: "*/api/v1/*" }, { urlPattern: "*telegram.org*" }, { urlPattern: "*wasm_exec.js*" }, { urlPattern: "*json.wasm*" }] });

let prep = 0;
async function open({ scheme = "light", storage = null, hash = "", acceptLanguage = "ru-RU,ru", width = 390, wait = true, before = null }) {
    await send("Emulation.setUserAgentOverride", { userAgent: "Mozilla/5.0 Chrome", acceptLanguage });
    await send("Emulation.setDeviceMetricsOverride", { width, height: 900, deviceScaleFactor: 1, mobile: width < 700 });
    await send("Emulation.setEmulatedMedia", { features: [{ name: "prefers-color-scheme", value: scheme }] });
    await send("Page.navigate", { url: `${BASE}/index.html?prep=${++prep}` });
    await waitFor(() => evaluate(`document.readyState === "complete"`), 15000, "prep load");
    await evaluate(`localStorage.clear(); ${storage ? `localStorage.setItem(${JSON.stringify(STORAGE_KEY)}, ${JSON.stringify(storage)});` : ""} true`);
    consoleProblems = [];
    if (before) before();
    await send("Page.navigate", { url: `${BASE}/?d=21097&t=1:38:48${hash}` });
    if (wait) await ready();
}

async function ready() {
    await waitFor(() => evaluate(`document.readyState === "complete" && document.body.dataset.state === "ready"`), 20000, "wasm ready");
    await sleep(200);
}

// What the theme decides: the tokens of the paper, the band and the road, the
// painted colours, the meta theme colours and the switch itself.
const STATE = `(() => {
    const root = getComputedStyle(document.documentElement);
    const tokens = {};
    for (const name of ["--paper", "--ink", "--muted", "--line", "--strip", "--strip-line", "--strip-well", "--road", "--footer-bg"]) {
        tokens[name] = root.getPropertyValue(name).trim();
    }
    const toggle = document.querySelector(".theme-toggle");
    return {
        theme: document.documentElement.dataset.theme || null,
        tokens,
        scheme: root.colorScheme,
        paper: getComputedStyle(document.body).backgroundColor,
        strip: getComputedStyle(document.querySelector(".strip")).backgroundColor,
        footer: getComputedStyle(document.querySelector("footer")).backgroundColor,
        metas: [...document.querySelectorAll('meta[name="theme-color"]')].map((m) => m.content),
        toggle: toggle && {hidden: toggle.hidden, visible: toggle.getBoundingClientRect().width > 0, pressed: toggle.getAttribute("aria-pressed"),
            label: toggle.getAttribute("aria-label"), disabled: toggle.disabled},
        stored: (() => { try { return localStorage.getItem(${JSON.stringify(STORAGE_KEY)}); } catch (e) { return "unavailable"; } })()
    };
})()`;

const clickToggle = async () => { await evaluate(`document.querySelector(".theme-toggle").click(), true`); await sleep(150); };
const same = (a, b) => JSON.stringify(a) === JSON.stringify(b);

try {
    // Reference: the system theme with no choice made.
    await open({ scheme: "light" });
    const light = await evaluate(STATE);
    await open({ scheme: "dark" });
    const dark = await evaluate(STATE);
    check("reference", "the system themes differ", !same(light.tokens, dark.tokens) && light.paper !== dark.paper && light.strip !== dark.strip && light.footer !== dark.footer, { light, dark });

    // 1. Without a choice the page follows the system and the switch says so.
    {
        const s = "system";
        check(s, "light system: no data-theme, switch shown and not pressed", light.theme === null && light.toggle && light.toggle.visible && light.toggle.pressed === "false", light.toggle);
        check(s, "dark system: switch pressed", dark.theme === null && dark.toggle && dark.toggle.pressed === "true", dark.toggle);
        check(s, "switch named in Russian", light.toggle && light.toggle.label === "Тёмная тема", light.toggle);
        check(s, "meta theme colours follow the system", same(light.metas, [COLORS.light, COLORS.dark]), light.metas);
    }

    // 2. Dark against a light system: every token flips, the choice is stored.
    {
        const s = "dark on a light system";
        await open({ scheme: "light", width: 1440 });
        await clickToggle();
        const r = await evaluate(STATE);
        check(s, "data-theme is dark and the switch is pressed", r.theme === "dark" && r.toggle.pressed === "true", r);
        check(s, "paper, band and road take the dark tokens", same(r.tokens, dark.tokens) && r.paper === dark.paper && r.strip === dark.strip, { r: r.tokens, dark: dark.tokens });
        check(s, "the footer band takes the dark colour", r.footer === dark.footer && r.footer !== r.paper, { footer: r.footer, dark: dark.footer, paper: r.paper });
        check(s, "color-scheme is dark", r.scheme === "dark", r.scheme);
        check(s, "both meta theme colours are dark", same(r.metas, [COLORS.dark, COLORS.dark]), r.metas);
        check(s, "the choice is stored", r.stored === "dark", r.stored);
        check(s, "console clean", consoleProblems.length === 0, consoleProblems);
        const { data } = await send("Page.captureScreenshot", { format: "png", captureBeyondViewport: true });
        writeFileSync(join(SHOTS, "theme-dark-on-light-1440.png"), Buffer.from(data, "base64"));

        await send("Page.reload");
        await ready();
        const again = await evaluate(STATE);
        check(s, "the choice survives a reload", again.theme === "dark" && same(again.tokens, dark.tokens) && again.toggle.pressed === "true", again);

        await clickToggle();
        const back = await evaluate(STATE);
        check(s, "a second click returns to light", back.theme === "light" && same(back.tokens, light.tokens) && back.stored === "light", back);
    }

    // 3. Light against a dark system.
    {
        const s = "light on a dark system";
        await open({ scheme: "dark", width: 390 });
        await clickToggle();
        const r = await evaluate(STATE);
        check(s, "paper, band and road take the light tokens", r.theme === "light" && same(r.tokens, light.tokens) && r.paper === light.paper && r.strip === light.strip, { r: r.tokens, light: light.tokens });
        check(s, "color-scheme and meta follow", r.scheme === "light" && same(r.metas, [COLORS.light, COLORS.light]), r);
        check(s, "the footer band takes the light colour", r.footer === light.footer && r.footer !== r.paper, { footer: r.footer, light: light.footer, paper: r.paper });
        check(s, "switch not pressed", r.toggle.pressed === "false", r.toggle);
        const { data } = await send("Page.captureScreenshot", { format: "png", captureBeyondViewport: true });
        writeFileSync(join(SHOTS, "theme-light-on-dark-390.png"), Buffer.from(data, "base64"));
    }

    // 4. No flash: the stored theme is on <html> before the page script loads,
    // and the switch works while WASM is still loading.
    {
        const s = "before the page script";
        let seen = null;
        await open({ scheme: "light", storage: "dark", wait: false, before: () => {
            hold.wasmExec = async () => { seen = await evaluate(`({theme: document.documentElement.dataset.theme || null, body: Boolean(document.body)})`); await release(); };
            hold.wasm = async () => {};
        } });
        await waitFor(() => seen, 10000, "held page script");
        check(s, "data-theme is set while the page script is still loading", seen.theme === "dark" && !seen.body, seen);
        await waitFor(() => evaluate(`document.readyState !== "loading" && document.body.dataset.state === "loading"`), 10000, "page parsed, WASM held");
        const before = await evaluate(STATE);
        check(s, "before WASM the switch is shown and enabled", before.toggle.visible && !before.toggle.disabled, before.toggle);
        await clickToggle();
        check(s, "before WASM a click switches the theme", (await evaluate(STATE)).theme === "light");
        hold = { wasmExec: null, wasm: null };
        await release();
        await ready();
        check(s, "after WASM the switch stays enabled", !(await evaluate(STATE)).toggle.disabled);
    }

    // 5. English.
    {
        const s = "english";
        await open({ scheme: "light", acceptLanguage: "en-US,en" });
        check(s, "switch named in English", (await evaluate(STATE)).toggle.label === "Dark theme");
        await evaluate(`document.querySelector('.lang-option[data-lang="ru"]').click(), true`);
        await sleep(150);
        check(s, "the name follows the language switch", (await evaluate(STATE)).toggle.label === "Тёмная тема");
    }

    // 6. Telegram sets the neutrals itself: no switch, the stored choice is ignored.
    {
        const s = "telegram";
        const user = JSON.stringify({ id: 7, first_name: "Ann", language_code: "ru" });
        const initData = new URLSearchParams({ query_id: "AAE", user, auth_date: "1757500000", hash: "0".repeat(64) }).toString();
        const hash = "#" + new URLSearchParams({ tgWebAppData: initData, tgWebAppVersion: "8.0" }).toString();
        await open({ scheme: "light", storage: "dark", hash });
        const r = await evaluate(STATE);
        check(s, "no switch inside the Mini App", r.toggle && r.toggle.hidden && !r.toggle.visible, r.toggle);
        check(s, "the stored choice is not applied", r.theme === null && same(r.tokens, light.tokens), r);
        check(s, "console clean", consoleProblems.length === 0, consoleProblems);
    }

    // 7. Without storage the switch still works until a reload.
    {
        const s = "no storage";
        const { identifier } = await send("Page.addScriptToEvaluateOnNewDocument", {
            source: `Object.defineProperty(window, "localStorage", {get() { throw new DOMException("denied", "SecurityError"); }});`
        });
        await send("Emulation.setEmulatedMedia", { features: [{ name: "prefers-color-scheme", value: "light" }] });
        consoleProblems = [];
        await send("Page.navigate", { url: `${BASE}/?d=21097&t=1:38:48` });
        await ready();
        await clickToggle();
        const r = await evaluate(STATE);
        check(s, "a click switches the theme without storage", r.theme === "dark" && r.stored === "unavailable", r);
        check(s, "console clean", consoleProblems.length === 0, consoleProblems);
        await send("Page.removeScriptToEvaluateOnNewDocument", { identifier });
    }

    // 8. Without JS there is nothing to switch: the markup hides the button.
    {
        const s = "no script";
        const hidden = await evaluate(`fetch("/index.html").then((r) => r.text()).then((html) => {
            const toggle = new DOMParser().parseFromString(html, "text/html").querySelector(".theme-toggle");
            return toggle ? toggle.hasAttribute("hidden") : null;
        })`);
        check(s, "the button is hidden in the markup", hidden === true, hidden);
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
