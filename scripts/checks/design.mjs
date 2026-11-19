// Headless Chrome check of the page's own design pieces: the bundled fonts,
// the road on the distance slider, the footer links and the scoreboard band
// under a Telegram theme. Usage: node scripts/checks/design.mjs
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
const HTTP_PORT = 8818;
const CDP_PORT = 9318;
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
let requests = [];
const fontResponses = [];
listeners.push((msg) => {
    if (msg.method === "Runtime.consoleAPICalled" && ["error", "warning", "assert"].includes(msg.params.type)) {
        consoleProblems.push(msg.params.args.map((a) => a.value ?? a.description).join(" "));
    }
    if (msg.method === "Runtime.exceptionThrown") {
        consoleProblems.push(msg.params.exceptionDetails.exception?.description || msg.params.exceptionDetails.text);
    }
    if (msg.method === "Network.requestWillBeSent") {
        requests.push(msg.params.request.url);
    }
    if (msg.method === "Network.responseReceived" && msg.params.response.url.includes("/fonts/")) {
        fontResponses.push({ url: msg.params.response.url, status: msg.params.response.status });
    }
});

// An empty Telegram SDK and a server without history, so nothing leaves the machine.
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
    return reply(503, "{}", "application/json");
});

await send("Runtime.enable");
await send("Page.enable");
await send("Network.enable");
await send("Fetch.enable", { patterns: [{ urlPattern: "*/api/v1/*" }, { urlPattern: "*telegram.org*" }] });

let prep = 0;
async function open({ hash = "", width = 390, scheme = "light", acceptLanguage = "ru-RU,ru" }) {
    await send("Emulation.setUserAgentOverride", { userAgent: "Mozilla/5.0 Chrome", acceptLanguage });
    await send("Emulation.setDeviceMetricsOverride", { width, height: 900, deviceScaleFactor: 1, mobile: width < 700 });
    await send("Emulation.setEmulatedMedia", { features: [{ name: "prefers-color-scheme", value: scheme }] });
    await send("Page.navigate", { url: `${BASE}/index.html?prep=${++prep}` });
    await waitFor(() => evaluate(`document.readyState === "complete"`), 15000, "prep load");
    await evaluate(`localStorage.clear(), true`);
    consoleProblems = [];
    requests = [];
    await send("Page.navigate", { url: `${BASE}/?d=21097&t=1:38:48${hash}` });
    await waitFor(() => evaluate(`document.readyState === "complete" && document.body.dataset.state === "ready"`), 20000, "wasm ready");
    await evaluate(`document.fonts.ready.then(() => true)`);
    await sleep(300);
}

// Colours as the screen shows them: any CSS colour (oklch, color-mix) is
// painted on a canvas and read back as sRGB.
const COLOR_TOOLS = `
    window.__rgb = (css) => {
        const c = document.createElement("canvas"); c.width = c.height = 1;
        const g = c.getContext("2d"); g.fillStyle = css; g.fillRect(0, 0, 1, 1);
        return [...g.getImageData(0, 0, 1, 1).data.slice(0, 3)];
    };
    window.__lum = ([r, g, b]) => {
        const f = (v) => { v /= 255; return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4; };
        return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b);
    };
    window.__contrast = (a, b) => {
        const [x, y] = [__lum(a), __lum(b)].sort((p, q) => q - p);
        return (x + 0.05) / (y + 0.05);
    };
    true`;

