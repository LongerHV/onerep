package plan

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/LongerHV/onerep/internal/calc"
	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/store"
)

//go:embed templates/starter.json
var starterTemplate []byte

// StarterTemplate is the document a new plan starts from.
func StarterTemplate() []byte { return starterTemplate }

// Store is the persistence the service needs. *store.DB implements it.
type Store interface {
	CreatePlan(ctx context.Context, userID, name string, doc []byte, status, source, note string) (store.Plan, store.PlanVersion, error)
	SavePlanVersion(ctx context.Context, userID, planID, name string, doc []byte, status, source, note string) (store.PlanVersion, error)
	ListPlans(ctx context.Context, userID string) ([]store.Plan, error)
	PlanByID(ctx context.Context, userID, id string) (store.Plan, error)
	SetPlanArchived(ctx context.Context, userID, id string, archived bool) error
	PlanVersions(ctx context.Context, userID, planID string) ([]store.PlanVersion, error)
	PlanVersionByID(ctx context.Context, userID, versionID string) (store.PlanVersion, error)
	ActivePlanVersion(ctx context.Context, userID, planID string) (store.PlanVersion, error)
	ActivateVersion(ctx context.Context, userID, versionID, name string) error
	DeleteDraft(ctx context.Context, userID, versionID string) error
	ActivePlan(ctx context.Context, userID string) (store.ActivePlan, error)
	SetActivePlan(ctx context.Context, userID string, a store.ActivePlan) error
	ClearActivePlan(ctx context.Context, userID string) error
}

// Exercises is what plans need from the catalog. *exercise.Service implements it.
type Exercises interface {
	Get(ctx context.Context, userID, slug string) (store.Exercise, error)
	Settings(ctx context.Context, userID string, ex store.Exercise) (exercise.Settings, error)
}

// History supplies estimated 1RMs from logged sets. *store.DB implements it.
type History interface {
	BestE1RM(ctx context.Context, userID, slug string, since time.Time) (*float64, error)
}

// Service implements plan rules on top of the store.
type Service struct {
	Store     Store
	Exercises Exercises
	History   History // optional: without it RPE loads stay unresolved
}

// ErrNoActiveVersion is returned when following a plan that has only drafts.
var ErrNoActiveVersion = errors.New("the plan has no active version yet")

// Save statuses.
const (
	SaveDraft    = store.PlanDraft
	SaveActivate = store.PlanActive
)

// catalog answers validation and resolution questions for one user, caching
// lookups for the duration of a request.
type catalog struct {
	ctx   context.Context
	svc   *Service
	user  store.User
	cache map[string]*slugInfo
}

type slugInfo struct {
	ex       store.Exercise
	exists   bool
	settings exercise.Settings
	e1rm     *float64
}

func (s *Service) catalogFor(ctx context.Context, user store.User) *catalog {
	return &catalog{ctx: ctx, svc: s, user: user, cache: map[string]*slugInfo{}}
}

func (c *catalog) info(slug string) *slugInfo {
	if i, ok := c.cache[slug]; ok {
		return i
	}
	i := &slugInfo{}
	if ex, err := c.svc.Exercises.Get(c.ctx, c.user.ID, slug); err == nil {
		i.ex, i.exists = ex, true
		if st, err := c.svc.Exercises.Settings(c.ctx, c.user.ID, ex); err == nil {
			i.settings = st
		}
		if c.svc.History != nil {
			since := time.Now().AddDate(0, 0, -c.user.E1RMWindowDays)
			if best, err := c.svc.History.BestE1RM(c.ctx, c.user.ID, slug, since); err == nil {
				i.e1rm = best
			}
		}
	}
	c.cache[slug] = i
	return i
}

func (c *catalog) Exercise(slug string) (bool, bool) {
	i := c.info(slug)
	return i.exists, i.ex.Hidden
}

func (c *catalog) HasTrainingMax(slug string) bool { return c.info(slug).settings.TrainingMaxKg != nil }

