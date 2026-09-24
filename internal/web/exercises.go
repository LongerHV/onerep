package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/LongerHV/onerep/internal/calc"
	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/store"
	"github.com/LongerHV/onerep/internal/web/views"
)

func (s *Server) exerciseRoutes(r chi.Router) {
	r.Get("/exercises", s.exerciseList)
	r.Get("/exercises/new", s.exerciseNew)
	r.Post("/exercises", s.exerciseCreate)
	r.Get("/exercises/{slug}", s.exerciseDetail)
	r.Get("/exercises/{slug}/edit", s.exerciseEdit)
	r.Post("/exercises/{slug}", s.exerciseUpdate)
	r.Post("/exercises/{slug}/delete", s.exerciseDelete)
	r.Post("/exercises/{slug}/settings", s.exerciseSettings)
	r.Get("/exercises/{slug}/calc", s.exerciseCalc)
	r.Post("/exercises/{slug}/alternatives", s.alternativeAdd)
	r.Post("/exercises/{slug}/alternatives/{alt}/delete", s.alternativeRemove)
}

func (s *Server) exerciseList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	list, err := s.Exercises.Catalog(r.Context(), user(r).ID, q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if r.Header.Get("HX-Target") == "exercise-results" {
		render(w, r, http.StatusOK, views.ExerciseResults(list))
		return
	}
	render(w, r, http.StatusOK, views.ExerciseList(page(r, "Exercises"), q, list))
}

// exerciseDetailData loads everything the exercise page shows.
func (s *Server) exerciseDetailData(r *http.Request, slug string) (views.ExerciseDetail, error) {
	ctx, u := r.Context(), user(r)
	ex, err := s.Exercises.Get(ctx, u.ID, slug)
	if err != nil {
		return views.ExerciseDetail{}, err
	}
	d := views.ExerciseDetail{Exercise: ex, Errors: map[string]string{}}
	if d.Settings, err = s.Exercises.Settings(ctx, u.ID, ex); err != nil {
		return d, err
	}
	if d.Equipment, err = s.Exercises.ListEquipment(ctx, u.ID); err != nil {
		return d, err
	}
	if d.Alternatives, err = s.Exercises.Alternatives(ctx, u.ID, slug); err != nil {
		return d, err
	}
	if d.History, err = s.Exercises.TrainingMaxHistory(ctx, u.ID, slug); err != nil {
		return d, err
	}
	catalog, err := s.Exercises.Catalog(ctx, u.ID, "")
	if err != nil {
		return d, err
	}
	taken := map[string]bool{slug: true}
	for _, a := range d.Alternatives {
		taken[a.Exercise.Slug] = true
	}
	for _, c := range catalog {
		if !taken[c.Slug] {
			d.Candidates = append(d.Candidates, c)
		}
	}
	if tm := d.Settings.TrainingMaxKg; tm != nil {
		d.TMInput = displayTM(*tm, u.Unit)
	}
	if d.Stats, err = s.Stats.ExerciseStats(ctx, u, slug); err != nil {
		return d, err
	}
	return d, nil
}

