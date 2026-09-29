package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"strings"
	"sync"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/LongerHV/onerep/internal/auth"
	"github.com/LongerHV/onerep/internal/training"
	"github.com/LongerHV/onerep/internal/web/static"
	"github.com/LongerHV/onerep/internal/web/views"
)

func (s *Server) sessionRoutes(r chi.Router) {
	r.Post("/sessions", s.sessionStart)
	r.Get("/sessions/{id}/live", s.sessionLive)
	r.Post("/api/sync", s.apiSync)
	r.Get("/api/sessions/{id}/sets", s.apiSessionSets)
	r.Get("/api/csrf", apiCSRF)
}

// apiSessionSets returns a session's sets, which the companion fetches after
// syncing to learn about sets deleted in history or on another device.
func (s *Server) apiSessionSets(w http.ResponseWriter, r *http.Request) {
	sets, err := s.Training.SessionSets(r.Context(), user(r), chi.URLParam(r, "id"))
	if err != nil {
		s.failJSON(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sets": sets})
}

func (s *Server) sessionStart(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	start := s.Training.StartPlanned
	if r.PostFormValue("kind") == "adhoc" {
		start = s.Training.StartAdHoc
	}
	sess, err := start(r.Context(), u)
	var open training.OpenSessionError
	switch {
	case errors.As(err, &open):
		http.Redirect(w, r, "/sessions/"+open.ID+"/live", http.StatusSeeOther)
	case errors.Is(err, training.ErrNothingToStart):
		s.renderError(w, r, http.StatusConflict, "There is no planned day to start: follow a plan, or start an empty workout.")
	case err != nil:
		s.fail(w, r, err)
	default:
		http.Redirect(w, r, "/sessions/"+sess.ID+"/live", http.StatusSeeOther)
	}
}

func (s *Server) sessionLive(w http.ResponseWriter, r *http.Request) {
	boot, err := s.Training.Bootstrap(r.Context(), user(r), chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	render(w, r, http.StatusOK, views.SessionLive(page(r, boot.Session.Name), boot))
}

type syncRequest struct {
	Ops []training.Op `json:"ops"`
}

type syncResponse struct {
	Results []training.OpResult `json:"results"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// apiSync applies the companion's queued operations (spec §9).
func (s *Server) apiSync(w http.ResponseWriter, r *http.Request) {
	var req syncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	results, err := s.Training.ApplyOps(r.Context(), user(r), req.Ops)
	var invalid training.InvalidError
	switch {
	case errors.As(err, &invalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": invalid.Reason})
	case err != nil:
		s.failJSON(w, r, err)
	default:
		writeJSON(w, http.StatusOK, syncResponse{Results: results})
	}
}

// apiCSRF returns the session's CSRF token, for a page loaded from the
// offline cache after the user signed in again.
func apiCSRF(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	// user_id lets the companion refuse a token of another user's session.
	writeJSON(w, http.StatusOK, map[string]string{"csrf": id.CSRFToken, "user_id": id.User.ID})
}

// serviceWorker serves /sw.js with its cache version set to shellVersion, so
// every release refreshes the offline copy, and with the hashed URLs of the
// static files, so it caches exactly what the pages reference.
var serviceWorker = sync.OnceValue(func() http.HandlerFunc {
	src, err := fs.ReadFile(static.FS, "js/sw.js")
	if err != nil {
		panic(err)
	}
	assets, err := json.Marshal(static.URLs())
	if err != nil {
		panic(err)
	}
	body := []byte(strings.NewReplacer("__VERSION__", shellVersion(), "{/*ASSETS*/}", string(assets)).Replace(string(src)))
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(body)
	}
})

// shellVersion covers what the service worker caches: the static files and
// the /offline page with its layout.
var shellVersion = sync.OnceValue(func() string {
	return shellVersionOf(static.Version(), offlineDocument())
})

func shellVersionOf(staticVersion string, offline []byte) string {
	h := sha256.New()
	h.Write([]byte(staticVersion))
	h.Write([]byte{0})
	h.Write(offline)
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// offlineDocument renders the /offline page as an anonymous visitor sees it.
func offlineDocument() []byte {
	p := views.Page{Title: "Offline"}
	var content, doc bytes.Buffer
	ctx := context.Background()
	if err := views.Offline(p).Render(ctx, &content); err != nil {
		panic(err)
	}
	if err := views.Layout(p).Render(templ.WithChildren(ctx, templ.Raw(content.String())), &doc); err != nil {
		panic(err)
	}
	return doc.Bytes()
}

func (s *Server) offline(w http.ResponseWriter, r *http.Request) {
	render(w, r, http.StatusOK, views.Offline(page(r, "Offline")))
}

// newSetID is a UUIDv7 for sets created in the history editor.
func newSetID() string {
	id, err := uuid.NewV7()
	if err != nil {
		panic(err) // crypto/rand failed
	}
	return id.String()
}
