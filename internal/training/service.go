// Package training runs workouts: starting sessions, applying the companion's
// offline operations (spec §9), and editing logged history.
package training

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"time"

	"github.com/LongerHV/onerep/internal/calc"
	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/plan"
	"github.com/LongerHV/onerep/internal/store"
)

// Store is the persistence the service needs. *store.DB implements it.
type Store interface {
	CreateSession(ctx context.Context, s store.Session) (store.Session, error)
	SessionByID(ctx context.Context, userID, id string) (store.Session, error)
	OpenSession(ctx context.Context, userID string) (store.Session, error)
	ListSessions(ctx context.Context, userID string, limit, offset int) ([]store.SessionSummary, error)
	DeleteSession(ctx context.Context, userID, id string) error
	SessionSets(ctx context.Context, userID, sessionID string) ([]store.Set, error)
	LastSets(ctx context.Context, userID, slug, excludeSessionID string) ([]store.Set, error)
	BestE1RM(ctx context.Context, userID, slug string, since time.Time) (*float64, error)
	UpsertSet(ctx context.Context, userID string, s store.Set, opID string) (store.Outcome, error)
	DeleteSet(ctx context.Context, userID, sessionID, setID string, at time.Time, opID string) (store.Outcome, error)
	SetSessionNotes(ctx context.Context, userID, sessionID, notes string, at time.Time, opID string) (store.Outcome, error)
	FinishSession(ctx context.Context, userID, sessionID string, at time.Time, opID string) (store.Outcome, error)
}

// Plans is what training needs from plans. *plan.Service implements it.
type Plans interface {
	Next(ctx context.Context, user store.User) (*plan.Next, error)
	AdvanceFrom(ctx context.Context, user store.User, planID string, week, day int) error
}

// Exercises is what training needs from the catalog. *exercise.Service implements it.
type Exercises interface {
	Get(ctx context.Context, userID, slug string) (store.Exercise, error)
	Catalog(ctx context.Context, userID, query string) ([]store.Exercise, error)
	Settings(ctx context.Context, userID string, ex store.Exercise) (exercise.Settings, error)
	Alternatives(ctx context.Context, userID, slug string) ([]exercise.AlternativeView, error)
	ListEquipment(ctx context.Context, userID string) ([]store.Equipment, error)
}

