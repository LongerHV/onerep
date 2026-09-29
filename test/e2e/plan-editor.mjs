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
  // Review fixes. Each starts from a fresh editor.
  const fresh = async () => {
    await go("/plans/new");
    await h.waitFor(() => evaluate("!!document.querySelector('#doc-form.je-ready')"), "a fresh form");
    await evaluate(`document.querySelector('[data-view="form"]').click()`); // the last view is remembered
    await h.waitFor(() => evaluate("!document.getElementById('doc-form').hidden"), "the form view");
  };
  const toJSONAndBack = async (mutate) => {
    await evaluate(`document.querySelector('[data-view="json"]').click()`);
    await evaluate(`(() => { const t = document.getElementById('doc'); const d = JSON.parse(t.value); (${mutate})(d); t.value = JSON.stringify(d); })()`);
    await evaluate(`document.querySelector('[data-view="form"]').click()`);
  };

  // Plan strings (from the AI or pasted JSON) must never run as HTML.
  await fresh();
  const payload = '<img src=x onerror="window.__xss = 1">';
  await toJSONAndBack(`(d) => { d.name = ${JSON.stringify(payload)}; d.days[0].groups[0].exercises[0].notes = ${JSON.stringify(payload)}; }`);
  await h.sleep(500);
  h.check("plan strings are never parsed as HTML", !(await evaluate("window.__xss")) &&
    (await evaluate(`document.querySelector('#doc-form input[name="root[name]"]')?.value`)) === payload);

  // Clearing Weeks to retype it must not collapse progressions.
  await fresh();
  await setInput(`input[name="root[weeks]"]`, "");
  await h.sleep(300);
  await setInput(`input[name="root[weeks]"]`, "4");
  await h.sleep(300);
  h.check("clearing weeks keeps per-week values", JSON.stringify((await doc()).days[0].groups[0].exercises[0].sets[1].count) === "[3,3,4,2]");

  // The last edit reaches the server even when Save follows at once (a tap).
  await fresh();
  await evaluate(`(() => { const i = document.querySelector('#doc-form input[name="root[name]"]'); i.value = "Tapped plan";
    i.dispatchEvent(new Event("change", {bubbles: true}));
    [...document.querySelectorAll("button")].find((b) => b.textContent.trim() === "Save as draft").click(); })()`);
  await h.waitFor(() => evaluate("/^\\/plans\\/[0-9a-f-]{36}$/.test(location.pathname)"), "the saved plan").catch(() => {});
  h.check("an edit followed at once by Save is saved", (await evaluate("document.body.textContent")).includes("Tapped plan"));

  // Enter in a form field doesn't submit the plan behind your back.
  await fresh();
  await evaluate(`document.querySelector('#doc-form input[name="root[name]"]').focus()`);
  await send("Input.dispatchKeyEvent", { type: "keyDown", key: "Enter", code: "Enter", windowsVirtualKeyCode: 13, text: "\r" });
  await send("Input.dispatchKeyEvent", { type: "keyUp", key: "Enter", code: "Enter", windowsVirtualKeyCode: 13 });
  await h.sleep(800);
  h.check("Enter in a form field doesn't save", (await evaluate("location.pathname")) === "/plans/new");

  // Back to the editor shows one form.
  await fresh();
  await evaluate(`document.querySelector('a[href="/plans"]').click()`);
  await h.waitFor(() => evaluate("location.pathname === '/plans'"), "the plans page");
  await evaluate("history.back()");
  await h.waitFor(() => evaluate("location.pathname === '/plans/new'"), "back to the editor");
  await h.sleep(1500);
  h.check("Back shows one form", (await evaluate(`document.querySelectorAll('input[name="root[name]"]').length`)) === 1);

  // Day tabs work from the keyboard.
  await fresh();
  await evaluate(`document.querySelectorAll('#doc-form [role=tab]')[1].focus()`);
  await send("Input.dispatchKeyEvent", { type: "keyDown", key: "Enter", code: "Enter", windowsVirtualKeyCode: 13, text: "\r" });
  await send("Input.dispatchKeyEvent", { type: "keyUp", key: "Enter", code: "Enter", windowsVirtualKeyCode: 13 });
  await h.sleep(300);
  h.check("day tabs open from the keyboard",
    await evaluate(`(() => { const i = document.querySelector('#doc-form input[name="root[days][1][name]"]'); return !!i && i.offsetParent !== null && i.value === "Lower"; })()`));
  // Alternatives keep their order; a new pick goes to the end.
  await fresh();
  const alts = `[data-slug-list="root.days.0.groups.0.exercises.0.alternatives"]`;
  await evaluate(`(() => { const s = document.querySelector('${alts} select'); s.value = "barbell-back-squat"; s.dispatchEvent(new Event("change", {bubbles: true})); })()`);
  await h.waitFor(async () => ((await doc()).days[0].groups[0].exercises[0].alternatives || []).length === 2, "the added alternative").catch(() => {});
  h.check("alternatives keep their order", JSON.stringify((await doc()).days[0].groups[0].exercises[0].alternatives) === '["dumbbell-bench-press","barbell-back-squat"]');

  // Picking from a dropdown keeps the page where it is, after the preview refreshes too.
  await fresh();
  const rpe = `[data-per-week="root.days.0.groups.1.exercises.0.sets.0.rpe"] select`;
  await evaluate(`document.querySelector('${rpe}').scrollIntoView({ block: "center" })`);
  const scrolled = await evaluate("window.scrollY");
  await evaluate(`(() => { window.__swapped = false;
    document.getElementById("plan-preview").addEventListener("htmx:afterSettle", () => { window.__swapped = true; }, { once: true });
    const s = document.querySelector('${rpe}'); s.value = "9"; s.dispatchEvent(new Event("change", {bubbles: true})); })()`);
  await h.waitFor(() => evaluate("window.__swapped"), "the preview after a select change").catch(() => {});
  // Warnings re-marked after the swap move the page briefly; scroll anchoring settles it.
  await h.waitFor(async () => (await evaluate("window.scrollY")) === scrolled, "the scroll position", 3000).catch(() => {});
  const after = await evaluate("window.scrollY");
  h.check("a select change keeps the scroll position", scrolled > 0 && after === scrolled, `before ${scrolled}, after ${after}`);

  // A refused document, once corrected, opens in the form.
  await fresh();
  await toJSONAndBack(`(d) => { d.days[0].groups[0].exercises[0].sets[0].load = { pct_tm: 0.5, weight: 60 }; }`);
  h.check("a two-key load is refused", await evaluate(`!document.querySelector('[data-view-note]').hidden`));
  await evaluate(`(() => { const t = document.getElementById('doc'); const d = JSON.parse(t.value); d.days[0].groups[0].exercises[0].sets[0].load = { weight: 60 }; t.value = JSON.stringify(d); })()`);
  await evaluate(`document.querySelector('[data-view="form"]').click()`);
  h.check("a corrected document opens in the form",
    await evaluate(`!document.getElementById('doc-form').hidden && document.querySelector('[data-view-note]').hidden`),
    await evaluate(`document.querySelector('[data-view-note]').textContent`));
  const load = (await doc()).days[0].groups[0].exercises[0].sets[0].load;
  h.check("the corrected load is the one in the form", JSON.stringify(load) === '{"weight":60}', JSON.stringify(load));

  // A per-week input that doesn't parse is flagged as you type, and stays
  // flagged through other form changes while its text is still wrong.
  await fresh();
  const reps = `[data-per-week="root.days.0.groups.0.exercises.0.sets.0.reps"]`;
  const repsBefore = (await doc()).days[0].groups[0].exercises[0].sets[0].reps;
  const typeInto = (selector, value) => evaluate(`(() => { const el = document.querySelector(${JSON.stringify(selector)});
    el.value = ${JSON.stringify(value)}; el.dispatchEvent(new Event('input', {bubbles: true})); })()`);
  const repsMsg = () => evaluate(`(() => { const p = document.querySelector('${reps} p'); return p && !p.hidden ? p.textContent : ""; })()`);
  await typeInto(`${reps} input:not([type=checkbox])`, "lots");
  await h.waitFor(async () => /enter reps like/.test(await repsMsg()), "the reps error while typing").catch(() => {});
  h.check("a bad per-week value is flagged as you type", /enter reps like/.test(await repsMsg()), await repsMsg());
  h.check("the error says the plan keeps the old value", (await repsMsg()).includes(`the plan still has ${repsBefore}`), await repsMsg());
  await evaluate(`(() => { window.__swapped = false;
    document.getElementById("plan-preview").addEventListener("htmx:afterSettle", () => { window.__swapped = true; }, { once: true }); })()`);
  await setInput(`#doc-form input[name="root[name]"]`, "Renamed plan");
  await h.waitFor(() => evaluate("window.__swapped"), "the preview after another change").catch(() => {});
  h.check("the error outlasts another form change", /enter reps like/.test(await repsMsg()) &&
    (await evaluate(`document.querySelector('${reps} input:not([type=checkbox])').value`)) === "lots", await repsMsg());
  h.check("the document keeps the old value", JSON.stringify((await doc()).days[0].groups[0].exercises[0].sets[0].reps) === JSON.stringify(repsBefore));
  await typeInto(`${reps} input:not([type=checkbox])`, "6");
  await h.waitFor(async () => (await repsMsg()) === "", "the reps error to clear").catch(() => {});
  h.check("a corrected value clears the error", (await repsMsg()) === "", await repsMsg());

  // Server markers stay put through a form change until a preview no longer reports them.
  await fresh();
  const restField = "[data-per-week='root.days.0.groups.0.rest_s']";
  const restMsg = () => evaluate(`(() => { const p = document.querySelector("${restField} p"); return p && !p.hidden ? p.textContent : ""; })()`);
  await setInput(`${restField} input:not([type=checkbox])`, "99999");
  await h.waitFor(async () => (await restMsg()).length > 0, "the rest marker").catch(() => {});
  await evaluate(`(() => { window.__swapped = false; window.__gaps = 0;
    const sample = () => { const p = document.querySelector("${restField} p");
      if (!p || p.hidden || !p.textContent) window.__gaps++;
      if (!window.__swapped) requestAnimationFrame(sample); };
    document.getElementById("plan-preview").addEventListener("htmx:afterSettle", () => { window.__swapped = true; }, { once: true });
    requestAnimationFrame(sample);
    const i = document.querySelector('#doc-form input[name="root[name]"]'); i.value = "Other name";
    i.dispatchEvent(new Event("change", {bubbles: true})); })()`);
  await h.waitFor(() => evaluate("window.__swapped"), "the preview after a name change").catch(() => {});
  const gaps = await evaluate("window.__gaps");
  h.check("server markers don't flicker on a form change", gaps === 0 && (await restMsg()).length > 0, `${gaps} frames without the marker`);
  await setInput(`${restField} input:not([type=checkbox])`, "180");
  await h.waitFor(async () => (await restMsg()) === "", "the rest marker to clear").catch(() => {});
  h.check("a marker clears once the preview no longer reports it", (await restMsg()) === "", await restMsg());

  // The "vary by week" toggle names its field, and checkboxes are easy to tap on a phone.
  await send("Emulation.setDeviceMetricsOverride", { width: 390, height: 844, deviceScaleFactor: 2, mobile: true });
  await fresh();
  h.check("the vary toggle names its field", await evaluate(`(() => {
    const c = document.querySelector('[data-per-week="root.days.0.groups.0.rest_s"]');
    const name = c.querySelector('input[type=checkbox]').getAttribute('aria-label') || '';
    return /vary/i.test(name) && name.includes(c.querySelector('label').textContent.trim()); })()`));
  const small = await evaluate(`[...document.querySelectorAll('#doc-form input[type=checkbox]')]
    .map((b) => b.closest('label').getBoundingClientRect()).filter((r) => r.width > 0 && (r.height < 40 || r.width < 40)).length`);
  h.check("checkboxes have a 40px tap area on phones", small === 0, `${small} too small`);
  await send("Emulation.clearDeviceMetricsOverride");
} catch (err) {
  h.check("scenario ran to the end", false, err.stack || String(err));
} finally {
  await h.finish(b);
}
