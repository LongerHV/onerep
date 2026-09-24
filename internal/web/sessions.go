package web

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/LongerHV/onerep/internal/auth"
	"github.com/LongerHV/onerep/internal/training"
	"github.com/LongerHV/onerep/internal/web/views"
)

func (s *Server) sessionRoutes(r chi.Router) {
	r.Post("/sessions", s.sessionStart)
	r.Get("/sessions/{id}/live", s.sessionLive)
	r.Post("/api/sync", s.apiSync)
	r.Get("/api/csrf", apiCSRF)
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
	writeJSON(w, http.StatusOK, map[string]string{"csrf": id.CSRFToken})
}

// serviceWorker serves /sw.js with its cache version set to a hash of the
// static files, so every release refreshes the offline copy.
func serviceWorker(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	src, _ := fs.ReadFile(staticFS, "static/js/sw.js")
	_, _ = w.Write([]byte(strings.ReplaceAll(string(src), "__VERSION__", staticVersion())))
}

var staticVersion = sync.OnceValue(func() string {
	h := sha256.New()
	_ = fs.WalkDir(staticFS, "static", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(staticFS, path)
		h.Write([]byte(path))
		h.Write(b)
		return err
	})
	return hex.EncodeToString(h.Sum(nil))[:12]
})

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
