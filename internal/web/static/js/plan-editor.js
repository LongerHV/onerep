// Upgrades the plan editor's <textarea id="doc"> to vanilla-jsoneditor with
// schema validation. The textarea stays the form field: every change is
// copied back and announced with a "doc-changed" event, which htmx uses to
// refresh the server-side preview. Without JS the textarea works on its own.
import { createAjvValidator, createJSONEditor } from "/static/vendor/jsoneditor/standalone.min.js";

const textarea = document.getElementById("doc");
const holder = document.getElementById("doc-editor");
const schemaEl = document.getElementById("plan-schema");

if (textarea && holder && schemaEl) {
  // The editor's validator (Ajv) predates draft 2020-12; the schema only uses
  // keywords both understand, so drop the $schema marker.
  const { $schema: _draft, ...schema } = JSON.parse(schemaEl.textContent);
  if (window.matchMedia("(prefers-color-scheme: dark)").matches) {
    holder.classList.add("jse-theme-dark");
  }
  textarea.hidden = true;
  holder.hidden = false;
  createJSONEditor({
    target: holder,
    props: {
      content: { text: textarea.value },
      mode: "text",
      validator: createAjvValidator({ schema }),
      onChange(content) {
        textarea.value = content.text !== undefined ? content.text : JSON.stringify(content.json, null, 2);
        textarea.dispatchEvent(new Event("doc-changed", { bubbles: true }));
      },
    },
  });
}
