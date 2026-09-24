// Package web wires HTTP routes, middleware, and page handlers.
package web

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/LongerHV/onerep/internal/account"
	"github.com/LongerHV/onerep/internal/auth"
	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/plan"
	"github.com/LongerHV/onerep/internal/stats"
	"github.com/LongerHV/onerep/internal/store"
	"github.com/LongerHV/onerep/internal/training"
	"github.com/LongerHV/onerep/internal/web/views"
)

// Server holds the dependencies of the HTTP handlers.
type Server struct {
	DB       *store.DB
	Sessions *auth.Sessions
	OIDC     *auth.OIDC // nil when only the dev bypass is configured
	DevUser  string     // non-empty enables the dev login bypass
	BaseURL  string     // public URL, for links shown to users
	Tokens   *auth.Tokens

	Exercises *exercise.Service
	Account   *account.Service
	Plans     *plan.Service
	Training  *training.Service
	Stats     *stats.Service
}

// Routes returns the application's HTTP handler.
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, logRequests, s.recoverer, s.Sessions.Middleware)
	r.NotFound(s.layout(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.renderError(w, r, http.StatusNotFound, "Page not found.")
	})).ServeHTTP)

	r.Get("/healthz", s.healthz)
	r.Get("/schema/plan.json", planSchema)
	r.Get("/sw.js", serviceWorker)
	r.With(s.layout).Get("/offline", s.offline)
	r.Handle("/static/*", staticHandler())
	r.With(s.layout).Get("/auth/signed-out", func(w http.ResponseWriter, r *http.Request) {
		render(w, r, http.StatusOK, views.SignedOut(page(r, "Signed out")))
	})
	if s.OIDC != nil {
		r.Get("/auth/login", s.OIDC.Login)
		r.Get("/auth/callback", s.OIDC.Callback)
	} else {
		r.Get("/auth/login", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/", http.StatusSeeOther)
		})
	}

	r.Group(func(r chi.Router) {
		r.Use(s.layout)
		if s.DevUser != "" {
			r.Use(auth.DevLogin(s.Sessions, s.DevUser))
		}
		r.Use(auth.RequireUser, s.limitBody, auth.CSRF, s.starterEquipment)
		r.Get("/", s.home)
		r.Post("/auth/logout", s.logout)
		s.exerciseRoutes(r)
		s.planRoutes(r)
		s.sessionRoutes(r)
		s.historyRoutes(r)
		s.statsRoutes(r)
		s.equipmentRoutes(r)
		r.Get("/settings", s.settings)
		r.Post("/settings", s.saveSettings)
		s.tokenRoutes(r)
	})
	return r
}

// page builds the data for a page titled title and marks the response as a
// page, so the layout middleware wraps it (see layout.go).
func page(r *http.Request, title string) views.Page {
	p := fragmentPage(r)
	p.Title = title
	markPage(r, p)
	return p
}

// fragmentPage is the page data for rendering a fragment, which the layout
// middleware leaves alone.
func fragmentPage(r *http.Request) views.Page {
	var p views.Page
	if id, ok := auth.FromContext(r.Context()); ok {
		p.Identity = &id
	}
	return p
}

func render(w http.ResponseWriter, r *http.Request, status int, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := c.Render(r.Context(), w); err != nil {
		slog.ErrorContext(r.Context(), "render", "err", err, "request_id", middleware.GetReqID(r.Context()))
	}
}

func (s *Server) renderError(w http.ResponseWriter, r *http.Request, status int, message string) {
	render(w, r, status, views.Error(page(r, http.StatusText(status)), status, message, middleware.GetReqID(r.Context())))
}

// recoverer turns panics into a logged 500 page carrying the request ID.
func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(v)
				}
				slog.ErrorContext(r.Context(), "panic", "value", v, "request_id", middleware.GetReqID(r.Context()))
				s.layout(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					s.renderError(w, r, http.StatusInternalServerError, "Something went wrong.")
				})).ServeHTTP(w, r)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		next.ServeHTTP(ww, r)
		slog.InfoContext(r.Context(), "request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", middleware.GetReqID(r.Context()))
	})
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	if err := s.DB.Ping(r.Context()); err != nil {
		slog.ErrorContext(r.Context(), "healthz", "err", err)
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if err := s.Sessions.End(w, r); err != nil {
		slog.ErrorContext(r.Context(), "logout", "err", err)
	}
	http.Redirect(w, r, "/auth/signed-out", http.StatusSeeOther)
}

// user returns the signed-in user. Only call it behind auth.RequireUser.
func user(r *http.Request) store.User {
	id, _ := auth.FromContext(r.Context())
	return id.User
}

// fail renders the error page for err: 404 for missing (or other users')
// resources, 500 otherwise.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrNotFound) {
		s.renderError(w, r, http.StatusNotFound, "Not found.")
		return
	}
	slog.ErrorContext(r.Context(), "request failed", "err", err, "request_id", middleware.GetReqID(r.Context()))
	s.renderError(w, r, http.StatusInternalServerError, "Something went wrong.")
}

// starterEquipment creates a new user's starter equipment on their first request.
func (s *Server) starterEquipment(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := s.Exercises.EnsureStarterEquipment(r.Context(), user(r)); err != nil {
			s.fail(w, r, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// maxBodyBytes bounds request bodies; the largest legitimate one is a plan document.
const maxBodyBytes = 1 << 20

// limitBody refuses oversized requests before anything reads them. Form posts
// are parsed here so the limit applies before the CSRF check reads the token.
func (s *Server) limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		if r.Method == http.MethodPost {
			var tooLarge *http.MaxBytesError
			if err := r.ParseForm(); errors.As(err, &tooLarge) {
				s.renderError(w, r, http.StatusRequestEntityTooLarge, "The request is too large (at most 1 MB).")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// failJSON is fail for JSON endpoints (spec §16).
func (s *Server) failJSON(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	id := middleware.GetReqID(r.Context())
	slog.ErrorContext(r.Context(), "request failed", "err", err, "request_id", id)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "something went wrong", "request_id": id})
}
