// Headless Chrome check of the page's localization, driven over CDP.
// Usage: node scripts/checks/language.mjs   (exit 0 = all checks pass)
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
const HTTP_PORT = 8811;
const CDP_PORT = 9311;
const BASE = `http://127.0.0.1:${HTTP_PORT}`;
const STORAGE_KEY = "pacer.lang";

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
    results.push({ scenario, name, ok: Boolean(ok), detail });
    console.log(`${ok ? "PASS" : "FAIL"} [${scenario}] ${name}${ok ? "" : " — " + JSON.stringify(detail)}`);
}

// ---------- CDP ----------
await waitFor(async () => (await fetch(`http://127.0.0.1:${CDP_PORT}/json/version`)).ok, 15000, "chrome");
await waitFor(async () => (await fetch(`${BASE}/index.html`)).ok, 15000, "http server");
const version = await (await fetch(`http://127.0.0.1:${CDP_PORT}/json/version`)).json();
const targets = await (await fetch(`http://127.0.0.1:${CDP_PORT}/json/list`)).json();
const page = targets.find((t) => t.type === "page");
const ws = new WebSocket(page.webSocketDebuggerUrl);
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

// Console problems are collected per scenario.
let consoleProblems = [];
listeners.push((msg) => {
    if (msg.method === "Runtime.consoleAPICalled" && ["error", "warning", "assert"].includes(msg.params.type)) {
        consoleProblems.push(msg.params.args.map((a) => a.value ?? a.description).join(" "));
    }
    if (msg.method === "Runtime.exceptionThrown") {
        consoleProblems.push(msg.params.exceptionDetails.exception?.description || msg.params.exceptionDetails.text);
    }
    if (msg.method === "Log.entryAdded" && msg.params.entry.level === "error" && !String(msg.params.entry.url).endsWith("favicon.ico")) {
        consoleProblems.push(msg.params.entry.text + " " + (msg.params.entry.url || ""));
    }
});

// Fake API and an empty Telegram SDK, so nothing leaves the machine.
const runs = [{ id: 1, distance: 21097, time: { text: "1h38m48s", seconds: 5928 }, saved_at: "2026-09-10T08:00:00Z" }];
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
    if (request.url.endsWith("/api/v1/runs") && request.method === "GET") return reply(200, JSON.stringify({ runs }), "application/json");
    return reply(404, "{}", "application/json");
});

await send("Runtime.enable");
await send("Log.enable");
await send("Page.enable");
await send("Fetch.enable", { patterns: [{ urlPattern: "*/api/v1/*" }, { urlPattern: "*telegram.org*" }] });

function telegramHash(languageCode) {
    const user = JSON.stringify({ id: 7, first_name: "Ann", language_code: languageCode });
    const initData = new URLSearchParams({ query_id: "AAE", user, auth_date: "1757500000", hash: "0".repeat(64) }).toString();
    const theme = JSON.stringify({ bg_color: "#ffffff", text_color: "#000000", hint_color: "#999999" });
    return "#" + new URLSearchParams({ tgWebAppData: initData, tgWebAppVersion: "8.0", tgWebAppThemeParams: theme }).toString();
}

let prep = 0;
async function open({ acceptLanguage, path = "/", hash = "", storage = null, width = 390, mobile = true, scheme = "light" }) {
    await send("Emulation.setUserAgentOverride", { userAgent: version["User-Agent"], acceptLanguage });
    await send("Emulation.setDeviceMetricsOverride", { width, height: 844, deviceScaleFactor: 2, mobile });
    await send("Emulation.setEmulatedMedia", { features: [{ name: "prefers-color-scheme", value: scheme }] });

    // A different path forces a full load, so the page never reuses the last run's state.
    await send("Page.navigate", { url: `${BASE}/index.html?prep=${++prep}` });
    await waitFor(() => evaluate(`document.readyState === "complete"`), 15000, "prep load");
    await evaluate(`localStorage.clear(); ${storage ? `localStorage.setItem(${JSON.stringify(STORAGE_KEY)}, ${JSON.stringify(storage)});` : ""} true`);

    consoleProblems = [];
    await send("Page.navigate", { url: BASE + path + hash });
    await waitFor(() => evaluate(`document.readyState === "complete" && document.body.dataset.state === "ready"`), 20000, "wasm ready");
    await sleep(300);
}

