package web

import (
	"bytes"
	"context"
	"html"
	"log/slog"
	"net/http"

	"github.com/a-h/templ"

	"github.com/LongerHV/onerep/internal/web/views"
)

// Pages render only their content. The layout middleware wraps a page in the
// full document for direct visits, reloads and htmx history restores, and
// sends just the content (plus its <title>, which htmx applies) to htmx
// navigation, which swaps it into <main>. Fragments and redirects pass
// through untouched.

type pageMetaKey struct{}

// pageMeta is filled in by page() while a handler renders.
type pageMeta struct {
	isPage bool
	page   views.Page
}

// markPage records that the response is a page, for the layout middleware.
func markPage(r *http.Request, p views.Page) {
	if m, ok := r.Context().Value(pageMetaKey{}).(*pageMeta); ok {
		m.isPage, m.page = true, p
	}
}

// bufferedWriter holds a response until the layout middleware decides how
// to send it, so status codes survive being wrapped in the layout.
type bufferedWriter struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (b *bufferedWriter) Header() http.Header { return b.header }

func (b *bufferedWriter) WriteHeader(status int) {
	if b.status == 0 {
		b.status = status
	}
}

func (b *bufferedWriter) Write(p []byte) (int, error) {
	if b.status == 0 {
		b.status = http.StatusOK
	}
	return b.body.Write(p)
}

func (b *bufferedWriter) sendHeaders(w http.ResponseWriter) {
	for k, vs := range b.header {
		w.Header()[k] = vs
	}
	if b.status == 0 {
		b.status = http.StatusOK
	}
	w.WriteHeader(b.status)
}

// partial reports whether r is htmx navigation that wants only the content.
func partial(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-History-Restore-Request") != "true"
}

func (s *Server) layout(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		meta := &pageMeta{}
		buf := &bufferedWriter{header: http.Header{}}
		next.ServeHTTP(buf, r.WithContext(context.WithValue(r.Context(), pageMetaKey{}, meta)))
		if !meta.isPage {
			buf.sendHeaders(w)
			_, _ = w.Write(buf.body.Bytes())
			return
		}
		buf.header.Add("Vary", "HX-Request")
		if partial(r) && buf.status >= 400 {
			// Show the error, but keep the URL of the page the user was on.
			buf.header.Set("HX-Push-Url", "false")
		}
		buf.sendHeaders(w)
		if partial(r) {
			_, _ = w.Write([]byte("<title>" + html.EscapeString(meta.page.Title) + " · onerep</title>"))
			_, _ = w.Write(buf.body.Bytes())
			return
		}
		ctx := templ.WithChildren(r.Context(), templ.Raw(buf.body.String()))
		if err := views.Layout(meta.page).Render(ctx, w); err != nil {
			slog.ErrorContext(r.Context(), "render layout", "err", err)
		}
	})
}
