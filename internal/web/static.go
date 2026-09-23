package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static
var staticFS embed.FS

func staticHandler() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	files := http.StripPrefix("/static/", http.FileServerFS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		files.ServeHTTP(w, r)
	})
}
