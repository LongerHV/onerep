package training

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/LongerHV/onerep/internal/calc"
	"github.com/LongerHV/onerep/internal/plan"
	"github.com/LongerHV/onerep/internal/store"
)

// Bootstrap is everything the companion needs to run a session offline
// (spec §9). It is embedded in the live page as JSON.
type Bootstrap struct {
	Session   BootSession                `json:"session"`
	Snapshot  plan.ExpandedDay           `json:"snapshot"`
	Sets      []SetInput                 `json:"sets"`
	Exercises map[string]BootExercise    `json:"exercises"`
	Catalog   []CatalogEntry             `json:"catalog"`
	Defaults  map[string]*calc.Equipment `json:"equipment_defaults"` // by kind, for exercises added during the session
	Unit      string                     `json:"unit"`
}

type BootSession struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Week      int       `json:"week"`
	Day       int       `json:"day"`
	StartedAt time.Time `json:"started_at"`
	Finished  bool      `json:"finished"`
	Notes     string    `json:"notes"`
}

// BootExercise is what the companion knows about an exercise of the session.
type BootExercise struct {
	Name          string          `json:"name"`
	Measurement   string          `json:"measurement"`
	EquipmentKind string          `json:"equipment_kind"`
	TMKg          *float64        `json:"tm_kg,omitempty"`
	E1RMKg        *float64        `json:"e1rm_kg,omitempty"`
	Equipment     *calc.Equipment `json:"equipment,omitempty"`
	Alternatives  []string        `json:"alternatives"`
	Last          []SetInput      `json:"last"` // sets from the previous session with this exercise
}

// CatalogEntry is a catalog exercise the user can add or swap to.
type CatalogEntry struct {
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	Measurement   string `json:"measurement"`
	EquipmentKind string `json:"equipment_kind"`
}

// Bootstrap assembles the companion data for one of the user's sessions.
func (s *Service) Bootstrap(ctx context.Context, user store.User, sessionID string) (Bootstrap, error) {
	sess, err := s.Store.SessionByID(ctx, user.ID, sessionID)
	if err != nil {
		return Bootstrap{}, err
	}
	b := Bootstrap{
		Session: BootSession{ID: sess.ID, Name: sess.Name, Week: sess.Week, Day: sess.Day,
			StartedAt: sess.StartedAt, Finished: sess.FinishedAt != nil, Notes: sess.Notes},
		Exercises: map[string]BootExercise{},
		Defaults:  map[string]*calc.Equipment{},
		Unit:      user.Unit,
	}
	if err := json.Unmarshal(sess.Snapshot, &b.Snapshot); err != nil {
		return Bootstrap{}, err
	}
	sets, err := s.Store.SessionSets(ctx, user.ID, sess.ID)
	if err != nil {
		return Bootstrap{}, err
	}
	b.Sets = make([]SetInput, 0, len(sets))
	for _, set := range sets {
		b.Sets = append(b.Sets, toInput(set))
	}

	// Exercises of the session, their alternatives, and anything logged.
	var slugs []string
	planned := map[string]bool{}
	for _, g := range b.Snapshot.Groups {
		for _, slot := range g.Exercises {
			planned[slot.Slug] = true
			slugs = append(slugs, slot.Slug)
			slugs = append(slugs, slot.Alternatives...)
		}
	}
	for _, set := range sets {
		slugs = append(slugs, set.Slug)
	}
	for i := 0; i < len(slugs); i++ { // grows as catalog alternatives are found
		slug := slugs[i]
		if _, done := b.Exercises[slug]; done {
			continue
		}
		be, alts, err := s.bootExercise(ctx, user, sess.ID, slug)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return Bootstrap{}, err
		}
		b.Exercises[slug] = be
		if planned[slug] { // catalog alternatives of planned exercises, not of alternatives
			slugs = append(slugs, alts...)
		}
	}

	catalog, err := s.Exercises.Catalog(ctx, user.ID, "")
	if err != nil {
		return Bootstrap{}, err
	}
	for _, e := range catalog {
		b.Catalog = append(b.Catalog, CatalogEntry{Slug: e.Slug, Name: e.Name, Measurement: e.Measurement, EquipmentKind: e.EquipmentKind})
	}
	equipment, err := s.Exercises.ListEquipment(ctx, user.ID)
	if err != nil {
		return Bootstrap{}, err
	}
	for i, e := range equipment {
		if e.IsDefault {
			b.Defaults[e.Spec.Kind] = &equipment[i].Spec
		}
	}
	return b, nil
}

func (s *Service) bootExercise(ctx context.Context, user store.User, sessionID, slug string) (BootExercise, []string, error) {
	ex, err := s.Exercises.Get(ctx, user.ID, slug)
	if err != nil {
		return BootExercise{}, nil, err
	}
	st, err := s.Exercises.Settings(ctx, user.ID, ex)
	if err != nil {
		return BootExercise{}, nil, err
	}
	be := BootExercise{Name: ex.Name, Measurement: ex.Measurement, EquipmentKind: ex.EquipmentKind,
		TMKg: st.TrainingMaxKg, Alternatives: []string{}, Last: []SetInput{}}
	if st.Equipment != nil {
		be.Equipment = &st.Equipment.Spec
	}
	if be.E1RMKg, err = s.Store.BestE1RM(ctx, user.ID, slug, s.now().AddDate(0, 0, -user.E1RMWindowDays)); err != nil {
		return BootExercise{}, nil, err
	}
	alts, err := s.Exercises.Alternatives(ctx, user.ID, slug)
	if err != nil {
		return BootExercise{}, nil, err
	}
	for _, a := range alts {
		be.Alternatives = append(be.Alternatives, a.Exercise.Slug)
	}
	last, err := s.Store.LastSets(ctx, user.ID, slug, sessionID)
	if err != nil {
		return BootExercise{}, nil, err
	}
	for _, set := range last {
		be.Last = append(be.Last, toInput(set))
	}
	return be, be.Alternatives, nil
}

func toInput(s store.Set) SetInput {
	return SetInput{ID: s.ID, SessionID: s.SessionID, Slug: s.Slug, GroupPos: s.GroupPos, ExercisePos: s.ExercisePos,
		SetPos: s.SetPos, Kind: s.Kind, Prescribed: json.RawMessage(s.Prescribed), WeightKg: s.WeightKg, Reps: s.Reps,
		RPE: s.RPE, DurationS: s.DurationS, DistanceM: s.DistanceM, DoneAt: s.DoneAt, UpdatedAt: s.UpdatedAt}
}