func (c *catalog) loadContext(slug string) calc.LoadContext {
	i := c.info(slug)
	st := i.settings
	ctx := calc.LoadContext{TMKg: st.TrainingMaxKg, E1RMKg: i.e1rm, Unit: c.user.Unit}
	if st.Equipment != nil {
		ctx.Equipment = &st.Equipment.Spec
	}
	return ctx
}

// Validate checks a document against the schema and the user's catalog.
func (s *Service) Validate(ctx context.Context, user store.User, raw []byte) (Doc, Problems) {
	return Validate(raw, s.catalogFor(ctx, user))
}

// Create stores a new plan from raw. With status SaveActivate it becomes the
// plan's active version. Invalid documents return Problems.
func (s *Service) Create(ctx context.Context, user store.User, raw []byte, status, source, note string) (store.Plan, store.PlanVersion, error) {
	doc, ps := s.Validate(ctx, user, raw)
	if ps.HasErrors() {
		return store.Plan{}, store.PlanVersion{}, ps
	}
	return s.Store.CreatePlan(ctx, user.ID, doc.Name, pinUnit(raw, doc, user.Unit), status, source, note)
}

// Save stores raw as the next version of planID. Activating a new version of
// the followed plan keeps the cursor when that day still exists (spec §8).
func (s *Service) Save(ctx context.Context, user store.User, planID string, raw []byte, status, source, note string) (store.PlanVersion, bool, error) {
	doc, ps := s.Validate(ctx, user, raw)
	if ps.HasErrors() {
		return store.PlanVersion{}, false, ps
	}
	v, err := s.Store.SavePlanVersion(ctx, user.ID, planID, doc.Name, pinUnit(raw, doc, user.Unit), status, source, note)
	if err != nil || status != SaveActivate {
		return v, false, err
	}
	reset, err := s.fitCursor(ctx, user, planID, doc)
	return v, reset, err
}

// compact stores documents without insignificant whitespace but keeps key order.
func compact(raw []byte) []byte {
	var b bytes.Buffer
	if err := json.Compact(&b, raw); err != nil {
		return raw
	}
	return b.Bytes()
}

// pinUnit compacts raw and, when the plan has no "unit", adds the user's so
// its absolute weights keep their meaning if the user later changes unit.
func pinUnit(raw []byte, doc Doc, unit string) []byte {
	c := compact(raw)
	if doc.Unit != "" || len(c) < 2 || c[0] != '{' {
		return c
	}
	field, _ := json.Marshal(unit)
	out := append([]byte(`{"unit":`), field...)
	if c[1] != '}' {
		out = append(out, ',')
	}
	return append(out, c[1:]...)
}

// Pretty indents a stored document for editing.
func Pretty(raw []byte) string {
	var b bytes.Buffer
	if err := json.Indent(&b, raw, "", "  "); err != nil {
		return string(raw)
	}
	return b.String()
}

// Decode parses a stored (already validated) document.
func Decode(raw []byte) (Doc, error) {
	var doc Doc
	err := json.Unmarshal(raw, &doc)
	return doc, err
}

// keepsCursor reports whether a cursor at (week, day) still makes sense in
// doc: a training day, or the "complete" position just past the last week.
func keepsCursor(doc Doc, week, day int) bool {
	return ValidPosition(doc, week, day) || (week == doc.Weeks+1 && day == 0)
}

// fitCursor resets the cursor of a followed plan whose day no longer exists.
// It reports whether it reset.
func (s *Service) fitCursor(ctx context.Context, user store.User, planID string, doc Doc) (bool, error) {
	a, err := s.Store.ActivePlan(ctx, user.ID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && a.PlanID != planID) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if keepsCursor(doc, a.Week, a.Day) {
		return false, nil
	}
	return true, s.Store.SetActivePlan(ctx, user.ID, store.ActivePlan{PlanID: planID, Week: 1, Day: 0})
}

// PlanSummary is a plan as listed.
type PlanSummary struct {
	store.Plan
	Active    *store.PlanVersion // nil if the plan has only drafts
	Drafts    int
	Following bool
}

