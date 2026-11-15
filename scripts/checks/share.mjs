// Exercise the existing share button with a local SDK/API stand-in; no Telegram messages are sent.
import { spawn } from "node:child_process";
import { mkdtempSync, rmSync, mkdirSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const ROOT = fileURLToPath(new URL("../../", import.meta.url));
const ASSETS = process.env.ASSETS || join(ROOT, "assets");
const CHROME = process.env.CHROME || "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
const HTTP_PORT = 8821;
const CDP_PORT = 9322;
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
    await send("Page.addScriptToEvaluateOnNewDocument", {source: `
        window.copied = []; window.prepared = []; window.sent = [];
        window.oldClient = location.search.includes("old=1");
        window.noSDK = location.search.includes("sdk=off");
        window.shareOff = location.search.includes("share=off");
        Object.defineProperty(navigator, "clipboard", {value: {writeText: async value => {
            if (window.failCopy) throw new Error("expected copy failure");
            copied.push(value);
        }}});
        const originalFetch = window.fetch;
        window.fetch = async (url, options = {}) => {
            if (url === "/api/v1/me") return new Response(JSON.stringify({user:{id:42,first_name:"Ann"}}));
            if (url === "/api/v1/runs") return new Response("{}",{status:503});
            if (url === "/api/v1/share") {
                prepared.push({body:JSON.parse(options.body),auth:options.headers.Authorization});
                if (window.shareOff) return new Response("{}",{status:503});
                if (window.failShare) return new Response("{}",{status:502});
                return new Response(JSON.stringify({id:"prepared-1",expiration_date:1900000000}));
            }
            return originalFetch(url, options);
        };
    `});
    await send("Fetch.enable", {patterns:[{urlPattern:"https://telegram.org/js/telegram-web-app.js*"}]});
    listeners.push(msg => {
        if (msg.method === "Fetch.requestPaused") send("Fetch.fulfillRequest", {
            requestId:msg.params.requestId,responseCode:200,
            responseHeaders:[{name:"Content-Type",value:"application/javascript"}],
            body:Buffer.from(`window.Telegram=window.noSDK ? null : {WebApp:{ready(){},expand(){},onEvent(){},themeParams:{},isVersionAtLeast(){return !window.oldClient},shareMessage(id,callback){sent.push(id);window.shareCallback=callback}}};`).toString("base64")
        });
    });
    await open("/?d=42195&t=3:30:00#tgWebAppData=fixture");
    await waitFor(() => evaluate(`document.getElementById("shareButton").textContent === "Поделиться"`), 5000, "share label");
    // Invalid edits must not alter the shared plan.
    await evaluate(`document.getElementById("distInput").value="0";document.getElementById("distInput").dispatchEvent(new Event("input",{bubbles:true}));document.getElementById("shareButton").click()`);
    await waitFor(() => evaluate(`sent.length === 1`),5000,"prepared share");
    let state = await evaluate(`({request:prepared[0],sent,disabled:document.getElementById("shareButton").disabled,hash:location.hash})`);
    check("Mini App prepares the shown plan with initData authorization",state.request.body.distance===42195 && state.request.body.time_seconds===12600 && state.request.auth==="tma fixture",state);
    check("Telegram receives the prepared id and duplicate clicks are blocked",state.sent[0]==="prepared-1" && state.disabled,state);
    await evaluate(`document.getElementById("shareButton").click();shareCallback(false)`);
    state=await evaluate(`({count:prepared.length,disabled:document.getElementById("shareButton").disabled,note:document.getElementById("shareNote").textContent})`);
    check("cancellation restores the button without copying",state.count===1 && !state.disabled && state.note.includes("не отправлен"),state);
    await evaluate(`document.getElementById("shareButton").click()`);
    await waitFor(() => evaluate(`sent.length===2`),5000,"second share");
    await evaluate(`shareCallback(true)`);
    check("success is announced",await evaluate(`document.getElementById("shareNote").textContent.includes("отправлен")`));
    await evaluate(`window.failShare=true;document.getElementById("shareButton").click()`);
    await waitFor(() => evaluate(`!document.getElementById("shareButton").disabled`),5000,"failure state");
    check("API failure neither opens Telegram nor silently copies",await evaluate(`sent.length===2 && copied.length===0 && document.getElementById("shareNote").textContent.includes("Не удалось")`));
    for (const theme of ["light","dark"]) {
        await send("Emulation.setEmulatedMedia",{features:[{name:"prefers-color-scheme",value:theme}]});
        check(theme+" Mini App layout fits",await evaluate(`document.documentElement.scrollWidth <= document.documentElement.clientWidth`));
        if (process.env.CHECK_OUT) {
            mkdirSync(process.env.CHECK_OUT,{recursive:true});
            const shot = await send("Page.captureScreenshot",{format:"png",captureBeyondViewport:true});
            writeFileSync(join(process.env.CHECK_OUT,`share-${theme}.png`),Buffer.from(shot.data,"base64"));
        }
    }
    await evaluate(`document.querySelector('[data-lang="en"]').click()`);
    check("Mini App share label follows language selection",await evaluate(`document.getElementById("shareButton").textContent === "Share"`));
    await evaluate(`document.querySelector('[data-lang="ru"]').click()`);
    // 503: sharing is not configured on the server, so the button falls back to copying.
    await open("/?d=42195&t=3:30:00&share=off#tgWebAppData=fixture");
    await waitFor(() => evaluate(`document.getElementById("shareButton").textContent === "Поделиться"`), 5000, "share label before 503");
    await evaluate(`document.getElementById("shareButton").click()`);
    await waitFor(() => evaluate(`copied.length===1`),5000,"copy after 503");
    state = await evaluate(`({prepared:prepared.length,sent:sent.length,copied,disabled:document.getElementById("shareButton").disabled,label:document.getElementById("shareButton").textContent,note:document.getElementById("shareNote").textContent})`);
    check("503 copies a safe plan link in the same click",state.prepared===1 && state.sent===0 && state.copied[0].includes("d=42195") && state.copied[0].includes("t=3%3A30%3A00") && !state.copied[0].includes("#") && !state.copied[0].includes("fixture") && state.note==="Ссылка скопирована." && !state.disabled,state);
    check("503 returns the button to copying",state.label==="Скопировать ссылку на расчёт",state);
    await evaluate(`document.getElementById("shareButton").click()`);
    await waitFor(() => evaluate(`copied.length===2`),5000,"second copy after 503");
    state = await evaluate(`({prepared:prepared.length,sent:sent.length})`);
    check("after 503 next clicks copy without asking the server",state.prepared===1 && state.sent===0,state);
    await evaluate(`document.querySelector('[data-lang="en"]').click()`);
    check("copy label after 503 follows language selection",await evaluate(`document.getElementById("shareButton").textContent === "Copy link to this run"`));
    await evaluate(`document.querySelector('[data-lang="ru"]').click()`);
    check("console clean after 503 fallback",consoleProblems.length===0,consoleProblems);
    await evaluate(`window.failCopy=true;document.getElementById("shareButton").click()`);
    await waitFor(() => evaluate(`document.getElementById("shareNote").textContent.startsWith("Не получилось скопировать")`),5000,"copy failure note");
    check("failed copy is logged to the console",consoleProblems.some((p) => p.includes("expected copy failure")),consoleProblems);
    await open("/?d=10000&t=50:00&old=1#tgWebAppData=fixture");
    check("old client retains the copy label",await evaluate(`document.getElementById("shareButton").textContent.includes("Скопировать")`));
    await evaluate(`document.getElementById("shareButton").click()`);
    await waitFor(() => evaluate(`copied.length===1`),5000,"old client copy");
    check("old clients copy a safe plan link",await evaluate(`!copied[0].includes("#") && !copied[0].includes("fixture") && prepared.length===0`));
    await open("/?d=10000&t=50:00&sdk=off#tgWebAppData=fixture");
    await evaluate(`document.getElementById("shareButton").click()`);
    await waitFor(() => evaluate(`copied.length===1`),5000,"missing SDK copy");
    check("Mini App without SDK keeps browser copying",await evaluate(`prepared.length===0 && !copied[0].includes("fixture")`));
    await open("/?d=42195&t=3:30:00&extra=private#private");
    await evaluate(`document.getElementById("shareButton").click()`);
    await waitFor(() => evaluate(`copied.length===1`),5000,"browser copy");
    check("browser keeps copying only d/t/l without hash or extra parameters",await evaluate(`copied[0].includes("d=42195") && copied[0].includes("l=ru") && !copied[0].includes("#") && !copied[0].includes("extra") && prepared.length===0`));
    check("browser console clean",consoleProblems.length===0,consoleProblems);
    for (const theme of ["light","dark"]) {
        await send("Emulation.setEmulatedMedia",{features:[{name:"prefers-color-scheme",value:theme}]});
        check(theme+" mobile layout fits",await evaluate(`document.documentElement.scrollWidth <= document.documentElement.clientWidth`));
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
