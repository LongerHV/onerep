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
  check: "inline-flex items-center gap-1 text-xs text-zinc-600 dark:text-zinc-400",
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

  class OnerepTheme extends JSONEditor.AbstractTheme {
    constructor(jsoneditor) { super(jsoneditor, { disable_theme_rules: true }); }
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
    weeks() {
      const w = this.jsoneditor.getEditor("root.weeks")?.getValue();
      return Number.isInteger(w) && w >= 1 && w <= 52 ? w : 1;
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
    showValidationErrors(errors) {
      const msgs = errors.filter((e) => e.path === this.path && e.property === "onerep").map((e) => e.message);
      const warning = msgs.length > 0 && msgs.every((m) => m.startsWith("Warning: "));
      this.errmsg.className = warning ? cls.warn : cls.error;
      this.errmsg.textContent = msgs.map((m) => m.replace(/^Warning: /, "")).join(". ");
      this.errmsg.hidden = msgs.length === 0;
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
      this.control = document.createElement("div");
      this.control.dataset.perWeek = this.path;
      const label = this.theme.getFormInputLabel(this.getTitle(), this.isRequired());
      const toggle = add(document.createElement("label"), cls.check);
      this.varyBox = document.createElement("input");
      this.varyBox.type = "checkbox";
      this.varyBox.addEventListener("change", () => {
        const first = this.state.values[0];
        this.state = this.varyBox.checked
          ? { vary: true, values: Array(this.weeks()).fill(first) }
          : { vary: false, values: [first] };
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
      if (!this.state.vary) return;
      this.state = core.resize(this.state, this.weeks());
      this.render();
      this.commit(core.perWeekValue(this.state));
    }
    setValue(value, initial) {
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
      el.value = core.formatValue(this.kind, v);
      el.setAttribute("aria-label", this.state.vary ? `${this.getTitle()} week ${i + 1}` : this.getTitle());
      el.addEventListener("change", () => {
        const r = core.parseInput(this.kind, el.value);
        if (r.error) {
          this.errmsg.textContent = r.error;
          this.errmsg.hidden = false;
          el.classList.add(...cls.invalid);
          return;
        }
        el.classList.remove(...cls.invalid);
        this.errmsg.hidden = true;
        const value = r.value === undefined && this.kind === "count" && this.state.vary ? null : r.value;
        this.state.values[i] = value;
        this.commit(core.perWeekValue(this.state));
      });
      if (!this.state.vary) return el;
      const wrap = add(document.createElement("label"), "flex flex-col text-xs text-zinc-500");
      wrap.append(document.createTextNode(`W${i + 1}`), el);
      return wrap;
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

  JSONEditor.defaults.themes.onerep = OnerepTheme;
  JSONEditor.defaults.editors.perweek = PerWeekEditor;
  JSONEditor.defaults.editors.weekset = WeekSetEditor;
  JSONEditor.defaults.resolvers.unshift((schema) =>
    schema.format === "per-week" ? "perweek" : schema.format === "week-set" ? "weekset" : undefined);
  JSONEditor.defaults.templates.onerep = () => ({ compile: (t) => (vars) => core.headerText(t, vars, names) });
}
