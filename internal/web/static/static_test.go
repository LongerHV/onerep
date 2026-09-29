package static_test

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LongerHV/onerep/internal/web/static"
)

func get(t *testing.T, target string, header ...string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	rec := httptest.NewRecorder()
	static.Handler().ServeHTTP(rec, req)
	return rec.Result()
}

func TestURLIsContentHashed(t *testing.T) {
	u := static.URL("app.css")
	if !strings.HasPrefix(u, "/static/app.css?v=") || len(u) <= len("/static/app.css?v=") {
		t.Fatalf("URL(app.css) = %q", u)
	}
	if static.URL("js/calc.js") == static.URL("js/stats.js") {
		t.Fatal("different files share a version")
	}
}

func TestURLOfAMissingFilePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("URL of a missing file must panic, so a typo fails every page test")
		}
	}()
	static.URL("nope.css")
}

func TestVersionedURLIsImmutable(t *testing.T) {
	resp := get(t, static.URL("app.css"))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "immutable") || !strings.Contains(cc, "max-age=31536000") {
		t.Fatalf("Cache-Control %q", cc)
	}
}

func TestUnversionedURLIsRevalidated(t *testing.T) {
	for _, target := range []string{"/static/app.css", "/static/app.css?v=stale"} {
		resp := get(t, target)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: status %d", target, resp.StatusCode)
		}
		if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
			t.Fatalf("%s: Cache-Control %q", target, cc)
		}
		etag := resp.Header.Get("ETag")
		if etag == "" {
			t.Fatalf("%s: no ETag", target)
		}
		if again := get(t, target, "If-None-Match", etag); again.StatusCode != http.StatusNotModified {
			t.Fatalf("%s: revalidation got %d", target, again.StatusCode)
		}
	}
}

func TestSourceIsNotServed(t *testing.T) {
	if resp := get(t, "/static/static.go"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

// Module imports between static files ("./calc.js", the uPlot dynamic
// import) resolve to unversioned URLs; the import map sends them to the
// versioned ones.
func TestImportMapCoversEveryScript(t *testing.T) {
	var m struct {
		Imports map[string]string `json:"imports"`
	}
	if err := json.Unmarshal([]byte(static.ImportMap()), &m); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"js/calc.js", "js/companion-core.js", "js/chart-data.js", "js/plan-form-core.js",
		"js/plan-form-theme.js", "vendor/uplot/uPlot.esm.js", "vendor/json-editor/jsoneditor.js"} {
		if got := m.Imports["/static/"+name]; got != static.URL(name) {
			t.Errorf("import map %s = %q, want %q", name, got, static.URL(name))
		}
	}
	for k := range m.Imports {
		if !strings.HasSuffix(k, ".js") {
			t.Errorf("import map has non-script %s", k)
		}
	}
}

func TestURLsListsEveryFile(t *testing.T) {
	urls := static.URLs()
	if urls["/static/app.css"] != static.URL("app.css") || urls["/static/js/sw.js"] == "" {
		t.Fatalf("URLs() = %v", urls)
	}
	resp := get(t, urls["/static/js/calc.js"])
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(b), "export") {
		t.Fatal("calc.js body")
	}
}

func TestManifest(t *testing.T) {
	b, err := fs.ReadFile(static.FS, "manifest.webmanifest")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Name            string `json:"name"`
		Display         string `json:"display"`
		StartURL        string `json:"start_url"`
		ThemeColor      string `json:"theme_color"`
		BackgroundColor string `json:"background_color"`
		Icons           []struct {
			Src     string `json:"src"`
			Sizes   string `json:"sizes"`
			Purpose string `json:"purpose"`
		} `json:"icons"`
	}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if ct := get(t, static.URL("manifest.webmanifest")).Header.Get("Content-Type"); ct != "application/manifest+json" {
		t.Errorf("Content-Type %q", ct)
	}
	if m.Name != "onerep" || m.Display != "standalone" || m.StartURL != "/" || m.ThemeColor == "" || m.BackgroundColor == "" {
		t.Fatalf("manifest %+v", m)
	}
	have := map[string]bool{}
	for _, ic := range m.Icons {
		name, ok := strings.CutPrefix(ic.Src, "/static/")
		if !ok {
			t.Fatalf("icon %s is not under /static/", ic.Src)
		}
		if _, err := fs.Stat(static.FS, name); err != nil {
			t.Errorf("icon %s: %v", ic.Src, err)
		}
		have[ic.Sizes+" "+ic.Purpose] = true
	}
	for _, want := range []string{"192x192 any", "512x512 any", "512x512 maskable"} {
		if !have[want] {
			t.Errorf("manifest has no %s icon", want)
		}
	}
}

func TestVersionIsSet(t *testing.T) {
	if len(static.Version()) != 12 {
		t.Fatal("version")
	}
}
