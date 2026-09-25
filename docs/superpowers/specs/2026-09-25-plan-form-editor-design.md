# Plan form editor — Design

Status: design approved in brainstorming (2026-09-25), pending spec review.

## 1. Summary

Replace the plan page's text JSON editor (`vanilla-jsoneditor`) with a form generated from the plan JSON Schema by [json-editor](https://github.com/json-editor/json-editor). The aim is to edit a plan without writing JSON. A plain JSON text view stays available.

`internal/plan/plan.schema.json` remains the single contract for the server, the web editor and the AI. The form gets a **derived editor schema** built on the server: the contract, plus UI hints from an overlay file, a few structural rewrites, and the user's exercise catalog. A dedicated theme matches the app's Tailwind look in light and dark, on desktop and phone.

## 2. Goals and non-goals

### Goals
- Edit every part of a plan through a form: days, groups, supersets, exercises, set lines, loads and per-week values.
- A per-week field that can never have the wrong number of weeks.
- Exercises picked from the user's own catalog in a searchable select.
- Server validation problems shown on the fields they concern, and still in the list.
- A responsive layout: table rows for set lines on wide screens, stacked fields on phones.
- Raw JSON editing kept as a plain text view.
- No change to the contract, server validation, saving, `/schema/plan.json` or MCP.

### Non-goals
- Client-side validation rules. The server's validator stays the only validator.
- Reordering the keys of existing documents written by the AI or by hand.
- Offline plan editing.
- A form for anything other than plans.

## 3. Decisions made in brainstorming

| Question | Decision |
|---|---|
| Raw JSON editing | Form plus a plain JSON text view (the existing textarea). `vanilla-jsoneditor` is removed. |
| Where plans are edited | Both desktop and phone: a responsive layout. |
| Validation feedback | Keep the server's problem list and also mark the fields. json-editor's own validator stays off. |
| Per-week values | A custom per-week field sized to the plan's `weeks`. |
| Where UI hints live | A derived editor schema built on the server (approach A); the contract stays free of UI keywords. |
| Dependency | Vendor `@json-editor/json-editor` 2.17.2 (MIT), which the user proposed. |

## 4. Architecture

```
plan.schema.json (contract) ──┐
editor.overlay.json ──────────┼─► plan.EditorSchema(catalog, docSlugs) ─► page: #plan-schema
user's catalog ───────────────┘

#doc textarea ◄── plan-form.js (json-editor + plan-form-theme.js + plan-form-core.js)
      │ doc-changed
      ▼
POST /plans/preview ─► fragment: problems list + resolved weeks + #plan-problems JSON
                                                    │ htmx:afterSwap
                                                    ▼
                                    plan-form.js marks fields (showValidationErrors)
```

- `<textarea id="doc">` stays the form field and the source of truth. The form writes pretty-printed JSON into it and fires `doc-changed`. Saving posts the textarea, as today.
- Without JavaScript, or if the library fails to load, the textarea works on its own, as today.
- The editor loads lazily on the editor page only (dynamic import from the layout-loaded script, set up via `htmx.onLoad`). It is destroyed when htmx swaps the page away.

## 5. The editor schema (`internal/plan/editor.go`)

`func EditorSchema(catalog []store.Exercise, docSlugs []string) ([]byte, error)` builds the form's schema from the contract in three steps.

1. **Overlay.** `internal/plan/editor.overlay.json` maps JSON Pointers into the contract (e.g. `/$defs/day`) to objects that are deep-merged at that location. The overlay contains only UI keywords: `title`, `headerTemplate`, `propertyOrder`, `format` and `options` (collapsed, grid columns, `remove_empty_properties`, confirmation on delete, and so on).
2. **Rewrites**, one small function each:
   - Drop the set line's object-level `oneOf` of `required: [reps]` / `required: [duration_s]`. Both fields are shown and the server enforces that exactly one is set.
   - Replace `load`'s `maxProperties: 1` object with a titled `oneOf` of four single-key objects (Weight, % of TM, RPE, Drop %) plus a "None" branch (an empty object, which `remove_empty_properties` drops, so the set line has no `load`).
   - Replace every per-week `oneOf [value, array of values]` with the value schema plus `"format": "per-week"` and `options.perWeek.kind` (see §6).
   - If json-editor doesn't resolve `$defs` (checked by the probe, §11), rename `$defs` to `definitions` and rewrite every `$ref`.
