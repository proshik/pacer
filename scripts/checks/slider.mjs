// Headless Chrome check of the distance slider: a fine scale up to 50 km on
// the first two thirds of the track, a fast one up to 500 km on the rest.
import { spawn } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const ROOT = fileURLToPath(new URL("../../", import.meta.url));
const ASSETS = process.env.ASSETS || join(ROOT, "assets");
const CHROME = process.env.CHROME || "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
const HTTP_PORT = 8818;
const CDP_PORT = 9319;
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

// Moving the thumb: the page listens to "input", as it does for a real drag.
const drag = (fraction) => evaluate(`(() => {
    const slider = document.getElementById("slider");
    slider.value = Number(slider.min) + (Number(slider.max) - Number(slider.min)) * ${fraction};
    slider.dispatchEvent(new Event("input", {bubbles: true}));
    return true;
})()`);
const press = (key) => evaluate(`(() => {
    const slider = document.getElementById("slider");
    slider.dispatchEvent(new KeyboardEvent("keydown", {key: ${JSON.stringify(key)}, bubbles: true, cancelable: true}));
    return true;
})()`);
const type = (meters) => evaluate(`(() => {
    const input = document.getElementById("distInput");
    input.value = ${JSON.stringify(String(meters))};
    input.dispatchEvent(new Event("input", {bubbles: true}));
    return true;
})()`);
const STATE = `(() => {
    const slider = document.getElementById("slider");
    return {
        meters: Number(document.getElementById("distInput").value),
        fraction: (Number(slider.value) - Number(slider.min)) / (Number(slider.max) - Number(slider.min)),
        readout: document.getElementById("sliderReadout").textContent,
        valuetext: slider.getAttribute("aria-valuetext"),
        distanceError: document.getElementById("distanceError").textContent
    };
})()`;
const state = async () => { await sleep(120); return evaluate(STATE); };
const near = (a, b, eps = 0.002) => Math.abs(a - b) <= eps;

try {
    await send("Page.navigate", { url: `${BASE}/` });
    await waitFor(() => evaluate(`document.body.dataset.state === "ready"`), 20000, "ready");
    await sleep(300);

    // The scale.
    await drag(2 / 3);
    let s = await state();
    check("two thirds of the track is 50 km", s.meters === 50000 && s.readout === "50\u00a0км", s);

    await drag(0.2);
    s = await state();
    check("below 50 km the scale is even and in 100 m steps", s.meters === 15000 && s.meters % 100 === 0, s);
    await drag(0.2137);
    s = await state();
    check("a value between steps snaps to 100 m", s.meters % 100 === 0 && s.meters > 15000 && s.meters < 16500, s);

    await drag(5 / 6);
    s = await state();
    check("above 50 km the scale is fast and in 1 km steps", s.meters === 275000, s);
    await drag(0.8512);
    s = await state();
    check("a value above 50 km snaps to 1 km", s.meters % 1000 === 0 && s.meters > 275000, s);

    await drag(1);
    s = await state();
    check("the far end is 500 km", s.meters === 500000, s);

    await drag(0);
    s = await state();
    check("the near end is 100 m, not an invalid zero", s.meters === 100 && s.distanceError === "", s);

    // Precision where people run: at 390 px no more than 250 m per pixel.
    const perPixel = await evaluate(`(() => {
        const width = document.getElementById("slider").getBoundingClientRect().width;
        return 50000 / ((width - 24) * 2 / 3);
    })()`);
    check("below 50 km one pixel is at most 250 m", perPixel <= 250, perPixel);

    // The thumb follows the distance from anywhere else.
    await type(42195);
    s = await state();
    check("a typed marathon puts the thumb at its place", near(s.fraction, 42195 / 50000 * 2 / 3), s);
    await type(160934);
    s = await state();
    check("a typed 100 miles puts the thumb in the fast part", near(s.fraction, 2 / 3 + (160934 - 50000) / 450000 / 3), s);
    await type(700000);
    s = await state();
    check("a distance past 500 km pins the thumb to the end and keeps the value", s.fraction === 1 && s.readout === "700\u00a0км", s);

    await evaluate(`document.getElementById("b100mile").click(), true`);
    s = await state();
    check("a preset moves the thumb to its distance", s.meters === 160934 && near(s.fraction, 2 / 3 + (160934 - 50000) / 450000 / 3), s);

    // The keyboard steps by the step of the part it is in.
    await type(10000);
    await press("ArrowRight");
    s = await state();
    check("an arrow adds 100 m below 50 km", s.meters === 10100, s);
    await press("PageUp");
    s = await state();
    check("PageUp adds ten steps", s.meters === 11100, s);
    await type(50000);
    await press("ArrowRight");
    s = await state();
    check("an arrow adds 1 km from 50 km up", s.meters === 51000, s);
    await press("ArrowLeft");
    await press("ArrowLeft");
    s = await state();
    check("arrows step back through the knee: 1 km, then 100 m", s.meters === 49900, s);
    await press("End");
    s = await state();
    check("End goes to 500 km", s.meters === 500000, s);

    // A screen reader hears the distance, in the page's language.
    await type(21097);
    s = await state();
    check("the slider announces kilometres", s.valuetext === s.readout && s.valuetext === "21,097\u00a0км", s);
    await evaluate(`document.querySelector('.lang-option[data-lang="en"]').click(), true`);
    s = await state();
    check("the announcement follows the language", s.valuetext === "21.097\u00a0km", s);

    // The knee is marked on the track, under the thumb's position for 50 km.
    await type(50000);
    const knee = await evaluate(`(() => {
        const tick = document.getElementById("sliderKnee");
        if (!tick) return null;
        const slider = document.getElementById("slider").getBoundingClientRect();
        const box = tick.getBoundingClientRect();
        const thumbCentre = slider.left + 12 + (slider.width - 24) * 2 / 3;
        return {label: tick.textContent.trim(), offset: Math.abs(box.left + box.width / 2 - thumbCentre)};
    })()`);
    check("the 50 km knee is marked under the thumb", knee && knee.label === "50\u00a0km" && knee.offset <= 2, knee);

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
