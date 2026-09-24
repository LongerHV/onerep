package web

import (
	"errors"
	"log/slog"
	"math"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/LongerHV/onerep/internal/calc"
	"github.com/LongerHV/onerep/internal/store"
	"github.com/LongerHV/onerep/internal/web/views"
)

// MuscleWeeks is how many ISO weeks the muscles page shows (spec §13).
const MuscleWeeks = 12

func (s *Server) statsRoutes(r chi.Router) {
	r.Get("/api/stats/exercises/{slug}/e1rm", s.apiE1RM)
	r.Get("/api/stats/muscles", s.apiMuscles)
}

// round1 rounds a chart value to 0.1.
func round1(v float64) float64 { return math.Round(v*10) / 10 }

type e1rmPoint struct {
	T        int64   `json:"t"` // unix seconds, what uPlot plots
	E1RM     float64 `json:"e1rm"`
	RPEBased bool    `json:"rpe_based"`
}

func (s *Server) apiE1RM(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	st, err := s.Stats.ExerciseStats(r.Context(), u, chi.URLParam(r, "slug"))
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "exercise not found"})
		return
	}
	if err != nil {
		s.apiFail(w, r, err)
		return
	}
	points := make([]e1rmPoint, 0, len(st.Series))
	for _, p := range st.Series {
		points = append(points, e1rmPoint{T: p.DoneAt.Unix(), E1RM: round1(calc.FromKg(p.E1RMKg, u.Unit)), RPEBased: p.RPEBased})
	}
	writeJSON(w, http.StatusOK, map[string]any{"unit": u.Unit, "points": points})
}

type muscleSeries struct {
	Muscle string    `json:"muscle"`
	Label  string    `json:"label"`
	Sets   []float64 `json:"sets"`
}

func (s *Server) apiMuscles(w http.ResponseWriter, r *http.Request) {
	mw, err := s.Stats.RecentMuscleSets(r.Context(), user(r), MuscleWeeks)
	if err != nil {
		s.apiFail(w, r, err)
		return
	}
	weeks := make([]string, 0, len(mw.Weeks))
	for _, wk := range mw.Weeks {
		weeks = append(weeks, wk.Label)
	}
	muscles := make([]muscleSeries, 0, len(mw.Muscles))
	for _, m := range mw.Muscles {
		muscles = append(muscles, muscleSeries{Muscle: m.Muscle, Label: views.Label(m.Muscle), Sets: m.Sets})
	}
	writeJSON(w, http.StatusOK, map[string]any{"weeks": weeks, "muscles": muscles})
}

// apiFail logs an unexpected error and answers with JSON, not an HTML page.
func (s *Server) apiFail(w http.ResponseWriter, r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "api request failed", "path", r.URL.Path, "err", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server error"})
}