// Cyrillic anywhere the reader or a screen reader can meet it, except inside
// elements that declare their own language (the language switcher's labels).
const REPORT = `(() => {
    const clone = document.body.cloneNode(true);
    clone.querySelectorAll("[lang]").forEach((n) => n.remove());
    clone.querySelectorAll("script").forEach((n) => n.remove());
    const texts = [clone.textContent, document.title, document.querySelector('meta[name="description"]').content];
    document.querySelectorAll("[aria-label]").forEach((el) => {
        if (!el.closest("body [lang]")) texts.push(el.getAttribute("aria-label"));
    });
    const pressed = document.querySelector('.lang-option[aria-pressed="true"]');
    // The glyphs, not the element: a box taller than its line would line up
    // with the switch while the word itself sits higher.
    const wordmarkText = document.createRange();
    wordmarkText.selectNodeContents(document.querySelector(".wordmark"));
    const wordmark = wordmarkText.getBoundingClientRect();
    const pill = document.querySelector(".lang-switch").getBoundingClientRect();
    return {
        lang: document.documentElement.lang,
        plan: document.getElementById("planLine").textContent,
        lede: document.getElementById("lede").textContent,
        km: document.getElementById("sliderReadout").textContent,
        vdot: document.getElementById("vdotValue").textContent,
        error: ["distanceError", "timeError", "paceError"].map((id) => document.getElementById(id).textContent).join(" ").trim(),
        saved: [...document.querySelectorAll("#savedList li")].map((li) => li.querySelector(".saved-open").innerText.replace(/\\n/g, " | ")),
        savedVisible: !document.getElementById("savedSection").hidden,
        pressed: pressed && pressed.dataset.lang,
        options: [...document.querySelectorAll(".lang-option")].map((b) => b.textContent).join("/"),
        offset: Math.abs((wordmark.top + wordmark.bottom) / 2 - (pill.top + pill.bottom) / 2),
        stored: localStorage.getItem(${JSON.stringify(STORAGE_KEY)}),
        cyrillic: texts.join(" ").match(/[\\u0400-\\u04FF]+/g) || [],
        hscroll: document.documentElement.scrollWidth > document.documentElement.clientWidth
    };
})()`;

async function shot(name) {
    const { cssContentSize } = await send("Page.getLayoutMetrics");
    const { data } = await send("Page.captureScreenshot", {
        format: "png", captureBeyondViewport: true,
        clip: { x: 0, y: 0, width: cssContentSize.width, height: cssContentSize.height, scale: 1 }
    });
    writeFileSync(join(SHOTS, name + ".png"), Buffer.from(data, "base64"));
}

async function input(id, value) {
    await evaluate(`(() => { const el = document.getElementById(${JSON.stringify(id)}); el.value = ${JSON.stringify(value)}; el.dispatchEvent(new Event("input", {bubbles: true})); return true; })()`);
    await sleep(150);
}

async function clickLang(code) {
    await evaluate(`document.querySelector('.lang-option[data-lang="${code}"]').click(), true`);
    await sleep(200);
}

function checkConsole(scenario) {
    check(scenario, "console clean", consoleProblems.length === 0, consoleProblems);
}

const NBSP = "\u00a0";

