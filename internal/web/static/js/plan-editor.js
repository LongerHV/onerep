// Upgrades the plan editor's <textarea id="doc"> to vanilla-jsoneditor with
// schema validation. Loaded once from the layout: navigation swaps pages into
// <main>, so the editor is set up from htmx.onLoad whenever one appears, and
// the 1.2 MB editor bundle is only fetched the first time. The textarea stays
// the form field: every change is copied back and announced with a
// "doc-changed" event, which htmx uses to refresh the server-side preview.
// Without JS (or if the editor fails to start) the textarea works on its own.

let bundle; // the editor module, loaded on first use

async function upgrade(textarea) {
  const holder = document.getElementById("doc-editor");
  const schemaEl = document.getElementById("plan-schema");
  if (!holder || !schemaEl || textarea.dataset.editor) return;
  textarea.dataset.editor = "starting";
  try {
    bundle ??= await import("/static/vendor/jsoneditor/standalone.min.js");
    // The editor's validator (Ajv) predates draft 2020-12; the schema only
    // uses keywords both understand, so drop the $schema marker.
    const { $schema: _draft, ...schema } = JSON.parse(schemaEl.textContent);
    if (window.matchMedia("(prefers-color-scheme: dark)").matches) {
      holder.classList.add("jse-theme-dark");
    }
    const editor = bundle.createJSONEditor({
      target: holder,
      props: {
        content: { text: textarea.value },
        mode: "text",
        validator: bundle.createAjvValidator({ schema }),
        onChange(content) {
          textarea.value = content.text !== undefined ? content.text : JSON.stringify(content.json, null, 2);
          textarea.dispatchEvent(new Event("doc-changed", { bubbles: true }));
        },
      },
    });
    // Only now swap the visible field; on failure the textarea stays usable.
    textarea.hidden = true;
    holder.hidden = false;
    textarea.dataset.editor = "ready";
    // Free the editor when htmx swaps the page away.
    // (htmx only signals cleanup for elements it processed: the textarea.)
    textarea.addEventListener("htmx:beforeCleanupElement", () => editor.destroy(), { once: true });
  } catch (err) {
    console.error("plan editor unavailable, using the plain text field", err);
    textarea.dataset.editor = "failed";
  }
}

htmx.onLoad((root) => {
  const textarea = root.id === "doc" ? root : root.querySelector?.("#doc");
  if (textarea) upgrade(textarea);
});
