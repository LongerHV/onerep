// Tests for the service worker's install step, run in a sandbox with fake caches.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";

test("install fetches the shell past the HTTP cache", async () => {
  const src = readFileSync(new URL("../static/js/sw.js", import.meta.url), "utf8").replace("__VERSION__", "abc123");
  const handlers = {};
  const added = [];
  const sandbox = {
    self: { addEventListener: (type, fn) => (handlers[type] = fn), skipWaiting: () => Promise.resolve() },
    caches: { open: async () => ({ addAll: async (reqs) => added.push(...reqs) }) },
    Request: class { constructor(url, init = {}) { this.url = url; this.cache = init.cache; } },
    location: { origin: "https://gym.example" },
  };
  vm.runInNewContext(src, sandbox);
  let done;
  handlers.install({ waitUntil: (p) => (done = p) });
  await done;
  assert.ok(added.length > 0, "the shell is cached");
  // /static/* is served with max-age=3600: a plain fetch could fill the new
  // release's cache with the previous release's files.
  for (const r of added) assert.equal(typeof r === "string" ? "default" : r.cache, "reload", `${r.url || r} uses the HTTP cache`);
});