3. **Catalog.** The slug schema gets `enum` with the user's catalog slugs (custom exercises included) and `options.enum_titles` with their names, rendered as a searchable select. Slugs the document already uses are added even when missing from the catalog (for example a now-hidden seeded exercise), so such a plan still opens. `alternatives` items use the same enum.

`views.PlanEditor.Schema` carries the editor schema. `/schema/plan.json` and MCP's `get_plan_schema` keep serving the contract.

## 6. The form

### Structure
- **Plan:** name, unit and weeks on one row.
- **Days:** one tab per day, labelled with the day's name (`headerTemplate`). On phones the tabs are a horizontally scrolling strip. `only_weeks` is a row of week checkboxes (W1…Wn).
- **Groups:** cards titled "Group n", or "Group n · superset" with more than one exercise, with the rest time in the header.
- **Exercise slots:** a searchable catalog select. `notes` and `alternatives` sit behind a collapsed "More" section.
- **Set lines:** kind, count, reps or duration, load and RPE, with add, remove and move buttons. On wide screens each set line is one row (json-editor's grid layout with responsive column classes); on phones the fields stack full-width.
- **Load:** a switcher (Weight, % of TM, RPE, Drop % or none) followed by that key's per-week field.
- Empty optional fields are removed from the JSON (`remove_empty_properties`).

### Per-week field (`format: "per-week"`)
A custom json-editor editor.
- **One input by default.** A "vary by week" toggle turns it into exactly `weeks` inputs labelled W1…Wn, in one row on wide screens and wrapping on phones.
- **Follows `weeks`:** it watches `root.weeks`. When the plan gets longer, new weeks copy the last value; when it gets shorter, extra weeks are trimmed.
- **Stores a plain value when every week is the same,** and an array otherwise.
- **Value kinds** (`options.perWeek.kind`):

| Kind | Used for | Input |
|---|---|---|
| `integer` | `rest_s`, `duration_s` | whole number |
| `count` | `count` | whole number; an empty week means "skip this week" (`null`) |
| `reps` | `reps` | a number, `lo-hi` or `AMRAP`, format-checked as you type |
| `number` | `weight` | decimal in the plan's unit |
| `rpe` | `rpe`, `load.rpe` | select, 6–10 in 0.5 steps |
| `percent` | `pct_tm`, `drop_pct` | typed as a percentage (75), stored as a fraction (0.75) |

The pure logic (resizing, collapsing to a single value, parsing and formatting per kind) lives in `static/js/plan-form-core.js`, tested in Node. The DOM part is thin.

## 7. Theme and responsive layout

`static/js/plan-form-theme.js` is a subclass of `JSONEditor.AbstractTheme` using the app's class sets. They are the same strings as `internal/web/views/ui.go`, duplicated on purpose with a comment pointing there:

- inputs and selects: `rounded border border-zinc-300 bg-white px-2 py-1.5` and `dark:` variants
- buttons: the secondary button style, small; delete in the danger style
- groups and slots: the `card` style; days: an underline tab strip
- labels, hints and errors: the existing form classes; warnings amber
- no icon library: short text buttons ("Add set", "Remove", "↑", "↓") with `aria-label`s

Other rules:
- Dark mode follows `prefers-color-scheme` via Tailwind `dark:` variants. json-editor's injected CSS is disabled (`disable_theme_rules`) and none of its stylesheets are loaded.
- json-editor `grid_columns` sizes map to literal classes (`w-full sm:w-2/12`, …) from a fixed lookup table, so the Tailwind scanner sees every class. `internal/web/styles/input.css` gets `@source "../static/js/plan-form-theme.js"`.
- On phones every field is full-width, with touch targets of at least 40px.
- Trimmed controls: no "Edit JSON", "Properties" or "delete all rows" buttons. Deleting a day or a group asks for confirmation; deleting a set line does not.

## 8. JSON view

- A **Form | JSON** switch above the editor. Form is the default; the last choice is kept in `localStorage` (per browser, a convenience only).
- **JSON view** is the existing textarea, pretty-printed with 2-space indents.
- **Form → JSON** always works, because the textarea holds the form's output.
- **JSON → Form:** the text is parsed, loaded into the form, read back and compared with the original (key order ignored; values, including `null`, compared exactly).
  - If it doesn't parse, or the form would change it (unknown keys, shapes the form can't show), the switch is refused with a one-line reason and the view stays on JSON.
  - A plan that opens with such content starts in the JSON view, with the same note.
