package auth

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"net/url"
)

const (
	// CSRFHeader carries the CSRF token on htmx and fetch requests.
	CSRFHeader = "X-CSRF-Token"
	// CSRFField carries the CSRF token on plain form posts.
	CSRFField = "csrf_token"
	// DevIssuer is the issuer recorded for users created by the dev bypass.
	DevIssuer = "dev"
)

// RequireUser rejects unauthenticated requests. Page loads are redirected to
// the login page, htmx requests get HX-Redirect, everything else gets 401.
func RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := FromContext(r.Context()); ok {
			next.ServeHTTP(w, r)
			return
		}
		login := "/auth/login?next=" + url.QueryEscape(r.URL.RequestURI())
		switch {
		case r.Header.Get("HX-Request") == "true":
			w.Header().Set("HX-Redirect", login)
			w.WriteHeader(http.StatusUnauthorized)
		case r.Method == http.MethodGet || r.Method == http.MethodHead:
			http.Redirect(w, r, login, http.StatusSeeOther)
		default:
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		}
	})
}

// CSRF requires the session's CSRF token on every unsafe request made with a session.
// It must run after Sessions.Middleware.
func CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		id, ok := FromContext(r.Context())
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		token := r.Header.Get(CSRFHeader)
		if token == "" {
			token = r.PostFormValue(CSRFField)
		}
		if subtle.ConstantTimeCompare([]byte(token), []byte(id.CSRFToken)) != 1 {
			http.Error(w, "invalid CSRF token", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// DevLogin signs every unauthenticated request in as the given dev user,
// creating the user and a real session on first use. Only for ONEREP_ENV=dev.
// It must run after Sessions.Middleware.
func DevLogin(s *Sessions, username string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := FromContext(r.Context()); ok {
				next.ServeHTTP(w, r)
				return
			}
			user, err := s.Store.UpsertOIDCUser(r.Context(), DevIssuer, username, username+"@localhost", username)
			if err != nil {
				http.Error(w, "dev login failed", http.StatusInternalServerError)
				return
			}
			id, err := s.Start(r.Context(), w, user)
			if err != nil {
				http.Error(w, "dev login failed", http.StatusInternalServerError)
				return
			}
			slog.DebugContext(r.Context(), "dev login", "user", username)
			next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), id)))
		})
	}
}
