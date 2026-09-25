// Service worker (spec §9): keeps the app shell and open workouts available
// offline. Served at /sw.js (scope "/") with __VERSION__ replaced by a hash
// of the static files, so a new release replaces the cached shell.
const VERSION = "__VERSION__";
const SHELL = `shell-${VERSION}`;
const PAGES = "pages"; // live workout pages, kept across releases
const SHELL_FILES = [
  "/offline",
  "/static/app.css",
  "/static/vendor/htmx.min.js",
  "/static/vendor/uplot/uPlot.min.css",
  "/static/js/calc.js",
  "/static/js/chart-data.js",
  "/static/js/companion-core.js",
  "/static/js/companion.js",
  "/static/js/plan-form.js",
  "/static/js/plan-form-core.js",
  "/static/js/plan-form-theme.js",
  "/static/js/stats.js",
];

self.addEventListener("install", (event) => {
  // cache: "reload" skips the HTTP cache (static files are max-age=3600), so a
  // new release never stores the previous release's files.
  const fresh = SHELL_FILES.map((url) => new Request(url, { cache: "reload" }));
  event.waitUntil(caches.open(SHELL).then((c) => c.addAll(fresh)).then(() => self.skipWaiting()));
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches.keys()
      .then((keys) => Promise.all(keys.filter((k) => k.startsWith("shell-") && k !== SHELL).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  );
});

const isLive = (url) => /^\/sessions\/[^/]+\/live$/.test(url.pathname);

self.addEventListener("fetch", (event) => {
  const req = event.request;
  const url = new URL(req.url);
  if (req.method !== "GET" || url.origin !== location.origin) return;

  if (url.pathname.startsWith("/static/")) {
    // Cache first: static files are versioned with the shell.
    event.respondWith(caches.match(req).then((hit) => hit || fetch(req)));
    return;
  }
  if (req.mode === "navigate" && isLive(url)) {
    // Network first, so the page carries the latest sets; the copy serves offline reloads.
    event.respondWith(
      fetch(req)
        .then((res) => {
          if (res.ok) {
            const copy = res.clone();
            caches.open(PAGES).then((c) => c.put(url.pathname, copy));
          }
          return res;
        })
        .catch(() => caches.match(url.pathname, { cacheName: PAGES }).then((hit) => hit || caches.match("/offline"))),
    );
    return;
  }
  if (req.mode === "navigate") {
    event.respondWith(fetch(req).catch(() => caches.match("/offline")));
  }
  // Everything else (htmx requests, /api/*) goes to the network as usual.
});
