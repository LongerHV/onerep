// Plan form logic, free of the DOM and json-editor so Node can test it
// (internal/web/jstest/plan-form.test.mjs). plan-form-theme.js renders the
// per-week and week-set fields with it; plan-form.js uses the rest.

export const KINDS = ["integer", "count", "reps", "number", "rpe", "percent"];

const num = (text) => Number(String(text).trim().replace(",", "."));
// round4 avoids float noise: 82.5 / 100 is 0.8250000000000001 otherwise.
const round4 = (v) => Math.round(v * 10000) / 10000;

// parseInput reads one input of a per-week field. An empty input is
// undefined ("not set"); callers turn it into null for a skipped count week.
export function parseInput(kind, text) {
  const t = String(text ?? "").trim();
  if (t === "") return { value: undefined };
  switch (kind) {
    case "integer":
    case "count": {
      const v = num(t);
      return Number.isInteger(v) && v >= 0 ? { value: v } : { error: "enter a whole number" };
    }
    case "reps": {
      if (/^amrap$/i.test(t)) return { value: "AMRAP" };
      if (/^[1-9][0-9]?-[1-9][0-9]?$/.test(t)) return { value: t };
      const v = num(t);
      return Number.isInteger(v) && v >= 1 ? { value: v } : { error: "enter reps like 5, 6-10 or AMRAP" };
    }
    case "percent": {
      const v = num(t);
      return Number.isFinite(v) && v > 0 ? { value: round4(v / 100) } : { error: "enter a percentage like 75" };
    }
    default: {
      const v = num(t);
      return Number.isFinite(v) ? { value: v } : { error: "enter a number" };
    }
  }
}

export function formatValue(kind, value) {
  if (value === undefined || value === null) return "";
  if (kind === "percent" && typeof value === "number") return String(round4(value * 100));
  return String(value);
}

// perWeekFrom turns a stored value into the field's state. A stored array
// keeps its length: fields can be set before the plan's weeks is, and only a
// change of weeks resizes (see resize).
export function perWeekFrom(value) {
  if (Array.isArray(value)) return { vary: true, values: value.slice() };
  return { vary: false, values: [value] };
}

// perWeekValue is what the field stores: nothing when empty, one value when
// every week is the same, otherwise one entry per week.
export function perWeekValue(state) {
  const vals = state.vary ? state.values : state.values.slice(0, 1);
  if (vals.every((v) => v === undefined || v === null)) return undefined;
  if (vals.every((v) => v === vals[0])) return vals[0];
  return vals.map((v) => (v === undefined ? null : v));
}

// resize fits a varying field to the plan's weeks: new weeks copy the last one.
export function resize(state, weeks) {
  if (!state.vary) return state;
  const values = state.values.slice(0, weeks);
  while (values.length < weeks) values.push(values.length ? values[values.length - 1] : undefined);
  return { vary: true, values };
}

// weekSetFrom reads only_weeks. Weeks beyond the plan are kept, so shrinking
// a plan never quietly turns "week 5 only" into "every week"; the server
// reports them instead.
export function weekSetFrom(value) {
  return Array.isArray(value) ? [...new Set(value.filter(Number.isInteger))].sort((a, b) => a - b) : [];
}

export function weekSetValue(weeks) {
  return weeks.length ? weekSetFrom(weeks) : undefined;
}

const isPlainObject = (v) => v !== null && typeof v === "object" && !Array.isArray(v);

// clean removes values the form produces for untouched optional fields.
export function clean(doc) {
  if (Array.isArray(doc)) return doc.map(clean);
  if (!isPlainObject(doc)) return doc;
  const out = {};
  for (const [k, v] of Object.entries(doc)) {
    if (v === undefined || v === "") continue;
    if ((k === "alternatives" || k === "only_weeks") && Array.isArray(v) && v.length === 0) continue;
    if (k === "load" && isPlainObject(v) && Object.keys(v).length === 0) continue;
    if (k === "kind" && v === "working") continue;
    out[k] = clean(v);
  }
  return out;
}

function canonical(v) {
  if (Array.isArray(v)) return v.map(canonical);
  if (!isPlainObject(v)) return v;
  return Object.fromEntries(Object.keys(v).sort().map((k) => [k, canonical(v[k])]));
}

// sameDoc reports whether two documents mean the same plan: key order is
// ignored, cleaned values are ignored, and a missing unit is the user's.
export function sameDoc(a, b, unit) {
  const norm = (d) => canonical(clean(isPlainObject(d) && !d.unit ? { ...d, unit } : d));
  return JSON.stringify(norm(a)) === JSON.stringify(norm(b));
}

export function pointerToPath(pointer) {
  if (!pointer) return "root";
  return "root." + pointer.slice(1).split("/").map((p) => p.replace(/~1/g, "/").replace(/~0/g, "~")).join(".");
}

// nearestPath walks up from path to the closest path that has an editor. A
// trailing week index on a per-week value becomes week (1-based).
export function nearestPath(path, has) {
  const parts = path.split(".");
  let week = null;
  while (parts.length > 1 && !has(parts.join("."))) {
    const last = parts.pop();
    week = parts.length > 0 && /^\d+$/.test(last) && week === null && has(parts.join(".")) ? Number(last) + 1 : null;
  }
  return { path: parts.join("."), week };
}

// problemsToErrors turns the server's problems into json-editor errors.
// Warnings are marked with a "Warning: " prefix, which the theme styles amber.
export function problemsToErrors(problems, has) {
  return (problems || []).map((p) => {
    const { path, week } = nearestPath(pointerToPath(p.pointer), has);
    const text = (week ? `W${week}: ` : "") + p.message;
    return { path, property: "onerep", message: p.warning ? `Warning: ${text}` : text };
  });
}

// headerText renders the "@…" header templates of the editor schema.
export function headerText(template, vars, names) {
  const self = vars?.self || {};
  switch (template) {
    case "@day": return self.name || `Day ${vars.i1}`;
    case "@group": return `Group ${vars.i1}` + ((self.exercises || []).length > 1 ? " · superset" : "");
    case "@slot": return names[self.slug] || self.slug || `Exercise ${vars.i1}`;
    case "@set": return `Set line ${vars.i1}`;
    default: return template;
  }
}

export function slugNames(schema) {
  const slug = schema?.definitions?.slug;
  const out = {};
  (slug?.enum || []).forEach((s, i) => { out[s] = slug.options?.enum_titles?.[i] || s; });
  return out;
}

// markRequired returns a copy of the editor schema with json-editor's boolean
// "required" on every property. json-editor drops an absent optional
// property when a document is loaded, so a set line without a load could
// never gain one; empty fields are still left out of the document (the
// per-week editors return nothing, remove_empty_properties and clean drop the
// rest). This is json-editor-only syntax, so it's added here rather than in
// the Go editor schema, which is checked against the contract.
export function markRequired(schema) {
  const copy = structuredClone(schema);
  const walk = (node) => {
    if (Array.isArray(node)) return node.forEach(walk);
    if (!isPlainObject(node)) return;
    if (isPlainObject(node.properties)) {
      for (const p of Object.values(node.properties)) if (isPlainObject(p)) p.required = true;
    }
    Object.values(node).forEach(walk);
  };
  walk(copy);
  return copy;
}
