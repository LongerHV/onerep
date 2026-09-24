package web

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/LongerHV/onerep/internal/calc"
	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/store"
	"github.com/LongerHV/onerep/internal/web/views"
)

func (s *Server) equipmentRoutes(r chi.Router) {
	r.Get("/equipment", s.equipmentList)
	r.Get("/equipment/new", s.equipmentNew)
	r.Post("/equipment", s.equipmentSave)
	r.Get("/equipment/{id}/edit", s.equipmentEdit)
	r.Post("/equipment/{id}", s.equipmentSave)
	r.Post("/equipment/{id}/delete", s.equipmentDelete)
}

func (s *Server) equipmentList(w http.ResponseWriter, r *http.Request) {
	items, err := s.Exercises.ListEquipment(r.Context(), user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	render(w, r, http.StatusOK, views.EquipmentList(page(r, "Equipment"), items))
}

func (s *Server) equipmentNew(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	if !slices.Contains(calc.Kinds, kind) {
		s.renderError(w, r, http.StatusNotFound, "Unknown equipment kind.")
		return
	}
	f := views.EquipmentForm{Kind: kind, Unit: user(r).Unit, Name: views.Label(kind)}
	render(w, r, http.StatusOK, views.EquipmentFormPage(page(r, "New equipment"), f))
}

func (s *Server) equipmentEdit(w http.ResponseWriter, r *http.Request) {
	e, err := s.Exercises.GetEquipment(r.Context(), user(r).ID, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	c := e.Spec.Config
	f := views.EquipmentForm{
		ID: e.ID, Name: e.Name, Kind: e.Spec.Kind, Unit: e.Spec.Unit, IsDefault: e.IsDefault,
		Plates: exercise.FormatWeights(c.Plates), PlatePairs: exercise.FormatPlatePairs(c.PlatePairs),
		Weights: exercise.FormatWeights(c.Weights), Stack: exercise.FormatWeights(c.Stack),
	}
	if e.Spec.Kind == calc.KindBarbell {
		f.Bar = exercise.FormatNumber(c.Bar)
	}
	if len(c.Stack) == 0 && c.Step > 0 {
		// Profiles created with min/step/max are edited as a stack list.
		f.Stack = exercise.FormatNumber(c.Min) + "-" + exercise.FormatNumber(c.Max) + "/" + exercise.FormatNumber(c.Step)
	}
	render(w, r, http.StatusOK, views.EquipmentFormPage(page(r, "Edit "+e.Name), f))
}

// equipmentFromForm parses the form into a profile, collecting parse errors.
func equipmentFromForm(r *http.Request) (views.EquipmentForm, store.Equipment, exercise.FieldErrors) {
	_ = r.ParseForm()
	f := views.EquipmentForm{
		ID:         chi.URLParam(r, "id"),
		Name:       r.PostForm.Get("name"),
		Kind:       r.PostForm.Get("kind"),
		Unit:       r.PostForm.Get("unit"),
		IsDefault:  r.PostForm.Get("is_default") != "",
		Bar:        r.PostForm.Get("bar"),
		Plates:     r.PostForm.Get("plates"),
		PlatePairs: r.PostForm.Get("plate_pairs"),
		Weights:    r.PostForm.Get("weights"),
		Stack:      r.PostForm.Get("stack"),
	}
	e := store.Equipment{ID: f.ID, UserID: user(r).ID, Name: f.Name, IsDefault: f.IsDefault,
		Spec: calc.Equipment{Kind: f.Kind, Unit: f.Unit}}
	errs := exercise.FieldErrors{}
	list := func(field, text string) []float64 {
		vs, err := exercise.ParseWeights(text)
		if err != nil {
			errs[field] = err.Error()
		}
		return vs
	}
	c := &e.Spec.Config
	switch f.Kind {
	case calc.KindBarbell:
		bar, err := strconv.ParseFloat(strings.TrimSpace(f.Bar), 64)
		if err != nil || bar < 0 {
			errs["bar"] = "enter the bar weight"
		}
		c.Bar = bar
		c.Plates = list("plates", f.Plates)
		pairs, err := exercise.ParsePlatePairs(f.PlatePairs)
		if err != nil {
			errs["plate_pairs"] = err.Error()
		}
		c.PlatePairs = pairs
	case calc.KindDumbbell:
		c.Weights = list("weights", f.Weights)
	case calc.KindMachine, calc.KindCable:
		c.Stack = list("stack", f.Stack)
	}
	return f, e, errs
}

func (s *Server) equipmentSave(w http.ResponseWriter, r *http.Request) {
	f, e, errs := equipmentFromForm(r)
	if len(errs) == 0 {
		_, err := s.Exercises.SaveEquipment(r.Context(), e)
		var fe exercise.FieldErrors
		switch {
		case errors.As(err, &fe):
			errs = fe
		case err != nil:
			s.fail(w, r, err)
			return
		default:
			http.Redirect(w, r, "/equipment", http.StatusSeeOther)
			return
		}
	}
	f.Errors = errs
	render(w, r, http.StatusUnprocessableEntity, views.EquipmentFormPage(page(r, "Equipment"), f))
}

func (s *Server) equipmentDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.Exercises.DeleteEquipment(r.Context(), user(r).ID, chi.URLParam(r, "id")); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/equipment", http.StatusSeeOther)
}
