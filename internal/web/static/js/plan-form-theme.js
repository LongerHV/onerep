// json-editor theme, custom editors and header templates for the plan form.
// Classes mirror internal/web/views/ui.go (keep them in sync); every class is
// a literal string so Tailwind's scanner (styles/input.css @source) sees it.
import * as core from "./plan-form-core.js";

const cls = {
  input: "block w-full rounded border border-zinc-300 bg-white px-2 py-1.5 text-sm text-zinc-900 min-h-10 sm:min-h-0 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-100",
  invalid: ["border-red-500", "dark:border-red-500"],
  warning: ["border-amber-500", "dark:border-amber-500"],
  label: "block text-sm font-medium",
  hint: "mt-1 text-xs text-zinc-500",
  error: "mt-1 text-sm text-red-600 dark:text-red-400",
  warn: "mt-1 text-sm text-amber-700 dark:text-amber-400",
  button: "inline-flex min-h-10 items-center rounded border border-zinc-300 px-2 py-1 text-xs hover:bg-zinc-100 sm:min-h-0 dark:border-zinc-700 dark:hover:bg-zinc-800",
  danger: "inline-flex min-h-10 items-center rounded border border-red-300 px-2 py-1 text-xs text-red-700 hover:bg-red-50 sm:min-h-0 dark:border-red-800 dark:text-red-400 dark:hover:bg-red-950",
  card: "mt-2 rounded border border-zinc-200 p-2 sm:p-3 dark:border-zinc-800",
  header: "text-sm font-semibold",
  tabs: "flex gap-1 overflow-x-auto border-b border-zinc-200 dark:border-zinc-800",
  tab: "cursor-pointer whitespace-nowrap border-b-2 px-3 py-2 text-sm",
  tabActive: ["border-zinc-900", "font-medium", "dark:border-zinc-100"],
  tabInactive: ["border-transparent", "text-zinc-500"],
  control: "mb-2",
  weeks: "flex flex-wrap gap-1",
  weekInput: "w-16 rounded border border-zinc-300 bg-white px-1 py-1 text-sm text-zinc-900 min-h-10 sm:min-h-0 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-100",
  // At least a 40px tap area on phones.
  check: "inline-flex min-h-10 min-w-10 items-center gap-1 text-xs text-zinc-600 sm:min-h-0 sm:min-w-0 dark:text-zinc-400",
};
// json-editor grid sizes 1-12; full width on phones.
const cols = ["", "sm:w-1/12", "sm:w-2/12", "sm:w-3/12", "sm:w-4/12", "sm:w-5/12", "sm:w-6/12",
  "sm:w-7/12", "sm:w-8/12", "sm:w-9/12", "sm:w-10/12", "sm:w-11/12", "sm:w-full"];
const add = (el, c) => { el.classList.add(...(Array.isArray(c) ? c : c.split(" "))); return el; };

let names = {};
export function setNames(n) { names = n; }