type Service struct {
	Store     Store
	Plans     Plans
	Exercises Exercises
	Now       func() time.Time // defaults to time.Now
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Errors.
var (
	// ErrNothingToStart: the user follows no plan, or has completed it.
	ErrNothingToStart = errors.New("no planned day to start")
)

// OpenSessionError is returned when starting while a session is still open.
type OpenSessionError struct{ ID string }

func (e OpenSessionError) Error() string { return "a workout is already in progress" }

func (s *Service) ensureNoOpen(ctx context.Context, user store.User) error {
	open, err := s.Store.OpenSession(ctx, user.ID)
	if err == nil {
		return OpenSessionError{ID: open.ID}
	}
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	return err
}

// StartPlanned starts the next day of the followed plan, snapshotting its
// resolved prescription.
func (s *Service) StartPlanned(ctx context.Context, user store.User) (store.Session, error) {
	if err := s.ensureNoOpen(ctx, user); err != nil {
		return store.Session{}, err
	}
	n, err := s.Plans.Next(ctx, user)
	if err != nil {
		return store.Session{}, err
	}
	if n == nil || n.Complete {
		return store.Session{}, ErrNothingToStart
	}
	snapshot, err := json.Marshal(n.Today)
	if err != nil {
		return store.Session{}, err
	}
	return s.Store.CreateSession(ctx, store.Session{
		UserID: user.ID, PlanID: n.Plan.ID, PlanVersionID: n.Version.ID,
		Week: n.Week, Day: n.Day, Name: n.Today.Name, Snapshot: snapshot,
	})
}

// StartAdHoc starts an empty workout; exercises are added as it goes.
func (s *Service) StartAdHoc(ctx context.Context, user store.User) (store.Session, error) {
	if err := s.ensureNoOpen(ctx, user); err != nil {
		return store.Session{}, err
	}
	snapshot, _ := json.Marshal(plan.ExpandedDay{Name: "Workout", Groups: []plan.ExpandedGroup{}})
	return s.Store.CreateSession(ctx, store.Session{UserID: user.ID, Name: "Workout", Snapshot: snapshot})
}

// Open returns the user's unfinished session, or ErrNotFound.
func (s *Service) Open(ctx context.Context, user store.User) (store.Session, error) {
	return s.Store.OpenSession(ctx, user.ID)
}

// SetInput is a set as sent by the companion or the history editor. Weights are kg.
type SetInput struct {
	ID          string          `json:"id"`
	SessionID   string          `json:"session_id"`
	Slug        string          `json:"slug"`
	GroupPos    int             `json:"group_pos"`
	ExercisePos int             `json:"exercise_pos"`
	SetPos      int             `json:"set_pos"`
	Kind        string          `json:"kind"`
	Prescribed  json.RawMessage `json:"prescribed,omitempty"`
	WeightKg    *float64        `json:"weight_kg"`
	Reps        *int            `json:"reps"`
	RPE         *float64        `json:"rpe"`
	DurationS   *int            `json:"duration_s"`
	DistanceM   *float64        `json:"distance_m"`
	DoneAt      *time.Time      `json:"done_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// InvalidError is returned for values a set or operation cannot have.
type InvalidError struct{ Reason string }

func (e InvalidError) Error() string { return e.Reason }

var idRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func finite(v *float64) bool { return v == nil || (!math.IsNaN(*v) && !math.IsInf(*v, 0)) }

func (in SetInput) validate() error {
	switch {
	case !idRE.MatchString(in.ID):
		return InvalidError{"set id must be a UUID"}
	case in.Kind != "warmup" && in.Kind != "working" && in.Kind != "drop" && in.Kind != "amrap":
		return InvalidError{"unknown set kind " + in.Kind}
	case !finite(in.WeightKg) || !finite(in.RPE) || !finite(in.DistanceM):
		return InvalidError{"numbers must be finite"}
	case in.WeightKg != nil && (*in.WeightKg < 0 || *in.WeightKg > 2000):
		return InvalidError{"weight out of range"}
	case in.Reps != nil && (*in.Reps < 0 || *in.Reps > 1000):
		return InvalidError{"reps out of range"}
	case in.RPE != nil && (*in.RPE < 1 || *in.RPE > 10):
		return InvalidError{"RPE must be between 1 and 10"}
	case in.DurationS != nil && (*in.DurationS < 0 || *in.DurationS > 86400):
		return InvalidError{"duration out of range"}
	case in.DistanceM != nil && (*in.DistanceM < 0 || *in.DistanceM > 1e6):
		return InvalidError{"distance out of range"}
	case in.GroupPos < 0 || in.ExercisePos < 0 || in.SetPos < 0 || in.GroupPos > 1000 || in.SetPos > 1000 || in.ExercisePos > 100:
		return InvalidError{"position out of range"}
	case len(in.Prescribed) > 4096:
		return InvalidError{"prescription too large"}
	}
	return nil
}

// toStore validates in and derives the e1RM from the exercise's measurement.
func (s *Service) toStore(ctx context.Context, user store.User, in SetInput) (store.Set, error) {
	if err := in.validate(); err != nil {
		return store.Set{}, err
	}
	ex, err := s.Exercises.Get(ctx, user.ID, in.Slug)
	if errors.Is(err, store.ErrNotFound) {
		return store.Set{}, InvalidError{"unknown exercise " + in.Slug}
	}
	if err != nil {
		return store.Set{}, err
	}
	set := store.Set{
		ID: in.ID, SessionID: in.SessionID, Slug: in.Slug, GroupPos: in.GroupPos, ExercisePos: in.ExercisePos,
		SetPos: in.SetPos, Kind: in.Kind, WeightKg: in.WeightKg, Reps: in.Reps, RPE: in.RPE,
		DurationS: in.DurationS, DistanceM: in.DistanceM, DoneAt: in.DoneAt, UpdatedAt: s.clamp(in.UpdatedAt),
	}
	if len(in.Prescribed) > 0 && string(in.Prescribed) != "null" {
		set.Prescribed = []byte(in.Prescribed)
	}
	if ex.Measurement == "weight_reps" && in.WeightKg != nil && in.Reps != nil {
		rpe := 0.0
		if in.RPE != nil {
			rpe = *in.RPE
		}
		if e1rm, _, ok := calc.E1RM(*in.WeightKg, *in.Reps, rpe); ok {
			set.E1RMKg = &e1rm
		}
	}
	return set, nil
}

// clamp keeps client clocks from pushing timestamps into the future (spec §9).
func (s *Service) clamp(t time.Time) time.Time {
	if limit := s.now().Add(5 * time.Minute); t.IsZero() || t.After(limit) {
		if t.IsZero() {
			return s.now()
		}
		return limit
	}
	return t
}

// Op is one queued companion operation (spec §9).
type Op struct {
	OpID     string          `json:"op_id"`
	Op       string          `json:"op"`
	Payload  json.RawMessage `json:"payload"`
	ClientTS time.Time       `json:"client_ts"`
}

// OpResult reports what happened to an operation.
type OpResult struct {
	OpID   string `json:"op_id"`
	Status string `json:"status"` // applied, duplicate or rejected
	Reason string `json:"reason,omitempty"`
}

// MaxOps bounds one sync request.
const MaxOps = 200

// Operation names.
const (
	OpUpsertSet     = "upsert_set"
	OpDeleteSet     = "delete_set"
	OpEditNotes     = "edit_notes"
	OpFinishSession = "finish_session"
)

// ApplyOps applies operations in order. A rejected operation does not stop
// the ones after it; the client keeps rejected ones for the user to see.
func (s *Service) ApplyOps(ctx context.Context, user store.User, ops []Op) ([]OpResult, error) {
	if len(ops) > MaxOps {
		return nil, InvalidError{fmt.Sprintf("at most %d operations per request", MaxOps)}
	}
	results := make([]OpResult, 0, len(ops))
	for _, op := range ops {
		res := OpResult{OpID: op.OpID}
		outcome, err := s.applyOp(ctx, user, op)
		var invalid InvalidError
		switch {
		case errors.As(err, &invalid):
			res.Status, res.Reason = "rejected", invalid.Reason
		case errors.Is(err, store.ErrNotFound):
			res.Status, res.Reason = "rejected", "unknown session or set"
		case err != nil:
			return results, err
		case outcome == store.Duplicate:
			res.Status = "duplicate"
		default:
			res.Status = "applied" // applied or ignored: either way the client is done with it
		}
		results = append(results, res)
	}
	return results, nil
}

func (s *Service) applyOp(ctx context.Context, user store.User, op Op) (store.Outcome, error) {
	if !idRE.MatchString(op.OpID) {
		return "", InvalidError{"op_id must be a UUID"}
	}
	switch op.Op {
	case OpUpsertSet:
		var in SetInput
		if err := json.Unmarshal(op.Payload, &in); err != nil {
			return "", InvalidError{"bad set: " + err.Error()}
		}
		set, err := s.toStore(ctx, user, in)
		if err != nil {
			return "", err
		}
		return s.Store.UpsertSet(ctx, user.ID, set, op.OpID)
	case OpDeleteSet:
		var p struct {
			ID        string    `json:"id"`
			SessionID string    `json:"session_id"`
			DeletedAt time.Time `json:"deleted_at"`
		}
		if err := json.Unmarshal(op.Payload, &p); err != nil || !idRE.MatchString(p.ID) {
			return "", InvalidError{"bad delete"}
		}
		return s.Store.DeleteSet(ctx, user.ID, p.SessionID, p.ID, s.clamp(p.DeletedAt), op.OpID)
	case OpEditNotes:
		var p struct {
			SessionID string    `json:"session_id"`
			Notes     string    `json:"notes"`
			UpdatedAt time.Time `json:"updated_at"`
		}
		if err := json.Unmarshal(op.Payload, &p); err != nil || len(p.Notes) > 10000 {
			return "", InvalidError{"bad notes"}
		}
		return s.Store.SetSessionNotes(ctx, user.ID, p.SessionID, p.Notes, s.clamp(p.UpdatedAt), op.OpID)
	case OpFinishSession:
		var p struct {
			SessionID  string    `json:"session_id"`
			FinishedAt time.Time `json:"finished_at"`
		}
		if err := json.Unmarshal(op.Payload, &p); err != nil {
			return "", InvalidError{"bad finish"}
		}
		return s.finish(ctx, user, p.SessionID, s.clamp(p.FinishedAt), op.OpID)
	}
	return "", InvalidError{"unknown operation " + op.Op}
}

// finish marks the session finished and, the first time, moves the plan
// cursor past the day it trained (spec §8).
func (s *Service) finish(ctx context.Context, user store.User, sessionID string, at time.Time, opID string) (store.Outcome, error) {
	out, err := s.Store.FinishSession(ctx, user.ID, sessionID, at, opID)
	if err != nil || out != store.Applied {
		return out, err
	}
	sess, err := s.Store.SessionByID(ctx, user.ID, sessionID)
	if err != nil || sess.PlanID == "" {
		return out, err
	}
	return out, s.Plans.AdvanceFrom(ctx, user, sess.PlanID, sess.Week, sess.Day)
}
