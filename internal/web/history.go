package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/LongerHV/onerep/internal/calc"
	"github.com/LongerHV/onerep/internal/store"
	"github.com/LongerHV/onerep/internal/training"
	"github.com/LongerHV/onerep/internal/web/views"
)

func (s *Server) historyRoutes(r chi.Router) {
	r.Get("/history", s.historyList)
	r.Get("/history/{id}", s.historyDetail)
	r.Post("/history/{id}/sets", s.historySaveSet)
	r.Post("/history/{id}/sets/{sid}/delete", s.historyDeleteSet)
	r.Post("/history/{id}/notes", s.historyNotes)
	r.Post("/history/{id}/finish", s.historyFinish)
	r.Post("/history/{id}/delete", s.historyDelete)
}

func (s *Server) historyList(w http.ResponseWriter, r *http.Request) {
	pageNo, _ := strconv.Atoi(r.URL.Query().Get("page"))
	sessions, more, err := s.Training.History(r.Context(), user(r), pageNo)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	render(w, r, http.StatusOK, views.HistoryList(page(r, "History"), sessions, max(pageNo, 0), more))
}

// historyData loads a session with the names and measurements of its exercises.
func (s *Server) historyData(r *http.Request, id string) (views.HistoryDetail, error) {
	ctx, u := r.Context(), user(r)
	sess, sets, err := s.Training.Session(ctx, u, id)
	if err != nil {
		return views.HistoryDetail{}, err
	}
	d := views.HistoryDetail{Session: sess, Exercises: map[string]store.Exercise{}}
	// New sets go after the planned groups, so the companion never takes
	// them for a planned set it has not reached yet.
	var snapshot struct {
		Groups []json.RawMessage `json:"groups"`
	}
	if err := json.Unmarshal(sess.Snapshot, &snapshot); err == nil {
		d.NextGroupPos = len(snapshot.Groups)
	}
	catalog, err := s.Exercises.Catalog(ctx, u.ID, "")
	if err != nil {
		return d, err
	}
	d.Catalog = catalog
	if d.PRs, err = s.Stats.SessionPRs(ctx, u, id); err != nil {
		return d, err
	}
	for _, set := range sets {
		if _, ok := d.Exercises[set.Slug]; !ok {
			ex, err := s.Exercises.Get(ctx, u.ID, set.Slug)
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				return d, err
			}
			if errors.Is(err, store.ErrNotFound) {
				ex = store.Exercise{Slug: set.Slug, Name: set.Slug, Measurement: "weight_reps"}
			}
			d.Exercises[set.Slug] = ex
		}
		if n := len(d.Groups); n == 0 || d.Groups[n-1].Slug != set.Slug || d.Groups[n-1].GroupPos != set.GroupPos {
			d.Groups = append(d.Groups, views.HistoryGroup{Slug: set.Slug, GroupPos: set.GroupPos})
		}
		d.Groups[len(d.Groups)-1].Sets = append(d.Groups[len(d.Groups)-1].Sets, set)
		d.NextGroupPos = max(d.NextGroupPos, set.GroupPos+1)
	}
	return d, nil
}

func (s *Server) historyDetail(w http.ResponseWriter, r *http.Request) {
	d, err := s.historyData(r, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	render(w, r, http.StatusOK, views.HistoryDetailPage(page(r, d.Session.Name), d))
}

// parseSetForm reads a set from the history editor. Weights are in the user's unit.
func parseSetForm(r *http.Request, unit string) (training.SetInput, error) {
	f := r.PostForm
	num := func(name string) (*float64, error) {
		v := strings.TrimSpace(strings.ReplaceAll(f.Get(name), ",", "."))
		if v == "" {
			return nil, nil
		}
		n, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, training.InvalidError{Reason: name + " must be a number"}
		}
		return &n, nil
	}
	integer := func(name string) (*int, error) {
		n, err := num(name)
		if err != nil || n == nil {
			return nil, err
		}
		i := int(*n)
		return &i, nil
	}
	in := training.SetInput{ID: f.Get("set_id"), Slug: f.Get("slug"), Kind: f.Get("kind")}
	var err error
	if in.WeightKg, err = num("weight"); err != nil {
		return in, err
	}
	if in.WeightKg != nil {
		kg := calc.ToKg(*in.WeightKg, unit)
		in.WeightKg = &kg
	}
	if in.Reps, err = integer("reps"); err != nil {
		return in, err
	}
	if in.RPE, err = num("rpe"); err != nil {
		return in, err
	}
	if in.DurationS, err = integer("duration"); err != nil {
		return in, err
	}
	if in.DistanceM, err = num("distance"); err != nil {
		return in, err
	}
	for name, dst := range map[string]*int{"group_pos": &in.GroupPos, "exercise_pos": &in.ExercisePos, "set_pos": &in.SetPos} {
		if v, err := strconv.Atoi(f.Get(name)); err == nil {
			*dst = v
		}
	}
	if in.Kind == "" {
		in.Kind = "working"
	}
	return in, nil
}

func (s *Server) historySaveSet(w http.ResponseWriter, r *http.Request) {
	u, id := user(r), chi.URLParam(r, "id")
	in, err := parseSetForm(r, u.Unit)
	if err == nil {
		in.SessionID = id
		if in.ID == "" { // a forgotten set: it belongs to when the workout was
			in.ID = newSetID()
			d, err := s.historyData(r, id)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			in.DoneAt = &d.Session.StartedAt
			in.GroupPos, in.ExercisePos, in.SetPos = d.NextGroupPos, 0, 0
		} else if sets := s.existingSet(r, id, in.ID); sets != nil {
			in.DoneAt, in.Prescribed = sets.DoneAt, sets.Prescribed
		}
		err = s.Training.SaveSet(r.Context(), u, in)
	}
	s.afterHistoryEdit(w, r, id, err)
}

// existingSet returns the stored set being edited, so edits keep when it was
// done and what was prescribed.
func (s *Server) existingSet(r *http.Request, sessionID, setID string) *store.Set {
	_, sets, err := s.Training.Session(r.Context(), user(r), sessionID)
	if err != nil {
		return nil
	}
	for i := range sets {
		if sets[i].ID == setID {
			return &sets[i]
		}
	}
	return nil
}

func (s *Server) historyDeleteSet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.afterHistoryEdit(w, r, id, s.Training.DeleteSet(r.Context(), user(r), id, chi.URLParam(r, "sid")))
}

func (s *Server) historyNotes(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.afterHistoryEdit(w, r, id, s.Training.SetNotes(r.Context(), user(r), id, r.PostFormValue("notes")))
}

func (s *Server) historyFinish(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.afterHistoryEdit(w, r, id, s.Training.Finish(r.Context(), user(r), id))
}

func (s *Server) historyDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.Training.Delete(r.Context(), user(r), chi.URLParam(r, "id")); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/history", http.StatusSeeOther)
}

func (s *Server) afterHistoryEdit(w http.ResponseWriter, r *http.Request, id string, err error) {
	var invalid training.InvalidError
	switch {
	case errors.As(err, &invalid):
		d, derr := s.historyData(r, id)
		if derr != nil {
			s.fail(w, r, derr)
			return
		}
		d.Error = invalid.Reason
		render(w, r, http.StatusUnprocessableEntity, views.HistoryDetailPage(page(r, d.Session.Name), d))
	case err != nil:
		s.fail(w, r, err)
	default:
		http.Redirect(w, r, "/history/"+id, http.StatusSeeOther)
	}
}
