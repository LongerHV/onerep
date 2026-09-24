package exercise

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"

	"github.com/LongerHV/onerep/internal/calc"
	"github.com/LongerHV/onerep/internal/store"
)

// Store is the persistence the service needs. *store.DB implements it.
type Store interface {
	CatalogExercises(ctx context.Context, userID string) ([]store.Exercise, error)
	ExerciseBySlug(ctx context.Context, userID, slug string) (store.Exercise, error)
	SaveUserExercise(ctx context.Context, userID string, e store.Exercise) (store.Exercise, error)
	DeleteUserExercise(ctx context.Context, userID, slug string) error
	Alternatives(ctx context.Context, userID, slug string) ([]store.Alternative, error)
	AddAlternative(ctx context.Context, userID, slug, altSlug string) error
	RemoveAlternative(ctx context.Context, userID, slug, altSlug string) error

	ListEquipment(ctx context.Context, userID string) ([]store.Equipment, error)
	EquipmentByID(ctx context.Context, userID, id string) (store.Equipment, error)
	DefaultEquipment(ctx context.Context, userID, kind string) (store.Equipment, error)
	SaveEquipment(ctx context.Context, e store.Equipment) (store.Equipment, error)
	DeleteEquipment(ctx context.Context, userID, id string) error
	InitStarterEquipment(ctx context.Context, userID string, items []store.Equipment) (bool, error)

	UserExercise(ctx context.Context, userID, slug string) (store.UserExercise, error)
	SetExerciseEquipment(ctx context.Context, userID, slug, equipmentID string) error
	SetTrainingMax(ctx context.Context, userID, slug string, newKg *float64, source, note string) error
	TrainingMaxHistory(ctx context.Context, userID, slug string) ([]store.TrainingMaxChange, error)
}

// Service implements the catalog and equipment rules.
type Service struct {
	Store Store
}

// Input is a user-defined exercise (new, or a customized seeded one).
type Input struct {
	Slug             string
	Name             string
	Measurement      string
	EquipmentKind    string
	PrimaryMuscles   []string
	SecondaryMuscles []string
	Aliases          []string
}

var slugRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func (in Input) validate() FieldErrors {
	errs := FieldErrors{}
	if !slugRE.MatchString(in.Slug) || len(in.Slug) > 64 {
		errs["slug"] = "use lowercase letters, digits and single dashes, like cable-row"
	}
	if name := strings.TrimSpace(in.Name); name == "" || len(name) > 100 {
		errs["name"] = "name is required (at most 100 characters)"
	}
	if !isMeasurement(in.Measurement) {
		errs["measurement"] = "choose how sets are recorded"
	}
	if !slices.Contains(calc.Kinds, in.EquipmentKind) {
		errs["equipment_kind"] = "choose the equipment"
	}
	if len(in.PrimaryMuscles) == 0 {
		errs["primary_muscles"] = "choose at least one primary muscle"
	}
	for _, m := range append(slices.Clone(in.PrimaryMuscles), in.SecondaryMuscles...) {
		if !isMuscle(m) {
			errs["primary_muscles"] = "unknown muscle " + m
		}
	}
	for _, m := range in.SecondaryMuscles {
		if slices.Contains(in.PrimaryMuscles, m) {
			errs["secondary_muscles"] = m + " is already a primary muscle"
		}
	}
	return errs
}

// Catalog returns the user's exercises whose name, slug, alias, muscle or
// equipment contains every word of query.
func (s *Service) Catalog(ctx context.Context, userID, query string) ([]store.Exercise, error) {
	all, err := s.Store.CatalogExercises(ctx, userID)
	if err != nil {
		return nil, err
	}
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return all, nil
	}
	var out []store.Exercise
	for _, e := range all {
		hay := strings.ToLower(strings.Join(append([]string{e.Name, e.Slug, e.EquipmentKind},
			append(append(slices.Clone(e.Aliases), e.PrimaryMuscles...), e.SecondaryMuscles...)...), " "))
		match := true
		for _, w := range words {
			if !strings.Contains(hay, w) {
				match = false
				break
			}
		}
		if match {
			out = append(out, e)
		}
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, userID, slug string) (store.Exercise, error) {
	return s.Store.ExerciseBySlug(ctx, userID, slug)
}

