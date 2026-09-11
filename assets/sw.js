// Офлайн на старте забега, где связь часто пропадает.
//
// Сначала сеть, потом кэш: после выкладки сразу грузится новая версия, а без
// сети — последняя сохранённая. Кэшируются только сама страница и её файлы;
// API, /healthz и чужие хосты (SDK Telegram) идут мимо.
const CACHE = "pacer-v1";
const SHELL = [
    "./",
    "wasm_exec.js",
    "json.wasm",
    "manifest.json",
    "icon.svg",
    "icon-192.png",
    "icon-512.png",
    "apple-touch-icon.png"
];

const scope = new URL("./", self.location);
const pageKey = scope.href;
const pagePaths = new Set([scope.pathname, scope.pathname + "index.html"]);
const shellPaths = new Set(SHELL.map((path) => new URL(path, scope).pathname));

self.addEventListener("install", (event) => {
    event.waitUntil(
        caches.open(CACHE)
            .then((cache) => cache.addAll(SHELL))
            .then(() => self.skipWaiting())
    );
});

self.addEventListener("activate", (event) => {
    event.waitUntil(
        caches.keys()
            .then((keys) => Promise.all(keys.filter((key) => key !== CACHE).map((key) => caches.delete(key))))
            .then(() => self.clients.claim())
    );
});

self.addEventListener("fetch", (event) => {
    const request = event.request;
    if (request.method !== "GET") {
        return;
    }

    const url = new URL(request.url);
    if (url.origin !== scope.origin) {
        return;
    }

    // Страница с любым ?d=&t= — одна запись: расчёт из адреса восстанавливает
    // сам скрипт. Навигация на другие адреса (например, открытый в браузере
    // /api/v1/pace) не должна затереть страницу в кэше.
    if (request.mode === "navigate" && pagePaths.has(url.pathname)) {
        event.respondWith(networkFirst(request, pageKey));
    } else if (shellPaths.has(url.pathname)) {
        event.respondWith(networkFirst(request, url.pathname));
    }
});

async function networkFirst(request, key) {
    const cache = await caches.open(CACHE);

    try {
        const response = await fetch(request);
        if (response.ok) {
            await cache.put(key, response.clone());
        }

        return response;
    } catch (error) {
        const cached = await cache.match(key);
        if (cached) {
            return cached;
        }

        throw error;
    }
}
