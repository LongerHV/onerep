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

	"github.com/LongerHV/onerep/internal/auth"
	"github.com/LongerHV/onerep/internal/store"
	"github.com/LongerHV/onerep/internal/web/views"
)

// Server holds the dependencies of the HTTP handlers.
type Server struct {
	DB       *store.DB
	Sessions *auth.Sessions
	OIDC     *auth.OIDC // nil when only the dev bypass is configured
	DevUser  string     // non-empty enables the dev login bypass
}

// Routes returns the application's HTTP handler.
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, logRequests, s.recoverer, s.Sessions.Middleware)
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		s.renderError(w, r, http.StatusNotFound, "Page not found.")
	})

	r.Get("/healthz", s.healthz)
	r.Handle("/static/*", staticHandler())
	r.Get("/auth/signed-out", func(w http.ResponseWriter, r *http.Request) {
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
		if s.DevUser != "" {
			r.Use(auth.DevLogin(s.Sessions, s.DevUser))
		}
		r.Use(auth.RequireUser, auth.CSRF)
		r.Get("/", s.home)
		r.Post("/auth/logout", s.logout)
	})
	return r
}

// page builds the common page data for r.
func page(r *http.Request, title string) views.Page {
	p := views.Page{Title: title}
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
				s.renderError(w, r, http.StatusInternalServerError, "Something went wrong.")
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

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	render(w, r, http.StatusOK, views.Home(page(r, "Home")))
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if err := s.Sessions.End(w, r); err != nil {
		slog.ErrorContext(r.Context(), "logout", "err", err)
	}
	http.Redirect(w, r, "/auth/signed-out", http.StatusSeeOther)
}
