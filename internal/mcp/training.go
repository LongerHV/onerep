package mcp

import (
	"context"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/LongerHV/onerep/internal/stats"
	"github.com/LongerHV/onerep/internal/store"
)

type listSessionsIn struct {
	From     string `json:"from,omitempty" jsonschema:"first day, YYYY-MM-DD (UTC)"`
	To       string `json:"to,omitempty" jsonschema:"last day, YYYY-MM-DD (UTC), inclusive"`
	Exercise string `json:"exercise,omitempty" jsonschema:"only sessions with sets of this exercise slug"`
	Limit    int    `json:"limit,omitempty" jsonschema:"at most this many sessions (default 20, max 100)"`
}

type sessionSummaryOut struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	StartedAt  string   `json:"started_at"`
	FinishedAt string   `json:"finished_at,omitempty"`
	PlanWeek   int      `json:"plan_week,omitempty"`
	PlanDay    int      `json:"plan_day,omitempty" jsonschema:"0-based day index in the plan week"`
	Sets       int      `json:"sets"`
	Exercises  []string `json:"exercises"`
	Notes      string   `json:"notes,omitempty"`
}

type listSessionsOut struct {
	Sessions []sessionSummaryOut `json:"sessions" jsonschema:"newest first"`
}

type idIn struct {
	ID string `json:"id"`
}

type setOut struct {
	ID          string   `json:"id"`
	Slug        string   `json:"slug"`
	GroupPos    int      `json:"group_pos"`
	ExercisePos int      `json:"exercise_pos"`
	SetPos      int      `json:"set_pos"`
	Kind        string   `json:"kind"`
	WeightKg    *float64 `json:"weight_kg"`
	Reps        *int     `json:"reps"`
	RPE         *float64 `json:"rpe"`
	DurationS   *int     `json:"duration_s,omitempty"`
	DistanceM   *float64 `json:"distance_m,omitempty"`
	E1RMKg      *float64 `json:"e1rm_kg,omitempty"`
	DoneAt      string   `json:"done_at,omitempty"`
	PR          bool     `json:"pr" jsonschema:"heavier than every earlier set of this exercise at these reps"`
}

type sessionOut struct {
	sessionSummaryOut
	Unit       string   `json:"unit"`
	Prescribed any      `json:"prescribed" jsonschema:"the planned day as it was when the session started (loads in kg)"`
	SetsDone   []setOut `json:"sets" jsonschema:"what was actually done"`
}

type slugIn struct {
	Slug string `json:"slug"`
}

type pointOut struct {
	Date      string   `json:"date"`
	SessionID string   `json:"session_id"`
	E1RMKg    float64  `json:"e1rm_kg"`
	RPEBased  bool     `json:"rpe_based"`
	WeightKg  float64  `json:"weight_kg"`
	Reps      int      `json:"reps"`
	RPE       *float64 `json:"rpe"`
}

type repMaxOut struct {
	Reps      int     `json:"reps"`
	WeightKg  float64 `json:"weight_kg"`
	Date      string  `json:"date"`
	SessionID string  `json:"session_id"`
}

type exerciseStatsOut struct {
	Slug           string      `json:"slug"`
	Name           string      `json:"name"`
	Unit           string      `json:"unit"`
	TrainingMaxKg  *float64    `json:"training_max_kg"`
	E1RMSeries     []pointOut  `json:"e1rm_series" jsonschema:"best estimated 1RM of each workout, oldest first"`
	RepMaxes       []repMaxOut `json:"rep_maxes" jsonschema:"heaviest working set per rep count, 1-12"`
	RecentBestSets []pointOut  `json:"recent_best_sets" jsonschema:"best set of each of the last 10 workouts, newest first"`
}

type volumeIn struct {
	From string `json:"from,omitempty" jsonschema:"a day in the first ISO week, YYYY-MM-DD (default: 11 weeks before to)"`
	To   string `json:"to,omitempty" jsonschema:"a day in the last ISO week, YYYY-MM-DD (default: today)"`
}

type muscleOut struct {
	Muscle string    `json:"muscle"`
	Sets   []float64 `json:"sets" jsonschema:"hard sets per week, aligned with weeks"`
	Total  float64   `json:"total"`
}

type volumeOut struct {
	Weeks   []string    `json:"weeks" jsonschema:"ISO weeks like 2026-W39, oldest first"`
	Muscles []muscleOut `json:"muscles" jsonschema:"most volume first; a hard set counts 1 for each primary muscle and 0.5 for each secondary"`
}

// day parses a YYYY-MM-DD tool argument (UTC).
func day(name, v string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return time.Time{}, inputError(name + " must be a date like 2026-09-24 (YYYY-MM-DD)")
	}
	return t, nil
}

func summaryOf(s store.SessionSummary) sessionSummaryOut {
	out := sessionSummaryOut{ID: s.ID, Name: s.Name, StartedAt: ts(s.StartedAt), PlanWeek: s.Week, PlanDay: s.Day,
		Sets: s.Sets, Exercises: append([]string{}, s.Slugs...), Notes: s.Notes}
	if s.FinishedAt != nil {
		out.FinishedAt = ts(*s.FinishedAt)
	}
	return out
}

func pointOf(p stats.Point) pointOut {
	return pointOut{Date: ts(p.DoneAt), SessionID: p.SessionID, E1RMKg: p.E1RMKg, RPEBased: p.RPEBased,
		WeightKg: p.WeightKg, Reps: p.Reps, RPE: p.RPE}
}