let registered = false;
export function register(JSONEditor) {
  if (registered) return;
  registered = true;

  // json-editor "sanitises" strings by parsing them with innerHTML, which runs
  // event handlers such as <img onerror>; plan strings come from users and the
  // AI. Values go into inputs as .value and headers as text, so strings are
  // left as they are, and text is extracted with an inert parser.
  const inert = (t) => new DOMParser().parseFromString(String(t ?? ""), "text/html").body.textContent || "";
  JSONEditor.AbstractEditor.prototype.purify = (v) => v;
  JSONEditor.AbstractEditor.prototype.cleanText = inert;

  class OnerepTheme extends JSONEditor.AbstractTheme {
    constructor(jsoneditor) { super(jsoneditor, { disable_theme_rules: true }); }
    cleanText(t) { return inert(t); }
    getFormInputLabel(text, req) { const l = super.getFormInputLabel(text, req); l.className = cls.label; return l; }
    getFormInputField(type) { return add(super.getFormInputField(type), cls.input); }
    getSelectInput(options, multiple) { return add(super.getSelectInput(options, multiple), multiple ? cls.input + " h-28" : cls.input); }
    getSwitcher(options) { return add(super.getSwitcher(options), cls.input); }
    getTextareaInput() { const t = add(super.getTextareaInput(), cls.input); t.rows = 2; return t; }
    getFormControl(label, input, description, infoText, formName) {
      return add(super.getFormControl(label, input, description, infoText, formName), cls.control);
    }
    getHeader(text, pathDepth) { return add(super.getHeader(text, pathDepth), cls.header); }
    getDescription(text) { return add(super.getDescription(text), cls.hint); }
    getIndentedPanel() { return add(document.createElement("div"), cls.card); }
    getTopIndentedPanel() { return add(document.createElement("div"), "mt-2"); }
    getGridContainer() { return document.createElement("div"); }
    getGridRow() { return add(document.createElement("div"), "-mx-1 flex flex-wrap"); }
    getGridColumn() { return add(document.createElement("div"), "w-full px-1"); }
    setGridColumnSize(el, size) { if (cols[size]) add(el, cols[size]); }
    getButtonHolder() { return add(document.createElement("span"), "inline-flex flex-wrap gap-1"); }
    getButton(text, icon, title) {
      const b = super.getButton(text, icon, title);
      const danger = /^(delete|remove)/i.test(title || text || "");
      return add(b, danger ? cls.danger : cls.button);
    }
    getErrorMessage(text) { const p = add(document.createElement("p"), cls.error); p.textContent = text; return p; }
    addInputError(input, text) {
      const warning = text.startsWith("Warning: ");
      const group = input.controlgroup || input.parentNode;
      if (!input.errmsg && group) { input.errmsg = document.createElement("p"); group.appendChild(input.errmsg); }
      if (input.errmsg) {
        input.errmsg.className = warning ? cls.warn : cls.error;
        input.errmsg.setAttribute("role", "alert");
        input.errmsg.textContent = text.replace(/^Warning: /, "").replace(/\.$/, "");
        input.errmsg.hidden = false;
      }
      input.classList.remove(...cls.invalid, ...cls.warning);
      input.classList.add(...(warning ? cls.warning : cls.invalid));
    }
    removeInputError(input) {
      if (input.errmsg) input.errmsg.hidden = true;
      input.classList.remove(...cls.invalid, ...cls.warning);
    }
    getTopTabHolder(propertyName) {
      const holder = document.createElement("div");
      const strip = add(document.createElement("div"), cls.tabs);
      strip.setAttribute("role", "tablist");
      const content = document.createElement("div");
      if (propertyName) content.id = propertyName;
      holder.append(strip, content);
      return holder;
    }
    getTopTabContentHolder(holder) { return holder.children[1]; }
    getTopTab(span, tabId) {
      const t = add(document.createElement("div"), cls.tab);
      t.id = tabId;
      t.setAttribute("role", "tab");
      t.tabIndex = 0; // reachable from the keyboard; Enter or Space opens it
      t.addEventListener("keydown", (e) => {
        if (e.key === "Enter" || e.key === " ") { e.preventDefault(); t.click(); }
      });
      t.appendChild(span);
      return t;
    }
    addTopTab(holder, tab) { holder.children[0].appendChild(tab); }
    markTabActive(row) {
      row.tab.classList.remove(...cls.tabInactive);
      row.tab.classList.add(...cls.tabActive);
      row.tab.setAttribute("aria-selected", "true");
      (row.rowPane || row.container).style.display = "";
    }
    markTabInactive(row) {
      row.tab.classList.remove(...cls.tabActive);
      row.tab.classList.add(...cls.tabInactive);
      row.tab.setAttribute("aria-selected", "false");
      (row.rowPane || row.container).style.display = "none";
    }
  }

  // Shared by the per-week and week-set fields.
  class WeeksAware extends JSONEditor.AbstractEditor {
    // weeks is the plan's current length, or its last valid one while Weeks is
    // empty or mistyped.
    weeks() {
      const w = core.validWeeks(this.jsoneditor.getEditor("root.weeks")?.getValue());
      if (w !== null) this.lastWeeks = w;
      return this.lastWeeks || 1;
    }
    // weeksValid reports whether Weeks holds a usable value right now.
    weeksValid() {
      return core.validWeeks(this.jsoneditor.getEditor("root.weeks")?.getValue()) !== null;
    }
    watchWeeks() {
      this.onWeeks = () => this.weeksChanged();
      this.jsoneditor.watch("root.weeks", this.onWeeks);
    }
    commit(value) {
      const changed = JSON.stringify(value) !== JSON.stringify(this.value);
      this.value = value;
      if (changed) { this.is_dirty = true; this.onChange(true); }
    }
    // showValidationErrors receives the server's problems (json-editor's own
    // validation is off, so its change pass sends none, and plan-form.js
    // re-applies the last ones right after).
    showValidationErrors(errors) {
      this.serverMsgs = errors.filter((e) => e.path === this.path && e.property === "onerep").map((e) => e.message);
      this.showMessage();
    }
    draftMessages() { return []; }
    showMessage() {
      const { text, warning } = core.fieldMessage(this.draftMessages(), this.serverMsgs || []);
      this.errmsg.className = warning ? cls.warn : cls.error;
      this.errmsg.textContent = text;
      this.errmsg.hidden = text === "";
    }
    destroy() {
      if (this.onWeeks) this.jsoneditor.unwatch("root.weeks", this.onWeeks);
      this.control?.remove();
      super.destroy();
    }
    getNumColumns() { return 2; }
  }

  class PerWeekEditor extends WeeksAware {
    build() {
      this.kind = this.options.perWeek?.kind || "number";
      this.state = { vary: false, values: [undefined] };
      // drafts holds, by input index, text that doesn't parse: it stays in its
      // input and is reported until corrected, while the document keeps the
      // old value.
      this.drafts = new Map();
      this.control = document.createElement("div");
      this.control.dataset.perWeek = this.path;
      const label = this.theme.getFormInputLabel(this.getTitle(), this.isRequired());
      const toggle = add(document.createElement("label"), cls.check);
      this.varyBox = document.createElement("input");
      this.varyBox.type = "checkbox";
      this.varyBox.setAttribute("aria-label", `Vary ${this.getTitle()} by week`);
      this.varyBox.addEventListener("change", () => {
        const first = this.state.values[0];
        this.state = this.varyBox.checked
          ? { vary: true, values: Array(this.weeks()).fill(first) }
          : { vary: false, values: [first] };
        this.drafts.clear();
        this.showMessage();
        this.render();
        this.commit(core.perWeekValue(this.state));
      });
      toggle.append(this.varyBox, document.createTextNode("vary by week"));
      this.inputs = add(document.createElement("div"), cls.weeks);
      this.errmsg = add(document.createElement("p"), cls.error);
      this.errmsg.hidden = true;
      this.control.append(label, this.inputs, toggle, this.errmsg);
      if (this.schema.description) this.control.title = this.schema.description;
      this.container.appendChild(this.control);
      this.watchWeeks();
      this.render();
    }
    weeksChanged() {
      if (!this.state.vary || !this.weeksValid()) return; // keep values while Weeks is being retyped
      this.state = core.resize(this.state, this.weeks());
      for (const i of [...this.drafts.keys()]) if (i >= this.state.values.length) this.drafts.delete(i);
      this.showMessage();
      this.render();
      this.commit(core.perWeekValue(this.state));
    }
    setValue(value, initial) {
      // A new value from outside (the JSON view, a moved row) replaces any draft.
      if (this.drafts.size && JSON.stringify(value) !== JSON.stringify(this.value)) {
        this.drafts.clear();
        this.showMessage();
      }
      this.state = core.perWeekFrom(value, this.weeks());
      this.value = core.perWeekValue(this.state);
      if (this.inputs) this.render();
      if (!initial) this.is_dirty = true;
      this.onChange(false);
    }
    getValue() { return this.value; }
    field(i) {
      const v = this.state.values[i];
      let el;
      if (this.kind === "rpe") {
        el = add(document.createElement("select"), this.state.vary ? cls.weekInput : cls.input);
        el.append(new Option("", ""));
        for (let r = 6; r <= 10; r += 0.5) el.append(new Option(String(r), String(r)));
      } else {
        el = add(document.createElement("input"), this.state.vary ? cls.weekInput : cls.input);
        el.inputMode = this.kind === "reps" ? "text" : "decimal";
        if (this.kind === "percent") el.placeholder = "%";
      }
      const draft = this.drafts.get(i);
      el.value = draft ? draft.text : core.formatValue(this.kind, v);
      if (draft) el.classList.add(...cls.invalid);
      el.setAttribute("aria-label", this.state.vary ? `${this.getTitle()} week ${i + 1}` : this.getTitle());
      // Checked as you type (a mistake shows after a short pause, a fix at
      // once); the value reaches the document on change, like other fields.
      let timer;
      el.addEventListener("input", () => {
        clearTimeout(timer);
        if (core.parseInput(this.kind, el.value).error) timer = setTimeout(() => this.check(i, el), 400);
        else this.check(i, el);
      });
      el.addEventListener("change", () => {
        clearTimeout(timer);
        const r = this.check(i, el);
        if (r.error) return;
        const value = r.value === undefined && this.kind === "count" && this.state.vary ? null : r.value;
        this.state.values[i] = value;
        this.commit(core.perWeekValue(this.state));
      });
      if (!this.state.vary) return el;
      const wrap = add(document.createElement("label"), "flex flex-col text-xs text-zinc-500");
      wrap.append(document.createTextNode(`W${i + 1}`), el);
      return wrap;
    }
    // check parses input i and records or clears its draft.
    check(i, el) {
      const r = core.parseInput(this.kind, el.value);
      if (r.error) this.drafts.set(i, { text: el.value, error: r.error });
      else this.drafts.delete(i);
      for (const c of cls.invalid) el.classList.toggle(c, !!r.error);
      this.showMessage();
      return r;
    }
    draftMessages() {
      return [...this.drafts.entries()].sort(([a], [b]) => a - b)
        .map(([i, d]) => core.draftError(this.kind, d.error, this.state.values[i], this.state.vary ? i + 1 : null));
    }
    render() {
      this.varyBox.checked = this.state.vary;
      const n = this.state.vary ? this.state.values.length : 1;
      this.inputs.replaceChildren(...Array.from({ length: n }, (_, i) => this.field(i)));
    }
  }

  class WeekSetEditor extends WeeksAware {
    build() {
      this.selected = [];
      this.control = document.createElement("div");
      this.control.dataset.weekSet = this.path;
      this.control.append(this.theme.getFormInputLabel(this.getTitle(), false));
      this.boxes = add(document.createElement("div"), cls.weeks);
      this.boxes.setAttribute("role", "group");
      this.boxes.setAttribute("aria-label", this.getTitle());
      this.errmsg = add(document.createElement("p"), cls.error);
      this.errmsg.hidden = true;
      this.control.append(this.boxes, this.errmsg);
      this.container.appendChild(this.control);
      this.watchWeeks();
      this.render();
    }
    weeksChanged() { this.render(); }
    setValue(value, initial) {
      this.selected = core.weekSetFrom(value);
      this.value = core.weekSetValue(this.selected);
      if (this.boxes) this.render();
      if (!initial) this.is_dirty = true;
      this.onChange(false);
    }
    getValue() { return this.value; }
    render() {
      const n = this.weeks();
      const weeks = [...new Set([...Array.from({ length: n }, (_, i) => i + 1), ...this.selected])].sort((a, b) => a - b);
      this.boxes.replaceChildren(...weeks.map((w) => {
        const l = add(document.createElement("label"), cls.check);
        const box = document.createElement("input");
        box.type = "checkbox";
        box.checked = this.selected.includes(w);
        box.addEventListener("change", () => {
          this.selected = box.checked ? [...this.selected, w] : this.selected.filter((x) => x !== w);
          this.selected = core.weekSetFrom(this.selected);
          this.commit(core.weekSetValue(this.selected));
        });
        l.append(box, document.createTextNode(w > n ? `W${w} (not in plan)` : `W${w}`));
        return l;
      }));
    }
  }

  // SlugListEditor edits alternatives as an ordered list: a select adds to the
  // end, each entry has a Remove button. (A native multi-select sorts them
  // and drops the other picks on a plain click.)
  class SlugListEditor extends WeeksAware {
    build() {
      const items = this.jsoneditor.expandRefs ? this.jsoneditor.expandRefs(this.schema.items || {}) : (this.schema.items || {});
      const slugs = items.enum || [];
      const titles = items.options?.enum_titles || slugs;
      this.names = Object.fromEntries(slugs.map((s, i) => [s, titles[i] || s]));
      this.list = [];
      this.control = document.createElement("div");
      this.control.dataset.slugList = this.path;
      this.control.append(this.theme.getFormInputLabel(this.getTitle(), false));
      this.items = add(document.createElement("ul"), "space-y-1 text-sm");
      this.picker = add(document.createElement("select"), cls.input);
      this.picker.setAttribute("aria-label", `Add to ${this.getTitle().toLowerCase()}`);
      this.picker.append(new Option("Add an alternative…", ""), ...slugs.map((s) => new Option(this.names[s], s)));
      this.picker.addEventListener("change", () => {
        this.list = core.listAdd(this.list, this.picker.value);
        this.picker.value = "";
        this.render();
        this.commit(core.listValue(this.list));
      });
      this.errmsg = add(document.createElement("p"), cls.error);
      this.errmsg.hidden = true;
      this.control.append(this.items, this.picker, this.errmsg);
      this.container.appendChild(this.control);
      this.render();
    }
    setValue(value, initial) {
      this.list = Array.isArray(value) ? value.filter((s) => typeof s === "string") : [];
      this.value = core.listValue(this.list);
      if (this.items) this.render();
      if (!initial) this.is_dirty = true;
      this.onChange(false);
    }
    getValue() { return this.value; }
    render() {
      this.items.replaceChildren(...this.list.map((s, i) => {
        const li = add(document.createElement("li"), "flex items-center justify-between gap-2");
        const name = document.createElement("span");
        name.textContent = this.names[s] || s;
        const remove = add(document.createElement("button"), cls.danger);
        remove.type = "button";
        remove.textContent = "Remove";
        remove.setAttribute("aria-label", `Remove ${this.names[s] || s}`);
        remove.addEventListener("click", () => {
          this.list = core.listRemove(this.list, i);
          this.render();
          this.commit(core.listValue(this.list));
        });
        li.append(name, remove);
        return li;
      }));
    }
    getNumColumns() { return 12; }
  }

  JSONEditor.defaults.themes.onerep = OnerepTheme;
  JSONEditor.defaults.editors.sluglist = SlugListEditor;
  JSONEditor.defaults.editors.perweek = PerWeekEditor;
  JSONEditor.defaults.editors.weekset = WeekSetEditor;
  JSONEditor.defaults.resolvers.unshift((schema) =>
    schema.format === "per-week" ? "perweek" : schema.format === "week-set" ? "weekset"
      : schema.format === "slug-list" ? "sluglist" : undefined);
  JSONEditor.defaults.templates.onerep = () => ({ compile: (t) => (vars) => core.headerText(t, vars, names) });
}