func (s *Service) Plans(ctx context.Context, user store.User) ([]PlanSummary, error) {
	plans, err := s.Store.ListPlans(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	following, err := s.Store.ActivePlan(ctx, user.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	out := make([]PlanSummary, 0, len(plans))
	for _, p := range plans {
		versions, err := s.Store.PlanVersions(ctx, user.ID, p.ID)
		if err != nil {
			return nil, err
		}
		sum := PlanSummary{Plan: p, Following: following.PlanID == p.ID}
		for i, v := range versions {
			switch v.Status {
			case store.PlanActive:
				sum.Active = &versions[i]
			case store.PlanDraft:
				sum.Drafts++
			}
		}
		out = append(out, sum)
	}
	return out, nil
}

// Plan returns a plan with its versions, newest first.
func (s *Service) Plan(ctx context.Context, user store.User, planID string) (store.Plan, []store.PlanVersion, error) {
	p, err := s.Store.PlanByID(ctx, user.ID, planID)
	if err != nil {
		return store.Plan{}, nil, err
	}
	vs, err := s.Store.PlanVersions(ctx, user.ID, planID)
	return p, vs, err
}

// Version returns a version of one of the user's plans.
func (s *Service) Version(ctx context.Context, user store.User, versionID string) (store.PlanVersion, error) {
	return s.Store.PlanVersionByID(ctx, user.ID, versionID)
}

// Activate makes a version active. It reports whether the cursor of the
// followed plan had to be reset because its day no longer exists.
func (s *Service) Activate(ctx context.Context, user store.User, versionID string) (bool, error) {
	v, err := s.Store.PlanVersionByID(ctx, user.ID, versionID)
	if err != nil {
		return false, err
	}
	doc, err := Decode(v.Doc)
	if err != nil {
		return false, err
	}
	if err := s.Store.ActivateVersion(ctx, user.ID, versionID, doc.Name); err != nil {
		return false, err
	}
	return s.fitCursor(ctx, user, v.PlanID, doc)
}

// Discard deletes a draft.
func (s *Service) Discard(ctx context.Context, user store.User, versionID string) error {
	return s.Store.DeleteDraft(ctx, user.ID, versionID)
}

// Follow makes planID the user's plan, starting at week 1, day 1.
func (s *Service) Follow(ctx context.Context, user store.User, planID string) error {
	if _, err := s.Store.ActivePlanVersion(ctx, user.ID, planID); errors.Is(err, store.ErrNotFound) {
		if _, perr := s.Store.PlanByID(ctx, user.ID, planID); perr != nil {
			return perr
		}
		return ErrNoActiveVersion
	} else if err != nil {
		return err
	}
	return s.Store.SetActivePlan(ctx, user.ID, store.ActivePlan{PlanID: planID, Week: 1, Day: 0})
}

func (s *Service) Unfollow(ctx context.Context, user store.User) error {
	return s.Store.ClearActivePlan(ctx, user.ID)
}

// Archive hides or restores a plan. Archiving the followed plan stops following it.
func (s *Service) Archive(ctx context.Context, user store.User, planID string, archived bool) error {
	if err := s.Store.SetPlanArchived(ctx, user.ID, planID, archived); err != nil {
		return err
	}
	if a, err := s.Store.ActivePlan(ctx, user.ID); archived && err == nil && a.PlanID == planID {
		return s.Store.ClearActivePlan(ctx, user.ID)
	}
	return nil
}

// Day expands and resolves (week, day) of doc for the user, with exercise
// names filled in. ok is false when the day does not exist.
func (s *Service) Day(ctx context.Context, user store.User, doc Doc, week, day int) (ExpandedDay, bool) {
	return s.catalogFor(ctx, user).day(doc, week, day)
}

func (c *catalog) day(doc Doc, week, day int) (ExpandedDay, bool) {
	d, ok := Expand(doc, week, day, c.user.Unit)
	if !ok {
		return d, false
	}
	for gi := range d.Groups {
		for si := range d.Groups[gi].Exercises {
			slot := &d.Groups[gi].Exercises[si]
			if i := c.info(slot.Slug); i.exists {
				slot.Name = i.ex.Name
			}
		}
	}
	Resolve(&d, c.loadContext)
	return d, true
}

// Preview expands and resolves every day of every week.
func (s *Service) Preview(ctx context.Context, user store.User, doc Doc) [][]ExpandedDay {
	c := s.catalogFor(ctx, user)
	weeks := make([][]ExpandedDay, 0, doc.Weeks)
	for w := 1; w <= doc.Weeks; w++ {
		var days []ExpandedDay
		for d := range DaysForWeek(doc, w) {
			day, _ := c.day(doc, w, d)
			days = append(days, day)
		}
		weeks = append(weeks, days)
	}
	return weeks
}

// Next is where the user is in the followed plan.
type Next struct {
	Plan     store.Plan
	Version  store.PlanVersion
	Doc      Doc
	Week     int
	Day      int
	Complete bool        // past the last week
	Today    ExpandedDay // the next training day (zero when complete)
}

// Next returns the user's position in their plan, or nil if they follow none.
func (s *Service) Next(ctx context.Context, user store.User) (*Next, error) {
	a, err := s.Store.ActivePlan(ctx, user.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p, err := s.Store.PlanByID(ctx, user.ID, a.PlanID)
	if err != nil {
		return nil, err
	}
	v, err := s.Store.ActivePlanVersion(ctx, user.ID, a.PlanID)
	if err != nil {
		return nil, err
	}
	doc, err := Decode(v.Doc)
	if err != nil {
		return nil, err
	}
	n := &Next{Plan: p, Version: v, Doc: doc, Week: a.Week, Day: a.Day}
	if a.Week > doc.Weeks {
		n.Complete = true
		return n, nil
	}
	if n.Today, _ = s.Day(ctx, user, doc, a.Week, a.Day); n.Today.Name == "" {
		// The stored position no longer exists; start the block over.
		n.Week, n.Day = 1, 0
		n.Today, _ = s.Day(ctx, user, doc, 1, 0)
	}
	return n, nil
}

// Skip moves the cursor past the next day without training it.
func (s *Service) Skip(ctx context.Context, user store.User) error {
	n, err := s.Next(ctx, user)
	if err != nil || n == nil || n.Complete {
		return err
	}
	w, d := NextPosition(n.Doc, n.Week, n.Day)
	return s.Store.SetActivePlan(ctx, user.ID, store.ActivePlan{PlanID: n.Plan.ID, Week: w, Day: d})
}

// ErrNoSuchDay is returned when choosing a day the plan does not have.
var ErrNoSuchDay = errors.New("the plan has no such day")

// Choose moves the cursor to (week, day).
func (s *Service) Choose(ctx context.Context, user store.User, week, day int) error {
	n, err := s.Next(ctx, user)
	if err != nil {
		return err
	}
	if n == nil || !ValidPosition(n.Doc, week, day) {
		return ErrNoSuchDay
	}
	return s.Store.SetActivePlan(ctx, user.ID, store.ActivePlan{PlanID: n.Plan.ID, Week: week, Day: day})
}

// Restart moves the cursor back to week 1, day 1.
func (s *Service) Restart(ctx context.Context, user store.User) error {
	a, err := s.Store.ActivePlan(ctx, user.ID)
	if err != nil {
		return err
	}
	return s.Store.SetActivePlan(ctx, user.ID, store.ActivePlan{PlanID: a.PlanID, Week: 1, Day: 0})
}

// Comparison is a version compared with the plan's active version.
type Comparison struct {
	Base   *store.PlanVersion // nil when the plan has no active version
	Target store.PlanVersion
	JSON   []DiffLine
	Days   []DayChange // only days whose prescription changed
	// ResetsCursor is true when activating Target would move the followed
	// plan back to week 1, day 1.
	ResetsCursor bool
}

// DayChange is the prescription diff of one (week, day name).
type DayChange struct {
	Week  int
	Name  string
	Lines []DiffLine
}

// Compare diffs a version against the plan's active version.
func (s *Service) Compare(ctx context.Context, user store.User, versionID string) (Comparison, error) {
	target, err := s.Store.PlanVersionByID(ctx, user.ID, versionID)
	if err != nil {
		return Comparison{}, err
	}
	cmp := Comparison{Target: target}
	var baseDoc Doc
	var baseRaw []byte
	if base, err := s.Store.ActivePlanVersion(ctx, user.ID, target.PlanID); err == nil {
		cmp.Base, baseRaw = &base, base.Doc
		if baseDoc, err = Decode(base.Doc); err != nil {
			return Comparison{}, err
		}
	} else if !errors.Is(err, store.ErrNotFound) {
		return Comparison{}, err
	}
	targetDoc, err := Decode(target.Doc)
	if err != nil {
		return Comparison{}, err
	}
	cmp.JSON = DiffLines(lines(baseRaw), lines(target.Doc))

	c := s.catalogFor(ctx, user)
	for w := 1; w <= max(baseDoc.Weeks, targetDoc.Weeks); w++ {
		keys := dayKeys(baseDoc, w)
		for _, k := range dayKeys(targetDoc, w) {
			if !slices.Contains(keys, k) {
				keys = append(keys, k)
			}
		}
		for _, k := range keys {
			a := dayLinesByKey(c, baseDoc, w, k)
			b := dayLinesByKey(c, targetDoc, w, k)
			if d := DiffLines(a, b); Changed(d) {
				cmp.Days = append(cmp.Days, DayChange{Week: w, Name: k.label(), Lines: d})
			}
		}
	}

	if a, err := s.Store.ActivePlan(ctx, user.ID); err == nil && a.PlanID == target.PlanID {
		cmp.ResetsCursor = !keepsCursor(targetDoc, a.Week, a.Day)
	}
	return cmp, nil
}

func lines(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	return strings.Split(Pretty(raw), "\n")
}

// dayKey identifies a day within a week across versions: its name and which
// occurrence of that name it is (weeks often repeat names, like A/B/A).
type dayKey struct {
	name string
	nth  int // 1 for the first day with this name in the week
}

func (k dayKey) label() string {
	if k.nth == 1 {
		return k.name
	}
	suffix := "th"
	switch k.nth {
	case 2:
		suffix = "nd"
	case 3:
		suffix = "rd"
	}
	return fmt.Sprintf("%s (%d%s)", k.name, k.nth, suffix)
}

func dayKeys(doc Doc, week int) []dayKey {
	if week > doc.Weeks {
		return nil
	}
	var out []dayKey
	seen := map[string]int{}
	for _, i := range DaysForWeek(doc, week) {
		name := doc.Days[i].Name
		seen[name]++
		out = append(out, dayKey{name, seen[name]})
	}
	return out
}

func dayLinesByKey(c *catalog, doc Doc, week int, k dayKey) []string {
	for d, key := range dayKeys(doc, week) {
		if key == k {
			day, _ := c.day(doc, week, d)
			return DayLines(day, c.user.Unit)
		}
	}
	return nil
}

// AdvanceFrom moves the cursor past (week, day) of planID after that day was
// trained, if the user follows planID and the cursor is still on that day.
func (s *Service) AdvanceFrom(ctx context.Context, user store.User, planID string, week, day int) error {
	a, err := s.Store.ActivePlan(ctx, user.ID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && (a.PlanID != planID || a.Week != week || a.Day != day)) {
		return nil
	}
	if err != nil {
		return err
	}
	v, err := s.Store.ActivePlanVersion(ctx, user.ID, planID)
	if err != nil {
		return err
	}
	doc, err := Decode(v.Doc)
	if err != nil {
		return err
	}
	w, d := NextPosition(doc, week, day)
	return s.Store.SetActivePlan(ctx, user.ID, store.ActivePlan{PlanID: planID, Week: w, Day: d})
}