try {
    // 1. Russian browser.
    {
        const s = "ru browser";
        await open({ acceptLanguage: "ru-RU,ru" });
        const r = await evaluate(REPORT);
        check(s, "html lang is ru", r.lang === "ru", r.lang);
        check(s, "plan line in Russian", r.plan === `Полумарафон за 1:25:39${NBSP}— это${NBSP}4:04 на километр`, r.plan);
        check(s, "km uses a decimal comma", r.km === `21,097${NBSP}км`, r.km);
        check(s, "RU/EN switch with RU pressed", r.options === "RU/EN" && r.pressed === "ru", [r.options, r.pressed]);
        check(s, "switch sits on the line of the wordmark", r.offset <= 1, r.offset);
        const share = new URL(await evaluate(`shareURL()`));
        check(s, "share link carries the plan and the language", share.searchParams.get("d") === "21097"
            && share.searchParams.get("t") === "1:25:39" && share.searchParams.get("l") === "ru", share.href);
        check(s, "no horizontal scroll", !r.hscroll);
        checkConsole(s);
        await shot("ru-light-390");
    }

    // 2. English browser, dark.
    {
        const s = "en browser";
        await open({ acceptLanguage: "en-US,en", scheme: "dark" });
        const r = await evaluate(REPORT);
        check(s, "html lang is en", r.lang === "en", r.lang);
        check(s, "no Cyrillic on the page", r.cyrillic.length === 0, r.cyrillic);
        check(s, "plan line in English", r.plan === `Half marathon in 1:25:39${NBSP}— that's${NBSP}4:04 per kilometer`, r.plan);
        check(s, "km uses a decimal point", r.km === `21.097${NBSP}km`, r.km);
        check(s, "VDOT uses a decimal point", /^\d+\.\d$/.test(r.vdot), r.vdot);
        check(s, "EN is pressed", r.pressed === "en", r.pressed);
        const share = new URL(await evaluate(`shareURL()`));
        check(s, "share link names English", share.searchParams.get("l") === "en", share.href);
        check(s, "no horizontal scroll", !r.hscroll);
        checkConsole(s);
        await shot("en-dark-390");
        const keys = await evaluate(`(() => {
            const ru = Object.keys(messages.ru), en = Object.keys(messages.en);
            return {onlyRu: ru.filter((k) => !en.includes(k)), onlyEn: en.filter((k) => !ru.includes(k))};
        })()`);
        check(s, "dictionaries have the same keys", keys.onlyRu.length === 0 && keys.onlyEn.length === 0, keys);
    }

    // 3. Unsupported browser language falls back to English; a later Russian preference wins.
    {
        await open({ acceptLanguage: "de-DE,de" });
        check("de browser", "falls back to English", (await evaluate(REPORT)).lang === "en");
        await open({ acceptLanguage: "de-DE,de,ru" });
        check("de,ru browser", "picks Russian from the list", (await evaluate(REPORT)).lang === "ru");
    }

    // 4. Switching by hand re-renders everything and is remembered.
    {
        const s = "switcher";
        await open({ acceptLanguage: "ru-RU,ru" });
        await clickLang("en");
        let r = await evaluate(REPORT);
        check(s, "switch to en: lang", r.lang === "en", r.lang);
        check(s, "switch to en: no Cyrillic", r.cyrillic.length === 0, r.cyrillic);
        check(s, "switch to en: stored", r.stored === "en", r.stored);
        await input("timeMinuteInput", "75");
        r = await evaluate(REPORT);
        check(s, "validation error in English", r.error !== "" && r.cyrillic.length === 0, r.error);
        await clickLang("ru");
        r = await evaluate(REPORT);
        check(s, "error follows the switch to ru", /[Ѐ-ӿ]/.test(r.error), r.error);
        await input("timeMinuteInput", "25");
        await clickLang("en");
        await input("timeHourInput", "2");
        r = await evaluate(REPORT);
        check(s, "calculator still recalculates", r.plan.startsWith("Half marathon in 2:25:39"), r.plan);
        await send("Page.reload");
        await waitFor(() => evaluate(`document.readyState === "complete" && document.body.dataset.state === "ready"`), 20000, "reload");
        r = await evaluate(REPORT);
        check(s, "choice survives a reload", r.lang === "en" && r.pressed === "en", [r.lang, r.pressed]);
        checkConsole(s);
    }

    // 5. Telegram's language_code decides inside the Mini App.
    {
        const s = "telegram en";
        await open({ acceptLanguage: "ru-RU,ru", hash: telegramHash("en") });
        await waitFor(() => evaluate(`!document.getElementById("savedSection").hidden`), 5000, "saved runs");
        const r = await evaluate(REPORT);
        check(s, "language_code en wins over the browser", r.lang === "en", r.lang);
        check(s, "greeting in English", r.lede.startsWith("Ann, "), r.lede);
        const share = await evaluate(`shareURL()`);
        check(s, "share link has no hash and names English", !share.includes("#")
            && !share.includes("tgWebAppData") && new URL(share).searchParams.get("l") === "en", share);
        check(s, "saved run in English", r.saved[0] === "Half marathon, 1:38:48 | pace 4:41 /km, September 10", r.saved);
        check(s, "no Cyrillic on the page", r.cyrillic.length === 0, r.cyrillic);
        check(s, "no horizontal scroll", !r.hscroll);
        checkConsole(s);
        await shot("tg-en-390");
    }

    // 6. A choice made by hand wins over Telegram's language_code.
    {
        const s = "telegram stored ru";
        await open({ acceptLanguage: "en-US,en", hash: telegramHash("en"), storage: "ru" });
        await waitFor(() => evaluate(`!document.getElementById("savedSection").hidden`), 5000, "saved runs");
        const r = await evaluate(REPORT);
        check(s, "stored choice wins", r.lang === "ru", r.lang);
        check(s, "saved run in Russian", r.saved[0] === "Полумарафон, 1:38:48 | темп 4:41 /км, 10 сентября", r.saved);
        check(s, "greeting in Russian", r.lede.startsWith("Ann, задайте"), r.lede);
        checkConsole(s);
    }

    // 7. Switching the language must not move the page: a translation is longer
    // or shorter, but the layout has to absorb that, not pass it on.
    {
        const s = "layout";
        for (const width of [390, 768, 1280]) {
            await open({ acceptLanguage: "ru-RU,ru", width, mobile: width < 700 });
            const before = await evaluate(`document.documentElement.scrollHeight`);
            await clickLang("en");
            await sleep(400);
            const after = await evaluate(`document.documentElement.scrollHeight`);
            check(s, `page height stays put at ${width} px`, before === after, { before, after });
        }
    }

    // 8. Desktop screenshots for a visual pass.
    {
        await open({ acceptLanguage: "en-US,en", width: 1280, mobile: false });
        check("en desktop", "no horizontal scroll", !(await evaluate(REPORT)).hscroll);
        await shot("en-light-1280");
        await open({ acceptLanguage: "ru-RU,ru", width: 1280, mobile: false, scheme: "dark" });
        await shot("ru-dark-1280");
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