- **Key order:** the form writes keys in the contract's order (the order of the starter template), so versions edited in the form diff cleanly against each other. A document written in another order (for example by the AI) shows as reordered once, on its first save from the form. That is accepted.
- Removed: `vanilla-jsoneditor` (vendor directory, its stylesheet link in the layout head, and its service-worker shell entry) and `static/js/plan-editor.js`.

## 9. Problems on the fields

- The `/plans/preview` fragment also renders `@templ.JSONScript("plan-problems", problems)` (`[{pointer, message, warning}]`) next to the existing list.
- After htmx swaps `#plan-preview`, `plan-form.js` reads it, clears the previous marks and calls json-editor's `showValidationErrors` with translated paths.
- **Pointer to path** (pure, in `plan-form-core.js`): `/days/0/groups/1/exercises/0/sets/2/count` becomes `root.days.0.groups.1.exercises.0.sets.2.count`, with `~1` and `~0` unescaped. If there is no editor at that exact path, it walks up to the nearest one:
  - a per-week entry `…/count/2` lands on the count field as "W3: …"
  - `…/load/pct_tm` lands on the load field
  - document-level problems land at the top of the form
- Errors are red and warnings amber, in the theme's classes.
- Each item in the problem list becomes a button. Clicking it switches to the right day tab, expands collapsed sections and focuses the field.
- The textarea's preview request gets `hx-sync="this:replace"`, so a slow response for an older document can't overwrite a newer one's marks.
- In the JSON view, problems show only in the list, as today.

## 10. Files

| File | Change |
|---|---|
| `internal/plan/editor.go`, `editor.overlay.json`, `editor_test.go` | new: editor schema |
| `internal/web/plans.go`, `views/plans.templ`, `views/models.go` | editor schema on the editor page; problems JSON in the preview; Form/JSON switch markup; `hx-sync` |
| `internal/web/static/vendor/json-editor/jsoneditor.js`, `LICENSE` | new vendored library (2.17.2, MIT) |
| `internal/web/static/vendor/jsoneditor/` | removed (`vanilla-jsoneditor`) |
| `internal/web/static/js/plan-form.js` | new: editor setup, switch, problem marking (replaces `plan-editor.js`) |
| `internal/web/static/js/plan-form-core.js` | new: pure logic |
| `internal/web/static/js/plan-form-theme.js` | new: theme and per-week editor DOM |
| `internal/web/jstest/plan-form.test.mjs` | new: Node tests |
| `internal/web/views/layout.templ`, `static/js/sw.js`, `styles/input.css` | head scripts, shell files, Tailwind source |
| `test/e2e/plan-editor.mjs`, `Taskfile.yml` | new browser check, run by `task e2e` |
| `AGENTS.md` | layout and rules |

## 11. Risks and the probe

The first implementation task is a probe against the vendored library, before anything is built on it:

1. `$ref` to `#/$defs/...` resolves, or the rename to `definitions` is needed.
2. The `load` switcher works as a titled `oneOf` of single-key objects, including "none".
3. A custom editor can register for `format: "per-week"` via a resolver and watch `root.weeks`.
4. `showValidationErrors` accepts externally built paths and clears on the next call.
5. Grid layout and tabs render with the custom theme's classes.

If any of these fails in a way that needs a different design, work stops and the finding goes back to the user instead of being worked around silently.

## 12. Testing

- **Go (`internal/plan`):**
  - every overlay pointer exists in the contract
  - the starter template, the MCP format guide's example and the golden test plans validate under both the contract and the editor schema (the rewrites only relax or retitle)
  - the slug `enum` holds the user's catalog, custom exercises and slugs the document already uses
- **Go (`internal/web`):** the editor page embeds the editor schema, and the preview fragment carries `plan-problems`.
- **Node (`jstest/plan-form.test.mjs`):**
  - per-week resize, collapse, count skips, reps parsing and percent conversion
  - pointer-to-path mapping, including walking up
  - the "would the form change this document?" comparison
- **Browser (`test/e2e/plan-editor.mjs`):**
  1. a new plan renders the starter template in the form
  2. add a set line
  3. switch a count to "vary by week", change weeks from 4 to 5, and see it become five inputs
  4. type an invalid rep range and see the field marked
  5. save a draft, reopen it, and get the same JSON back
  6. switch from JSON with an unknown key and see the refusal
- **Manual:** screenshots in light and dark, at desktop and phone widths.

## 13. Unchanged

The plan contract, server validation (schema plus semantic checks), saving, versions, the compare page, `/schema/plan.json`, MCP (`get_plan_schema`, `validate_plan`, `save_plan_draft`) and the AI-facing format guide.