// Create adds a new user exercise. The slug must not be in use.
func (s *Service) Create(ctx context.Context, userID string, in Input) (store.Exercise, error) {
	in = normalize(in)
	errs := in.validate()
	if _, err := s.Store.ExerciseBySlug(ctx, userID, in.Slug); err == nil {
		errs["slug"] = "an exercise with this slug already exists"
	} else if !errors.Is(err, store.ErrNotFound) {
		return store.Exercise{}, err
	}
	if len(errs) > 0 {
		return store.Exercise{}, errs
	}
	return s.Store.SaveUserExercise(ctx, userID, toStore(in))
}

// Update changes an existing exercise. Updating a seeded exercise creates the
// user's customized copy.
func (s *Service) Update(ctx context.Context, userID string, in Input) (store.Exercise, error) {
	in = normalize(in)
	if _, err := s.Store.ExerciseBySlug(ctx, userID, in.Slug); err != nil {
		return store.Exercise{}, err
	}
	if errs := in.validate(); len(errs) > 0 {
		return store.Exercise{}, errs
	}
	return s.Store.SaveUserExercise(ctx, userID, toStore(in))
}

// Delete removes the user's own exercise, or restores a customized seeded one.
func (s *Service) Delete(ctx context.Context, userID, slug string) error {
	return s.Store.DeleteUserExercise(ctx, userID, slug)
}

func normalize(in Input) Input {
	in.Slug = strings.TrimSpace(in.Slug)
	in.Name = strings.TrimSpace(in.Name)
	var aliases []string
	for _, a := range in.Aliases {
		if a = strings.TrimSpace(a); a != "" && !slices.Contains(aliases, a) {
			aliases = append(aliases, a)
		}
	}
	in.Aliases = aliases
	return in
}

func toStore(in Input) store.Exercise {
	return store.Exercise{Slug: in.Slug, Name: in.Name, Measurement: in.Measurement, EquipmentKind: in.EquipmentKind,
		PrimaryMuscles: in.PrimaryMuscles, SecondaryMuscles: in.SecondaryMuscles, Aliases: in.Aliases}
}

// AlternativeView is an alternative with its exercise resolved.
type AlternativeView struct {
	Exercise  store.Exercise
	UserAdded bool
}

// Alternatives lists the alternatives of slug that exist in the user's catalog.
func (s *Service) Alternatives(ctx context.Context, userID, slug string) ([]AlternativeView, error) {
	alts, err := s.Store.Alternatives(ctx, userID, slug)
	if err != nil {
		return nil, err
	}
	var out []AlternativeView
	for _, a := range alts {
		e, err := s.Store.ExerciseBySlug(ctx, userID, a.Slug)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, AlternativeView{Exercise: e, UserAdded: a.UserAdded})
	}
	return out, nil
}

func (s *Service) AddAlternative(ctx context.Context, userID, slug, altSlug string) error {
	if slug == altSlug {
		return FieldErrors{"alternative": "an exercise cannot be its own alternative"}
	}
	for _, sl := range []string{slug, altSlug} {
		if _, err := s.Store.ExerciseBySlug(ctx, userID, sl); err != nil {
			return err
		}
	}
	return s.Store.AddAlternative(ctx, userID, slug, altSlug)
}

func (s *Service) RemoveAlternative(ctx context.Context, userID, slug, altSlug string) error {
	return s.Store.RemoveAlternative(ctx, userID, slug, altSlug)
}

// Settings is a user's view of one exercise's settings.
type Settings struct {
	store.UserExercise
	// Equipment is the profile used for rounding: the linked one, else the
	// default for the exercise's kind; nil if neither exists.
	Equipment *store.Equipment
	// Linked is true when Equipment comes from an explicit link.
	Linked bool
}

func (s *Service) Settings(ctx context.Context, userID string, ex store.Exercise) (Settings, error) {
	ue, err := s.Store.UserExercise(ctx, userID, ex.Slug)
	if err != nil {
		return Settings{}, err
	}
	st := Settings{UserExercise: ue}
	if ue.EquipmentID != "" {
		eq, err := s.Store.EquipmentByID(ctx, userID, ue.EquipmentID)
		if err != nil {
			return Settings{}, err
		}
		st.Equipment, st.Linked = &eq, true
		return st, nil
	}
	eq, err := s.Store.DefaultEquipment(ctx, userID, ex.EquipmentKind)
	if errors.Is(err, store.ErrNotFound) {
		return st, nil
	}
	if err != nil {
		return Settings{}, err
	}
	st.Equipment = &eq
	return st, nil
}

// LinkEquipment links slug to a profile; "" goes back to the kind's default.
func (s *Service) LinkEquipment(ctx context.Context, userID, slug, equipmentID string) error {
	if _, err := s.Store.ExerciseBySlug(ctx, userID, slug); err != nil {
		return err
	}
	return s.Store.SetExerciseEquipment(ctx, userID, slug, equipmentID)
}