// The footer is a service band: its own background step across the whole
// viewport, no rule on top, smaller and muted text that still reads.
const FOOTER_BAND = `(() => {
    const footer = document.querySelector("footer");
    const style = getComputedStyle(footer);
    const band = __rgb(style.backgroundColor);
    const paper = __rgb(getComputedStyle(document.body).backgroundColor);
    const rect = footer.getBoundingClientRect();
    const links = [...footer.querySelectorAll("a")];
    // The last content of the last shown section, not the section's own padding.
    const shown = (el) => [...el.children].filter((child) => child.getBoundingClientRect().height > 0);
    const section = shown(document.querySelector("main")).pop();
    return {
        band, paper, step: __contrast(band, paper),
        fullWidth: Math.abs(rect.width - document.documentElement.clientWidth) < 1 && Math.abs(rect.left) < 1,
        rule: style.borderTopStyle !== "none" && parseFloat(style.borderTopWidth) > 0,
        innerRule: [...footer.querySelectorAll("*")].some((el) => { const s = getComputedStyle(el); return s.borderTopStyle !== "none" && parseFloat(s.borderTopWidth) > 0; }),
        size: parseFloat(getComputedStyle(footer.querySelector("p")).fontSize),
        bodySize: parseFloat(getComputedStyle(document.body).fontSize),
        text: __contrast(__rgb(getComputedStyle(footer.querySelector("p")).color), band),
        links: links.map((a) => __contrast(__rgb(getComputedStyle(a).color), band)),
        underline: links.every((a) => getComputedStyle(a).textDecorationLine.includes("underline")),
        gap: Math.round(rect.top - Math.max(...shown(section).map((el) => el.getBoundingClientRect().bottom)))
    };
})()`;

// The road is read from a real screenshot: the pixels of the slider row are
// drawn into a canvas inside the page and sampled there.
async function roadPixels() {
    const box = await evaluate(`(() => { const r = document.getElementById("slider").getBoundingClientRect(); return {x: r.left + scrollX, y: r.top + scrollY, w: r.width, h: r.height}; })()`);
    const { data } = await send("Page.captureScreenshot", { format: "png", captureBeyondViewport: true, clip: { x: box.x, y: box.y, width: box.w, height: box.h, scale: 1 } });
    return evaluate(`new Promise((resolve) => {
        const img = new Image();
        img.onload = () => {
            const c = document.createElement("canvas"); c.width = img.width; c.height = img.height;
            const g = c.getContext("2d"); g.drawImage(img, 0, 0);
            const px = (x, y) => [...g.getImageData(Math.round(x), Math.round(y), 1, 1).data.slice(0, 3)];
            const slider = document.getElementById("slider");
            const fraction = slider.value / slider.max;
            const centre = 12 + (img.width - 24) * fraction;
            const mid = img.height / 2;
            // Dashes run 14 px on and 10 px off from 12 px in; sample the middle of whole dashes.
            const dashes = [];
            for (let x = 12 + 7; x < img.width - 12; x += 24) dashes.push(x);
            const behind = dashes.filter((x) => x < centre - 20);
            const ahead = dashes.filter((x) => x > centre + 20);
            resolve({
                width: img.width, centre,
                behind: behind.map((x) => px(x, mid)),
                ahead: ahead.map((x) => px(x, mid)),
                thumb: px(centre, mid),
                edge: px(centre + 40, mid - 12 + 4),
                asphalt: px(centre + 40, mid - 6),
                strip: __rgb(getComputedStyle(document.querySelector(".strip")).backgroundColor)
            });
        };
        img.src = "data:image/png;base64,${data}";
    })`);
}

const isMark = ([r, g, b]) => r > 220 && g > 180 && b < 90;
const isDimmed = ([r, g, b]) => r < 170 && r > b + 15 && !isMark([r, g, b]);
const brightness = ([r, g, b]) => 0.2126 * r + 0.7152 * g + 0.0722 * b;