func (s *Server) addTrainingTools(srv *sdk.Server) {
	tool(s, srv, &sdk.Tool{Name: "list_sessions", Description: "The user's workouts, newest first, optionally by date range and exercise."},
		func(ctx context.Context, u store.User, in listSessionsIn) (listSessionsOut, error) {
			f := store.SessionFilter{Slug: in.Exercise, Limit: in.Limit}
			var err error
			if in.From != "" {
				if f.From, err = day("from", in.From); err != nil {
					return listSessionsOut{}, err
				}
			}
			if in.To != "" {
				if f.To, err = day("to", in.To); err != nil {
					return listSessionsOut{}, err
				}
				f.To = f.To.AddDate(0, 0, 1)
			}
			list, err := s.Training.Sessions(ctx, u, f)
			if err != nil {
				return listSessionsOut{}, err
			}
			out := listSessionsOut{Sessions: []sessionSummaryOut{}}
			for _, sum := range list {
				out.Sessions = append(out.Sessions, summaryOf(sum))
			}
			return out, nil
		})
	tool(s, srv, &sdk.Tool{Name: "get_session", Description: "One workout: what was prescribed and every set actually done, with PRs marked."},
		func(ctx context.Context, u store.User, in idIn) (sessionOut, error) {
			sess, sets, err := s.Training.Session(ctx, u, in.ID)
			if err != nil {
				return sessionOut{}, err
			}
			prs, err := s.Stats.SessionPRs(ctx, u, sess.ID)
			if err != nil {
				return sessionOut{}, err
			}
			out := sessionOut{sessionSummaryOut: summaryOf(store.SessionSummary{Session: sess, Sets: len(sets)}), Unit: u.Unit,
				Prescribed: jsonValue(sess.Snapshot), SetsDone: []setOut{}}
			seen := map[string]bool{}
			for _, x := range sets {
				if !seen[x.Slug] {
					seen[x.Slug] = true
					out.Exercises = append(out.Exercises, x.Slug)
				}
				so := setOut{ID: x.ID, Slug: x.Slug, GroupPos: x.GroupPos, ExercisePos: x.ExercisePos, SetPos: x.SetPos, Kind: x.Kind,
					WeightKg: x.WeightKg, Reps: x.Reps, RPE: x.RPE, DurationS: x.DurationS, DistanceM: x.DistanceM, E1RMKg: x.E1RMKg, PR: prs[x.ID]}
				if x.DoneAt != nil {
					so.DoneAt = ts(*x.DoneAt)
				}
				out.SetsDone = append(out.SetsDone, so)
			}
			return out, nil
		})
	tool(s, srv, &sdk.Tool{Name: "get_exercise_stats", Description: "Progress on one exercise: e1RM over time, rep maxes, recent best sets and the current training max."},
		func(ctx context.Context, u store.User, in slugIn) (exerciseStatsOut, error) {
			st, err := s.Stats.ExerciseStats(ctx, u, in.Slug)
			if err != nil {
				return exerciseStatsOut{}, err
			}
			settings, err := s.Exercises.Settings(ctx, u.ID, st.Exercise)
			if err != nil {
				return exerciseStatsOut{}, err
			}
			out := exerciseStatsOut{Slug: st.Exercise.Slug, Name: st.Exercise.Name, Unit: u.Unit, TrainingMaxKg: settings.TrainingMaxKg,
				E1RMSeries: []pointOut{}, RepMaxes: []repMaxOut{}, RecentBestSets: []pointOut{}}
			for _, p := range st.Series {
				out.E1RMSeries = append(out.E1RMSeries, pointOf(p))
			}
			for i := len(st.Series) - 1; i >= 0 && len(out.RecentBestSets) < 10; i-- {
				out.RecentBestSets = append(out.RecentBestSets, pointOf(st.Series[i]))
			}
			for _, m := range st.RepMaxes {
				out.RepMaxes = append(out.RepMaxes, repMaxOut{Reps: m.Reps, WeightKg: m.WeightKg, Date: ts(m.DoneAt), SessionID: m.SessionID})
			}
			return out, nil
		})
	tool(s, srv, &sdk.Tool{Name: "get_weekly_muscle_volume", Description: "Hard sets per muscle per ISO week (working, drop and AMRAP sets at RPE 7+ or without RPE). Default: the last 12 weeks; at most 104."},
		func(ctx context.Context, u store.User, in volumeIn) (volumeOut, error) {
			to := time.Now().UTC()
			var err error
			if in.To != "" {
				if to, err = day("to", in.To); err != nil {
					return volumeOut{}, err
				}
			}
			from := to.AddDate(0, 0, -7*11)
			if in.From != "" {
				if from, err = day("from", in.From); err != nil {
					return volumeOut{}, err
				}
			}
			if to.Before(from) || to.Sub(from) > 104*7*24*time.Hour {
				return volumeOut{}, inputError("from must be before to, and the range at most 104 weeks")
			}
			mw, err := s.Stats.WeeklyMuscleSets(ctx, u, from, to)
			if err != nil {
				return volumeOut{}, err
			}
			out := volumeOut{Weeks: []string{}, Muscles: []muscleOut{}}
			for _, w := range mw.Weeks {
				out.Weeks = append(out.Weeks, w.Label)
			}
			for _, m := range mw.Muscles {
				out.Muscles = append(out.Muscles, muscleOut{Muscle: m.Muscle, Sets: m.Sets, Total: m.Total})
			}
			return out, nil
		})
}
