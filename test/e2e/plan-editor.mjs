// End-to-end check of the plan form editor: the starter template in the form,
// adding a set line, a per-week field following the plan's weeks, a server
// problem marked on its field, a draft round-tripping, and the JSON view
// refusing a document the form can't show.
import { setup } from "./harness.mjs";

const h = setup("plan-editor");
let b;
try {
  await h.startServer();
  b = await h.browser();
  const { send, evaluate } = b;
  const go = async (path) => {
    await send("Page.navigate", { url: h.BASE + path });
    await h.waitFor(() => evaluate("document.readyState === 'complete'"), `load of ${path}`);
  };
  const doc = () => evaluate("JSON.parse(document.getElementById('doc').value)");
  const setInput = (selector, value) => evaluate(`(() => { const el = document.querySelector(${JSON.stringify(selector)});
    el.value = ${JSON.stringify(value)}; el.dispatchEvent(new Event('change', {bubbles: true})); el.dispatchEvent(new Event('input', {bubbles: true})); })()`);

  await go("/plans/new");
  await h.waitFor(() => evaluate("!!document.querySelector('#doc-form.je-ready') && !document.getElementById('doc-form').hidden"), "the form");
  h.check("the starter template opens in the form",
    await evaluate(`document.querySelector('#doc-form input[name="root[name]"]').value === 'Upper/Lower starter'`));

  const setsBefore = (await doc()).days[0].groups[0].exercises[0].sets.length;
  await evaluate(`[...document.querySelectorAll('#doc-form button')].find(b => /^Add Set line/.test(b.textContent.trim())).click()`);
  await h.waitFor(async () => (await doc()).days[0].groups[0].exercises[0].sets.length === setsBefore + 1, "a new set line").catch(() => {});
  h.check("adding a set line updates the JSON", (await doc()).days[0].groups[0].exercises[0].sets.length === setsBefore + 1);
  // A new set line is blank until filled: give it a count and reps, or the plan can't be saved.
  const newLine = `root.days.0.groups.0.exercises.0.sets.${setsBefore}`;
  await setInput(`[data-per-week="${newLine}.count"] input:not([type=checkbox])`, "2");
  await setInput(`[data-per-week="${newLine}.reps"] input:not([type=checkbox])`, "8");

  const rest = "[data-per-week='root.days.0.groups.0.rest_s']";
  await evaluate(`document.querySelector("${rest} input[type=checkbox]").click()`);
  await setInput(`input[name="root[weeks]"]`, "5");
  await h.waitFor(async () => (await evaluate(`document.querySelectorAll("${rest} input:not([type=checkbox])").length`)) === 5, "five week inputs").catch(() => {});
  h.check("a varying field follows the plan's weeks", (await evaluate(`document.querySelectorAll("${rest} input:not([type=checkbox])").length`)) === 5);
  await setInput(`${rest} input:not([type=checkbox])`, "99999");
  await h.waitFor(async () => (await evaluate(`document.querySelector("${rest} p")?.textContent || ''`)).length > 0, "the rest problem").catch(() => {});
  h.check("a server problem is marked on its field", /W1|3600|maximum/i.test(await evaluate(`document.querySelector("${rest} p")?.textContent || ''`)));

  await setInput(`${rest} input:not([type=checkbox])`, "180");
  const noErrors = `(JSON.parse(document.getElementById("plan-problems")?.textContent || "[]") || []).every((p) => p.warning)`;
  await h.waitFor(() => evaluate(noErrors), "a preview without errors");
  const saved = await doc();
  await evaluate(`[...document.querySelectorAll('button')].find(b => b.textContent.trim() === 'Save as draft').click()`);
  await h.waitFor(() => evaluate("/^\\/plans\\/[0-9a-f-]{36}$/.test(location.pathname)"), "the plan page");
  const edit = await evaluate(`[...document.querySelectorAll('a')].find(a => /\\/edit(\\?from=|$)/.test(a.getAttribute('href') || ''))?.getAttribute('href')`);
  await go(edit);
  await h.waitFor(() => evaluate("!!document.querySelector('#doc-form.je-ready')"), "the form again");
  const norm = (d) => JSON.stringify(d, (k, v) => (v && typeof v === "object" && !Array.isArray(v) ? Object.fromEntries(Object.entries(v).sort()) : v));
  h.check("a saved draft reopens with the same plan", norm(await doc()) === norm({ unit: "kg", ...saved }));

  await evaluate(`document.querySelector('[data-view="json"]').click()`);
  await evaluate(`(() => { const t = document.getElementById('doc'); const d = JSON.parse(t.value); d.surprise = 1; t.value = JSON.stringify(d); })()`);
  await evaluate(`document.querySelector('[data-view="form"]').click()`);
  h.check("the form refuses a plan it can't show",
    await evaluate(`!document.getElementById('doc').hidden && !document.querySelector('[data-view-note]').hidden`));
} catch (err) {
  h.check("scenario ran to the end", false, err.stack || String(err));
} finally {
  await h.finish(b);
}