// MaxTrainingMaxKg bounds training max input; heavier is a typo.
const MaxTrainingMaxKg = 1500

// SetTrainingMax sets (nil clears) the training max of slug.
func (s *Service) SetTrainingMax(ctx context.Context, userID, slug string, kg *float64, source string) error {
	if kg != nil && (*kg <= 0 || *kg > MaxTrainingMaxKg) {
		return FieldErrors{"training_max": "enter a positive weight"}
	}
	if _, err := s.Store.ExerciseBySlug(ctx, userID, slug); err != nil {
		return err
	}
	return s.Store.SetTrainingMax(ctx, userID, slug, kg, source, "")
}

func (s *Service) TrainingMaxHistory(ctx context.Context, userID, slug string) ([]store.TrainingMaxChange, error) {
	return s.Store.TrainingMaxHistory(ctx, userID, slug)
}

// PercentOfTM rounds pct of the training max to the exercise's equipment.
func (s *Service) PercentOfTM(ctx context.Context, user store.User, ex store.Exercise, pct float64) (calc.Rounded, error) {
	st, err := s.Settings(ctx, user.ID, ex)
	if err != nil {
		return calc.Rounded{}, err
	}
	if st.TrainingMaxKg == nil {
		return calc.Rounded{}, ErrNoTrainingMax
	}
	var eq *calc.Equipment
	if st.Equipment != nil {
		eq = &st.Equipment.Spec
	}
	r, _ := calc.ResolveLoad(calc.Load{PctTM: &pct}, 1, calc.LoadContext{TMKg: st.TrainingMaxKg, Equipment: eq, Unit: user.Unit})
	return r, nil
}

// Equipment profiles.

func (s *Service) ListEquipment(ctx context.Context, userID string) ([]store.Equipment, error) {
	return s.Store.ListEquipment(ctx, userID)
}

func (s *Service) GetEquipment(ctx context.Context, userID, id string) (store.Equipment, error) {
	return s.Store.EquipmentByID(ctx, userID, id)
}

// SaveEquipment validates and stores a profile (empty ID creates one).
func (s *Service) SaveEquipment(ctx context.Context, e store.Equipment) (store.Equipment, error) {
	errs := FieldErrors{}
	e.Name = strings.TrimSpace(e.Name)
	if e.Name == "" || len(e.Name) > 100 {
		errs["name"] = "name is required (at most 100 characters)"
	}
	if err := e.Spec.Validate(); err != nil {
		errs["config"] = err.Error()
	}
	if len(errs) > 0 {
		return store.Equipment{}, errs
	}
	return s.Store.SaveEquipment(ctx, e)
}

func (s *Service) DeleteEquipment(ctx context.Context, userID, id string) error {
	return s.Store.DeleteEquipment(ctx, userID, id)
}

// EnsureStarterEquipment gives a new user default barbell and dumbbell
// profiles in their unit. It does nothing once done, even if the user later
// deletes the profiles.
func (s *Service) EnsureStarterEquipment(ctx context.Context, user store.User) error {
	if user.EquipmentInitialized {
		return nil
	}
	_, err := s.Store.InitStarterEquipment(ctx, user.ID, StarterEquipment(user.Unit))
	return err
}

// StarterEquipment is the default equipment for a new user (spec §6).
func StarterEquipment(unit string) []store.Equipment {
	bar, plates, dumbbells := 20.0, []float64{25, 20, 15, 10, 5, 2.5, 1.25}, weightRange(2, 50, 2)
	if unit == calc.UnitLb {
		bar, plates, dumbbells = 45, []float64{45, 35, 25, 10, 5, 2.5}, weightRange(5, 100, 5)
	} else {
		unit = calc.UnitKg
	}
	return []store.Equipment{
		{Name: "Barbell", IsDefault: true, Spec: calc.Equipment{Kind: calc.KindBarbell, Unit: unit,
			Config: calc.EquipmentConfig{Bar: bar, Plates: plates}}},
		{Name: "Dumbbells", IsDefault: true, Spec: calc.Equipment{Kind: calc.KindDumbbell, Unit: unit,
			Config: calc.EquipmentConfig{Weights: dumbbells}}},
	}
}

func weightRange(lo, hi, step float64) []float64 {
	var out []float64
	for v := lo; v <= hi; v += step {
		out = append(out, v)
	}
	return out
}
