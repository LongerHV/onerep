// Package stats turns logged sets into progress views (spec §13): e1RM over
// time, rep-max PRs and weekly hard sets per muscle. Everything is in kg; the
// web layer converts to the user's unit.
package stats

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/LongerHV/onerep/internal/calc"
	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/store"
)

// MaxRepMax is the highest rep count in the rep-max table.
const MaxRepMax = 12

// Store is the persistence the service needs. *store.DB implements it.
type Store interface {
	RepMaxes(ctx context.Context, userID, slug, excludeSessionID string) ([]store.RepMax, error)
	E1RMSeries(ctx context.Context, userID, slug string) ([]store.E1RMPoint, error)
	SessionPRs(ctx context.Context, userID, sessionID string) (map[string]bool, error)
	HardSets(ctx context.Context, userID string, from, to time.Time) ([]store.HardSet, error)
}

// Exercises resolves the user's view of an exercise. *exercise.Service implements it.
type Exercises interface {
	Get(ctx context.Context, userID, slug string) (store.Exercise, error)
}

type Service struct {
	Store     Store
	Exercises Exercises
	Now       func() time.Time // defaults to time.Now
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Point is one session's best e1RM.
type Point struct {
	SessionID string
	DoneAt    time.Time
	E1RMKg    float64
	RPEBased  bool // from the RTS table; otherwise estimated from reps
}

// ExerciseStats is what the exercise page (and the AI) shows about progress.
type ExerciseStats struct {
	Exercise store.Exercise
	Series   []Point
	RepMaxes []store.RepMax // rep counts 1–MaxRepMax
}

// ExerciseStats returns the e1RM series and rep maxes of one of the user's exercises.
func (s *Service) ExerciseStats(ctx context.Context, user store.User, slug string) (ExerciseStats, error) {
	ex, err := s.Exercises.Get(ctx, user.ID, slug)
	if err != nil {
		return ExerciseStats{}, err
	}
	st := ExerciseStats{Exercise: ex}
	points, err := s.Store.E1RMSeries(ctx, user.ID, slug)
	if err != nil {
		return st, err
	}
	for _, p := range points {
		st.Series = append(st.Series, Point{SessionID: p.SessionID, DoneAt: p.DoneAt, E1RMKg: p.E1RMKg, RPEBased: isRPEBased(p)})
	}
	maxes, err := s.Store.RepMaxes(ctx, user.ID, slug, "")
	if err != nil {
		return st, err
	}
	for _, m := range maxes {
		if m.Reps <= MaxRepMax {
			st.RepMaxes = append(st.RepMaxes, m)
		}
	}
	return st, nil
}

// isRPEBased reports whether calc derived the point's e1RM from the RTS table.
func isRPEBased(p store.E1RMPoint) bool {
	if p.RPE == nil {
		return false
	}
	_, method, ok := calc.E1RM(p.WeightKg, p.Reps, *p.RPE)
	return ok && method == calc.MethodRPE
}

// SessionPRs returns the ids of a session's PR sets (spec §13).
func (s *Service) SessionPRs(ctx context.Context, user store.User, sessionID string) (map[string]bool, error) {
	return s.Store.SessionPRs(ctx, user.ID, sessionID)
}

// Week is an ISO week, starting Monday 00:00 UTC.
type Week struct {
	Label string // "2026-W39"
	Start time.Time
}

func weekOf(t time.Time) Week {
	t = t.UTC()
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	start := day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
	year, week := start.ISOWeek()
	return Week{Label: fmt.Sprintf("%d-W%02d", year, week), Start: start}
}

// MuscleVolume is one muscle's hard sets per week, aligned with MuscleWeeks.Weeks.
type MuscleVolume struct {
	Muscle string
	Sets   []float64
	Total  float64
}

// MuscleWeeks is weekly hard sets per muscle. Muscles without sets are left
// out; the rest are sorted by total, most first.
type MuscleWeeks struct {
	Weeks   []Week
	Muscles []MuscleVolume
}

// WeeklyMuscleSets counts hard sets per muscle for the ISO weeks from the
// week of from to the week of to, inclusive. Primary muscles count 1, each
// secondary muscle 0.5 (spec §13).
func (s *Service) WeeklyMuscleSets(ctx context.Context, user store.User, from, to time.Time) (MuscleWeeks, error) {
	var out MuscleWeeks
	last := weekOf(to).Start
	for w := weekOf(from).Start; !w.After(last); w = w.AddDate(0, 0, 7) {
		out.Weeks = append(out.Weeks, weekOf(w))
	}
	if len(out.Weeks) == 0 {
		return out, nil
	}
	sets, err := s.Store.HardSets(ctx, user.ID, out.Weeks[0].Start, last.AddDate(0, 0, 7))
	if err != nil {
		return out, err
	}
	exercises := map[string]*store.Exercise{} // nil: unknown to this user
	volume := map[string][]float64{}
	add := func(muscle string, week int, v float64) {
		if volume[muscle] == nil {
			volume[muscle] = make([]float64, len(out.Weeks))
		}
		volume[muscle][week] += v
	}
	for _, set := range sets {
		ex, seen := exercises[set.Slug]
		if !seen {
			got, err := s.Exercises.Get(ctx, user.ID, set.Slug)
			switch {
			case errors.Is(err, store.ErrNotFound):
			case err != nil:
				return out, err
			default:
				ex = &got
			}
			exercises[set.Slug] = ex
		}
		if ex == nil {
			continue
		}
		week := int(weekOf(set.DoneAt).Start.Sub(out.Weeks[0].Start).Hours() / (24 * 7))
		for _, m := range ex.PrimaryMuscles {
			add(m, week, 1)
		}
		for _, m := range ex.SecondaryMuscles {
			add(m, week, 0.5)
		}
	}
	for muscle, weeks := range volume {
		mv := MuscleVolume{Muscle: muscle, Sets: weeks}
		for _, v := range weeks {
			mv.Total += v
		}
		out.Muscles = append(out.Muscles, mv)
	}
	order := func(m string) int {
		if i := slices.Index(exercise.Muscles, m); i >= 0 {
			return i
		}
		return len(exercise.Muscles)
	}
	slices.SortFunc(out.Muscles, func(a, b MuscleVolume) int {
		if a.Total != b.Total {
			if a.Total > b.Total {
				return -1
			}
			return 1
		}
		return order(a.Muscle) - order(b.Muscle)
	})
	return out, nil
}

// RecentMuscleSets is WeeklyMuscleSets for the last weeks ISO weeks, this one included.
func (s *Service) RecentMuscleSets(ctx context.Context, user store.User, weeks int) (MuscleWeeks, error) {
	now := s.now()
	return s.WeeklyMuscleSets(ctx, user, now.AddDate(0, 0, -7*(weeks-1)), now)
}
