package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/LongerHV/onerep/internal/auth"
	"github.com/LongerHV/onerep/internal/web/views"
)

func (s *Server) tokenRoutes(r chi.Router) {
	r.Get("/settings/tokens", s.tokensPage)
	r.Post("/settings/tokens", s.tokenCreate)
	r.Post("/settings/tokens/{id}/revoke", s.tokenRevoke)
}

func (s *Server) tokensData(r *http.Request) (views.TokensData, error) {
	list, err := s.Tokens.List(r.Context(), user(r).ID)
	return views.TokensData{Tokens: list, MCPURL: strings.TrimSuffix(s.BaseURL, "/") + "/mcp"}, err
}

func (s *Server) tokensPage(w http.ResponseWriter, r *http.Request) {
	d, err := s.tokensData(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	render(w, r, http.StatusOK, views.TokensPage(page(r, "API tokens"), d))
}

// tokenCreate shows the new token once. The response is never cached, and
// hx-history="false" keeps htmx from saving the page in its history cache.
func (s *Server) tokenCreate(w http.ResponseWriter, r *http.Request) {
	secret, _, err := s.Tokens.Create(r.Context(), user(r).ID, r.PostFormValue("name"))
	d, derr := s.tokensData(r)
	switch {
	case derr != nil:
		s.fail(w, r, derr)
	case errors.Is(err, auth.ErrTokenName):
		d.Error = err.Error()
		render(w, r, http.StatusUnprocessableEntity, views.TokensPage(page(r, "API tokens"), d))
	case err != nil:
		s.fail(w, r, err)
	default:
		d.Secret = secret
		w.Header().Set("Cache-Control", "no-store")
		render(w, r, http.StatusOK, views.TokensPage(page(r, "API tokens"), d))
	}
}

func (s *Server) tokenRevoke(w http.ResponseWriter, r *http.Request) {
	if err := s.Tokens.Revoke(r.Context(), user(r).ID, chi.URLParam(r, "id")); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/settings/tokens", http.StatusSeeOther)
}
