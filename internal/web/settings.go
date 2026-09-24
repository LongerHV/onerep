package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/LongerHV/onerep/internal/account"
	"github.com/LongerHV/onerep/internal/web/views"
)

func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	render(w, r, http.StatusOK, views.SettingsPage(page(r, "Settings"), r.URL.Query().Has("saved"), ""))
}

func (s *Server) saveSettings(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.PostFormValue("e1rm_window_days"))
	err := s.Account.UpdateSettings(r.Context(), user(r).ID, r.PostFormValue("unit"), days)
	if errors.Is(err, account.ErrInvalidSettings) {
		render(w, r, http.StatusUnprocessableEntity, views.SettingsPage(page(r, "Settings"), false, err.Error()))
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/settings?saved", http.StatusSeeOther)
}