try {
    // 1. Fonts come from the page itself, and nothing else leaves the origin.
    {
        const s = "fonts";
        await open({});
        await evaluate(COLOR_TOOLS);
        const faces = await evaluate(`(() => {
            const loaded = [...document.fonts].filter((f) => f.status === "loaded").map((f) => f.family.replace(/"/g, ""));
            const family = (sel) => getComputedStyle(document.querySelector(sel)).fontFamily.split(",")[0].replace(/"/g, "").trim();
            return {
                loaded: [...new Set(loaded)].sort(),
                failed: [...document.fonts].filter((f) => f.status === "error").length,
                plan: family("#planLine"), body: family("body"), splits: family(".band-at"), clock: family("#timeHourInput")
            };
        })()`);
        check(s, "display, text and mono faces are loaded", ["Fira Sans Extra Condensed", "JetBrains Mono", "Onest"].every((f) => faces.loaded.includes(f)) && faces.failed === 0, faces);
        check(s, "heads and clock use the condensed display face", faces.plan === "Fira Sans Extra Condensed" && faces.clock === "Fira Sans Extra Condensed", faces);
        check(s, "text uses Onest and splits use JetBrains Mono", faces.body === "Onest" && faces.splits === "JetBrains Mono", faces);
        const fonts = fontResponses.filter((r) => r.url.startsWith(BASE));
        const cyrillic = ["fira-sans-extra-condensed-800-cyrillic.woff2", "onest-cyrillic.woff2"];
        check(s, "a Russian page loads the Cyrillic files of the display and text faces", cyrillic.every((name) => fonts.some((r) => r.url.endsWith("/fonts/" + name))), fonts);
        check(s, "font files are served by the page's own origin", fonts.length > 0 && fonts.every((r) => r.status === 200 || r.status === 304), fontResponses);
        const external = requests.filter((url) => !url.startsWith(BASE) && !/^(data|blob):/.test(url));
        check(s, "the page makes no external requests", external.length === 0, external);
        check(s, "console clean", consoleProblems.length === 0, consoleProblems);
    }

    // 2. The slider is the road from the icon: bright dashes behind the thumb,
    // dimmed dashes ahead, light edges over the asphalt, a yellow thumb.
    for (const { width, scheme } of [{ width: 390, scheme: "light" }, { width: 1440, scheme: "light" }, { width: 390, scheme: "dark" }, { width: 1440, scheme: "dark" }]) {
        const s = `road ${width} ${scheme}`;
        await open({ width, scheme });
        await evaluate(COLOR_TOOLS);
        const r = await roadPixels();
        check(s, "dashes behind the thumb are bright yellow", r.behind.length > 0 && r.behind.every(isMark), r.behind);
        check(s, "dashes ahead are dimmed, not gone", r.ahead.length > 0 && r.ahead.every(isDimmed), r.ahead);
        check(s, "the thumb is yellow", isMark(r.thumb), r.thumb);
        check(s, "the road edge is lighter than the asphalt", brightness(r.edge) > brightness(r.asphalt) + 20, { edge: r.edge, asphalt: r.asphalt });
        check(s, "the asphalt is not lighter than the band", brightness(r.asphalt) <= brightness(r.strip) + 2, { asphalt: r.asphalt, strip: r.strip });
        await send("Page.captureScreenshot", { format: "png" }).then(({ data }) => writeFileSync(join(SHOTS, `design-road-${width}-${scheme}.png`), Buffer.from(data, "base64")));
    }

    // 3. Moving the thumb moves the bright part with it.
    {
        const s = "road follows the thumb";
        await open({ width: 390 });
        await evaluate(COLOR_TOOLS);
        await evaluate(`(() => { const el = document.getElementById("b100mile"); el.click(); return true; })()`);
        await sleep(200);
        const r = await roadPixels();
        check(s, "after 100 miles most dashes are bright", r.behind.length > r.ahead.length && r.behind.every(isMark) && r.ahead.every(isDimmed), { behind: r.behind.length, ahead: r.ahead.length });
    }

    // 4. Footer: the author and the source, in both languages.
    {
        const s = "footer";
        await open({ width: 390 });
        const read = `(() => {
            const footer = document.querySelector("footer");
            const author = footer && footer.querySelector('a[href="https://github.com/proshik"]');
            const source = footer && footer.querySelector('a[href="https://github.com/proshik/pacer"]');
            return {text: footer && footer.innerText.replace(/\\s+/g, " ").trim(), author: author && author.textContent, source: source && source.textContent};
        })()`;
        let f = await evaluate(read);
        check(s, "author links to GitHub", f.author === "proshik", f);
        for (const scheme of ["light", "dark"]) {
            await open({ width: scheme === "light" ? 390 : 1440, scheme });
            await evaluate(COLOR_TOOLS);
            const b = await evaluate(FOOTER_BAND);
            check(s, `${scheme}: the footer is a full-width band of its own colour`, b.fullWidth && b.band.join() !== b.paper.join() && b.step >= 1.05, b);
            check(s, `${scheme}: no rule on top of the footer`, !b.rule && !b.innerRule, b);
            check(s, `${scheme}: footer text is smaller than the body text`, b.size < b.bodySize, b);
            check(s, `${scheme}: footer text and links read on the band`, b.text >= 4.5 && b.links.length === 2 && b.links.every((v) => v >= 4.5) && b.underline, b);
            check(s, `${scheme}: 48 px between the last section and the band`, b.gap === 48, b.gap);
        }
        await open({ width: 390 });
        f = await evaluate(read);
        check(s, "source code link in Russian", f.source === "Исходный код", f);
        const year = await evaluate(`new Date().getFullYear()`);
        const line = year > 2020 ? `© 2020–${year}, proshik` : "© 2020, proshik";
        check(s, "copyright range ends with the current year", f.text === `${line} Исходный код`, { f, year });
        // The page without JS reads the range from the markup.
        const plain = await evaluate(`fetch("/index.html").then((r) => r.text()).then((html) => {
            const footer = new DOMParser().parseFromString(html, "text/html").querySelector("footer p");
            return footer && footer.textContent.replace(/\\s+/g, " ").trim();
        })`);
        check(s, "markup without JS carries a whole range", /^© 2020–\d{4}, proshik$/.test(plain || ""), plain);
        const single = await evaluate(`[messages.ru.copyright(2020), messages.en.copyright(2020), messages.en.copyright(2031)]`);
        check(s, "in 2020 the line is a single year", single[0] === "© 2020," && single[1] === "© 2020," && single[2] === "© 2020–2031,", single);
        await evaluate(`document.querySelector('.lang-option[data-lang="en"]').click(), true`);
        await sleep(200);
        f = await evaluate(read);
        check(s, "source code link in English", f.source === "Source code" && f.author === "proshik", f);
        check(s, "copyright range in English ends with the current year", f.text === `${line} Source code`, f);
    }

    // 5. The logo leads to the home page; inside the Mini App it keeps the
    // launch hash, or the next page would lose the sign-in and the theme.
    {
        const s = "logo link";
        await open({ width: 390 });
        const read = `(() => {
            const link = document.querySelector("a.wordmark");
            if (!link) return null;
            const style = getComputedStyle(link);
            return {href: link.getAttribute("href"), url: link.href, label: link.getAttribute("aria-label"),
                underline: style.textDecorationLine, text: link.textContent.trim()};
        })()`;
        let l = await evaluate(read);
        check(s, "the logo is a link to the site root", l && l.href === "./" && l.url === `${BASE}/`, l);
        check(s, "the link is named in Russian and is not underlined", l && l.label === "Pacer, на главную" && l.underline === "none" && l.text === "Pacer", l);
        await evaluate(`document.querySelector('.lang-option[data-lang="en"]').click(), true`);
        await sleep(200);
        l = await evaluate(read);
        check(s, "the link is named in English", l && l.label === "Pacer, home", l);

        const user = JSON.stringify({ id: 7, first_name: "Ann", language_code: "ru" });
        const initData = new URLSearchParams({ query_id: "AAE", user, auth_date: "1757500000", hash: "0".repeat(64) }).toString();
        const hash = "#" + new URLSearchParams({ tgWebAppData: initData, tgWebAppVersion: "8.0" }).toString();
        await open({ width: 390, hash });
        l = await evaluate(read);
        check(s, "in the Mini App the link keeps the launch hash", l && l.url === `${BASE}/${hash}`, l);
        const share = await evaluate(`shareURL()`);
        check(s, "the share link still has no hash", !share.includes("#") && !share.includes("tgWebAppData"), share);
        // The page writes the plan back into the address after it loads, so a
        // marker on the old window tells the new page from the old one.
        await evaluate(`window.__beforeHome = true, document.querySelector("a.wordmark").click(), true`);
        await waitFor(() => evaluate(`!window.__beforeHome && document.readyState === "complete" && document.body.dataset.state === "ready"`), 20000, "home page");
        await waitFor(() => evaluate(`document.getElementById("lede").textContent.startsWith("Ann, ")`), 5000, "greeting");
        const after = await evaluate(`({hash: location.hash, lede: document.getElementById("lede").textContent})`);
        check(s, "following it keeps the Mini App sign-in", after.hash === hash && after.lede.startsWith("Ann, "), after);
    }

    // 6. In Telegram the neutrals follow the theme, the band keeps its own colours.
    const themes = {
        light: { bg_color: "#ffffff", secondary_bg_color: "#f1f1f1", text_color: "#000000", hint_color: "#999999", section_separator_color: "#e7e7e7" },
        dark: { bg_color: "#17212b", secondary_bg_color: "#232e3c", text_color: "#f5f5f5", hint_color: "#708499", section_separator_color: "#111921" }
    };
    const plainStrip = {};
    for (const scheme of ["light", "dark"]) {
        await open({ width: 390, scheme });
        await evaluate(COLOR_TOOLS);
        plainStrip[scheme] = await evaluate(`__rgb(getComputedStyle(document.querySelector(".strip")).backgroundColor)`);
    }
    for (const [name, theme] of Object.entries(themes)) {
        for (const scheme of ["light", "dark"]) {
            const s = `telegram ${name} theme, ${scheme} system`;
            const user = JSON.stringify({ id: 7, first_name: "Ann", language_code: "ru" });
            const initData = new URLSearchParams({ query_id: "AAE", user, auth_date: "1757500000", hash: "0".repeat(64) }).toString();
            const hash = "#" + new URLSearchParams({ tgWebAppData: initData, tgWebAppVersion: "8.0", tgWebAppThemeParams: JSON.stringify(theme) }).toString();
            await open({ width: 390, scheme, hash });
            await evaluate(COLOR_TOOLS);
            const c = await evaluate(`(() => {
                const color = (sel, prop) => __rgb(getComputedStyle(document.querySelector(sel))[prop]);
                const strip = color(".strip", "backgroundColor");
                const pairs = {
                    digits: __contrast(color("#timeHourInput", "color"), strip),
                    derived: __contrast(color("#paceMinuteInput", "color"), strip),
                    label: __contrast(color("#distance-title", "color"), strip),
                    preset: __contrast(color("#b5km", "color"), strip),
                    split: __contrast(color(".band-mark", "color"), strip),
                    note: __contrast(color("#shareNote", "color"), strip)
                };
                // Muted text on the paper takes Telegram's hint colour as its base.
                const paper = color("body", "backgroundColor");
                const surface = color(".forecast-table tr.is-current th", "backgroundColor");
                const onPaper = {
                    lede: __contrast(color("#lede", "color"), paper),
                    note: __contrast(color(".forecast .section-note", "color"), paper),
                    head: __contrast(color(".forecast-table thead th", "color"), paper),
                    hint: __contrast(color(".zone-hint", "color"), paper),
                    ownRow: __contrast(color(".current-note", "color"), surface)
                };
                return {paper, strip, pairs, onPaper};
            })()`);
            check(s, "the paper takes Telegram's background", c.paper.join() === __hex(theme.bg_color).join(), c.paper);
            check(s, "the band keeps its own colour", c.strip.join() === plainStrip[scheme].join(), { strip: c.strip, plain: plainStrip[scheme] });
            check(s, "every text on the band stays readable", Object.values(c.pairs).every((v) => v >= 4.5), c.pairs);
            check(s, "muted text on the paper stays readable", Object.values(c.onPaper).every((v) => v >= 4.5), c.onPaper);
            const band = await evaluate(FOOTER_BAND);
            check(s, "the footer band takes secondary_bg_color and its text reads", band.band.join() === __hex(theme.secondary_bg_color).join()
                && band.band.join() !== band.paper.join() && band.text >= 4.5 && band.links.every((v) => v >= 4.5), band);
            check(s, "no horizontal scroll", !(await evaluate(`document.documentElement.scrollWidth > document.documentElement.clientWidth`)));
            check(s, "console clean", consoleProblems.length === 0, consoleProblems);
            const { data } = await send("Page.captureScreenshot", { format: "png", captureBeyondViewport: true });
            writeFileSync(join(SHOTS, `design-telegram-${name}-${scheme}.png`), Buffer.from(data, "base64"));
        }
    }
} catch (error) {
    check("runner", "no exception", false, String(error && error.stack || error));
} finally {
    clearTimeout(hardStop);
    ws.close();
    cleanup();
}

function __hex(value) {
    const n = parseInt(value.slice(1), 16);
    return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
}

const failed = results.filter((r) => !r.ok).length;
console.log(`\n${results.length - failed}/${results.length} checks passed`);
process.exit(failed ? 1 : 0);