func (s *Server) exerciseDetail(w http.ResponseWriter, r *http.Request) {
	d, err := s.exerciseDetailData(r, chi.URLParam(r, "slug"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	render(w, r, http.StatusOK, views.ExerciseDetailPage(page(r, d.Exercise.Name), d))
}

func (s *Server) exerciseNew(w http.ResponseWriter, r *http.Request) {
	f := views.ExerciseForm{New: true, Input: exercise.Input{Measurement: "weight_reps", EquipmentKind: calc.KindBarbell}}
	render(w, r, http.StatusOK, views.ExerciseFormPage(page(r, "New exercise"), f))
}

func (s *Server) exerciseEdit(w http.ResponseWriter, r *http.Request) {
	ex, err := s.Exercises.Get(r.Context(), user(r).ID, chi.URLParam(r, "slug"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	f := views.ExerciseForm{
		Input: exercise.Input{Slug: ex.Slug, Name: ex.Name, Measurement: ex.Measurement, EquipmentKind: ex.EquipmentKind,
			PrimaryMuscles: ex.PrimaryMuscles, SecondaryMuscles: ex.SecondaryMuscles, Aliases: ex.Aliases},
		AliasesText: strings.Join(ex.Aliases, ", "),
	}
	render(w, r, http.StatusOK, views.ExerciseFormPage(page(r, "Edit "+ex.Name), f))
}

func exerciseInput(r *http.Request, slug string) (exercise.Input, string) {
	_ = r.ParseForm()
	aliases := r.PostForm.Get("aliases")
	return exercise.Input{
		Slug:             slug,
		Name:             r.PostForm.Get("name"),
		Measurement:      r.PostForm.Get("measurement"),
		EquipmentKind:    r.PostForm.Get("equipment_kind"),
		PrimaryMuscles:   r.PostForm["primary_muscles"],
		SecondaryMuscles: r.PostForm["secondary_muscles"],
		Aliases:          strings.Split(aliases, ","),
	}, aliases
}

func (s *Server) exerciseCreate(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	in, aliases := exerciseInput(r, strings.TrimSpace(r.PostForm.Get("slug")))
	ex, err := s.Exercises.Create(r.Context(), user(r).ID, in)
	s.afterExerciseSave(w, r, views.ExerciseForm{New: true, Input: in, AliasesText: aliases}, ex, err)
}

func (s *Server) exerciseUpdate(w http.ResponseWriter, r *http.Request) {
	in, aliases := exerciseInput(r, chi.URLParam(r, "slug"))
	ex, err := s.Exercises.Update(r.Context(), user(r).ID, in)
	s.afterExerciseSave(w, r, views.ExerciseForm{Input: in, AliasesText: aliases}, ex, err)
}

func (s *Server) afterExerciseSave(w http.ResponseWriter, r *http.Request, f views.ExerciseForm, ex store.Exercise, err error) {
	var fe exercise.FieldErrors
	switch {
	case errors.As(err, &fe):
		f.Errors = fe
		render(w, r, http.StatusUnprocessableEntity, views.ExerciseFormPage(page(r, "Exercise"), f))
	case err != nil:
		s.fail(w, r, err)
	default:
		http.Redirect(w, r, "/exercises/"+ex.Slug, http.StatusSeeOther)
	}
}

func (s *Server) exerciseDelete(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := s.Exercises.Delete(r.Context(), user(r).ID, slug); err != nil {
		s.fail(w, r, err)
		return
	}
	// A reset seeded exercise still exists; a deleted custom one does not.
	if _, err := s.Exercises.Get(r.Context(), user(r).ID, slug); err == nil {
		http.Redirect(w, r, "/exercises/"+slug, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/exercises", http.StatusSeeOther)
}

func (s *Server) exerciseSettings(w http.ResponseWriter, r *http.Request) {
	ctx, u, slug := r.Context(), user(r), chi.URLParam(r, "slug")
	_ = r.ParseForm()
	tmText := strings.TrimSpace(r.PostForm.Get("training_max"))
	var tm *float64
	errs := map[string]string{}
	if tmText != "" {
		v, err := strconv.ParseFloat(strings.ReplaceAll(tmText, ",", "."), 64)
		if err != nil {
			errs["training_max"] = "enter a number, or leave empty for none"
		} else {
			kg := calc.ToKg(v, u.Unit)
			tm = &kg
		}
	}
	if len(errs) == 0 {
		err := s.Exercises.LinkEquipment(ctx, u.ID, slug, r.PostForm.Get("equipment_id"))
		if err == nil && !s.showsCurrentTM(r, slug, tmText) {
			err = s.Exercises.SetTrainingMax(ctx, u.ID, slug, tm, "web")
		}
		var fe exercise.FieldErrors
		switch {
		case errors.As(err, &fe):
			errs = fe
		case err != nil:
			s.fail(w, r, err)
			return
		default:
			http.Redirect(w, r, "/exercises/"+slug, http.StatusSeeOther)
			return
		}
	}
	d, err := s.exerciseDetailData(r, slug)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	d.Errors, d.TMInput = errs, tmText
	render(w, r, http.StatusUnprocessableEntity, views.ExerciseDetailPage(page(r, d.Exercise.Name), d))
}

func (s *Server) exerciseCalc(w http.ResponseWriter, r *http.Request) {
	ctx, u := r.Context(), user(r)
	ex, err := s.Exercises.Get(ctx, u.ID, chi.URLParam(r, "slug"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	pctText := strings.TrimSpace(r.URL.Query().Get("pct"))
	pct, err := strconv.ParseFloat(pctText, 64)
	if err != nil || pct <= 0 || pct > 150 {
		render(w, r, http.StatusOK, views.CalcResultFragment(u.Unit, views.CalcResult{Error: "Enter a percentage between 0 and 150."}))
		return
	}
	res, err := s.Exercises.PercentOfTM(ctx, u, ex, pct/100)
	if errors.Is(err, exercise.ErrNoTrainingMax) {
		render(w, r, http.StatusOK, views.CalcResultFragment(u.Unit, views.CalcResult{Error: "Set a training max first."}))
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	st, err := s.Exercises.Settings(ctx, u.ID, ex)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := views.CalcResult{PctText: exercise.FormatNumber(pct), TMKg: *st.TrainingMaxKg, Kg: res.Kg, PerSide: res.PerSide}
	if st.Equipment != nil {
		out.Equipment = *st.Equipment
	}
	render(w, r, http.StatusOK, views.CalcResultFragment(u.Unit, out))
}

func (s *Server) alternativeAdd(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	err := s.Exercises.AddAlternative(r.Context(), user(r).ID, slug, r.PostFormValue("alternative"))
	var fe exercise.FieldErrors
	if errors.As(err, &fe) {
		d, derr := s.exerciseDetailData(r, slug)
		if derr != nil {
			s.fail(w, r, derr)
			return
		}
		d.Errors = fe
		render(w, r, http.StatusUnprocessableEntity, views.ExerciseDetailPage(page(r, d.Exercise.Name), d))
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/exercises/"+slug, http.StatusSeeOther)
}

func (s *Server) alternativeRemove(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := s.Exercises.RemoveAlternative(r.Context(), user(r).ID, slug, chi.URLParam(r, "alt")); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/exercises/"+slug, http.StatusSeeOther)
}

// displayTM is how a training max appears in the settings form.
func displayTM(kg float64, unit string) string {
	return exercise.FormatNumber(calc.FromKg(kg, unit))
}

// showsCurrentTM reports whether text is the current training max as the form
// displayed it. The display is rounded to 0.01 of the user's unit, so saving it
// back would change the stored kg value and log a change nobody made.
func (s *Server) showsCurrentTM(r *http.Request, slug, text string) bool {
	u := user(r)
	ex, err := s.Exercises.Get(r.Context(), u.ID, slug)
	if err != nil {
		return false
	}
	st, err := s.Exercises.Settings(r.Context(), u.ID, ex)
	if err != nil || st.TrainingMaxKg == nil {
		return false
	}
	return text == displayTM(*st.TrainingMaxKg, u.Unit)
}
