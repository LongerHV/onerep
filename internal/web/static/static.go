// Package static embeds the files served under /static/ and gives each one a
// content-hashed URL ("/static/app.css?v=<hash>"), which is served as
// immutable. Pages reference files through URL, and module imports between
// scripts reach the hashed URLs through ImportMap.
package static

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"
	"sync"
)

// Every served file or directory must be listed here; Go files are not.
//
//go:embed app.css manifest.webmanifest icons js vendor
var files embed.FS

// FS holds the static files, named as under /static/.
var FS fs.FS = files

type index struct {
	hashes  map[string]string // name → content hash
	version string            // hash of every name and content
}

var idx = sync.OnceValue(func() index {
	ix := index{hashes: map[string]string{}}
	all := sha256.New()
	err := fs.WalkDir(files, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(files, name)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		ix.hashes[name] = hex.EncodeToString(sum[:])[:12]
		all.Write([]byte(name))
		all.Write(sum[:])
		return nil
	})
	if err != nil {
		panic(err)
	}
	ix.version = hex.EncodeToString(all.Sum(nil))[:12]
	return ix
})

// URL returns the content-hashed URL of a static file, named relative to
// /static/ ("js/calc.js"). It panics for a file that doesn't exist.
func URL(name string) string {
	h, ok := idx().hashes[name]
	if !ok {
		panic("static: no file " + name)
	}
	return "/static/" + name + "?v=" + h
}

// URLs maps every file's plain URL ("/static/app.css") to its hashed one.
func URLs() map[string]string {
	m := make(map[string]string, len(idx().hashes))
	for name := range idx().hashes {
		m["/static/"+name] = URL(name)
	}
	return m
}

// ImportMap is the page's import map: it sends module imports of the plain
// script URLs ("./calc.js", resolved to /static/js/calc.js) to the hashed
// ones, so imported modules are versioned too.
var ImportMap = sync.OnceValue(func() string {
	imports := map[string]string{}
	for plain, hashed := range URLs() {
		if strings.HasSuffix(plain, ".js") {
			imports[plain] = hashed
		}
	}
	b, err := json.Marshal(map[string]any{"imports": imports})
	if err != nil {
		panic(err)
	}
	return string(b)
})

// Favicon answers /favicon.ico, which browsers and other clients request
// without looking at the page.
func Favicon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/x-icon")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFileFS(w, r, FS, "icons/favicon.ico")
}

// Version is a hash of all static files.
func Version() string { return idx().version }

// Handler serves /static/*. A request carrying the file's current hash is
// cached for a year as immutable; any other (a plain URL, or an old page's
// hash) must be revalidated, which the ETag makes cheap.
func Handler() http.Handler {
	files := http.StripPrefix("/static/", http.FileServerFS(FS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/static/")
		if h, ok := idx().hashes[name]; ok {
			w.Header().Set("ETag", `"`+h+`"`)
			if r.URL.Query().Get("v") == h {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
		}
		if strings.HasSuffix(name, ".webmanifest") {
			w.Header().Set("Content-Type", "application/manifest+json")
		}
		files.ServeHTTP(w, r)
	})
}
