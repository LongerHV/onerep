package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/LongerHV/onerep/internal/plan"
	"github.com/LongerHV/onerep/internal/store"
	"github.com/LongerHV/onerep/internal/web/views"
)

// cursorResetNotice is shown when a new version no longer has the user's day.
const cursorResetNotice = "Your current day does not exist in the new version, so the plan starts over at week 1, day 1."

func (s *Server) planRoutes(r chi.Router) {
	r.Get("/plans", s.planList)
	r.Get("/plans/new", s.planNew)
	r.Post("/plans", s.planCreate)
	r.Post("/plans/preview", s.planPreview)
	r.Get("/plans/{id}", s.planDetail)
	r.Get("/plans/{id}/edit", s.planEdit)
	r.Post("/plans/{id}", s.planSave)
	r.Post("/plans/{id}/follow", s.planFollow)
	r.Post("/plans/{id}/archive", s.planArchive)
	r.Get("/plans/{id}/versions/{vid}", s.planVersion)
	r.Get("/plans/{id}/versions/{vid}/compare", s.planCompare)
	r.Post("/plans/{id}/versions/{vid}/activate", s.planActivate)
	r.Post("/plans/{id}/versions/{vid}/discard", s.planDiscard)
	r.Post("/plan/skip", s.planSkip)
	r.Post("/plan/choose", s.planChoose)
	r.Post("/plan/restart", s.planRestart)
	r.Post("/plan/unfollow", s.planUnfollow)
}

// planSchema serves the plan JSON Schema; it is public so tools can fetch it.
func planSchema(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/schema+json")
	_, _ = w.Write(plan.Schema())
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	next, err := s.Plans.Next(r.Context(), user(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	home := views.HomePage{Next: next}
	if open, err := s.Training.Open(r.Context(), user(r)); err == nil {
		home.Open = &open
	} else if !errors.Is(err, store.ErrNotFound) {
		s.fail(w, r, err)
		return
	}
	render(w, r, http.StatusOK, views.Home(page(r, "Home"), home))
}

func (s *Server) planList(w http.ResponseWriter, r *http.Request) {
	plans, err := s.Plans.Plans(r.Context(), user(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	render(w, r, http.StatusOK, views.PlansPage(page(r, "Plans"), plans))
}

func (s *Server) editor(title, planID, doc string, problems plan.Problems) views.PlanEditor {
	return views.PlanEditor{PlanID: planID, Title: title, Doc: doc, Schema: string(plan.Schema()), Errors: problems}
}

func (s *Server) planNew(w http.ResponseWriter, r *http.Request) {
	e := s.editor("New plan", "", plan.Pretty(plan.StarterTemplate()), nil)
	render(w, r, http.StatusOK, views.PlanEditorPage(page(r, "New plan"), e))
}

func saveStatus(r *http.Request) string {
	if r.PostFormValue("action") == "activate" {
		return plan.SaveActivate
	}
	return plan.SaveDraft
}

func (s *Server) planCreate(w http.ResponseWriter, r *http.Request) {
	doc := r.PostFormValue("doc")
	p, _, err := s.Plans.Create(r.Context(), user(r), []byte(doc), saveStatus(r), "web", "")
	var ps plan.Problems
	switch {
	case errors.As(err, &ps):
		render(w, r, http.StatusUnprocessableEntity, views.PlanEditorPage(page(r, "New plan"), s.editor("New plan", "", doc, ps)))
	case err != nil:
		s.fail(w, r, err)
	default:
		http.Redirect(w, r, "/plans/"+p.ID, http.StatusSeeOther)
	}
}

func (s *Server) planPreview(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	doc, ps := s.Plans.Validate(r.Context(), u, []byte(r.PostFormValue("doc")))
	var weeks [][]plan.ExpandedDay
	if !ps.HasErrors() {
		weeks = s.Plans.Preview(r.Context(), u, doc)
	}
	render(w, r, http.StatusOK, views.PlanPreview(fragmentPage(r), ps, weeks))
}

func (s *Server) planDetail(w http.ResponseWriter, r *http.Request) {
	ctx, u := r.Context(), user(r)
	p, versions, err := s.Plans.Plan(ctx, u, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	next, err := s.Plans.Next(ctx, u)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	d := views.PlanPage{Plan: p, Versions: versions, Following: next != nil && next.Plan.ID == p.ID}
	if r.URL.Query().Has("reset") {
		d.Notice = cursorResetNotice
	}
	render(w, r, http.StatusOK, views.PlanDetailPage(page(r, p.Name), d))
}

// planEdit opens the editor on ?from=<version>, else the active version,
// else the newest version.
func (s *Server) planEdit(w http.ResponseWriter, r *http.Request) {
	p, versions, err := s.Plans.Plan(r.Context(), user(r), chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	from := r.URL.Query().Get("from")
	var src *store.PlanVersion
	for i, v := range versions {
		if (from != "" && v.ID == from) || (from == "" && v.Status == store.PlanActive) {
			src = &versions[i]
		}
	}
	if src == nil && from == "" && len(versions) > 0 {
		src = &versions[0]
	}
	if src == nil {
		s.renderError(w, r, http.StatusNotFound, "Version not found.")
		return
	}
	e := s.editor("Edit "+p.Name, p.ID, plan.Pretty(src.Doc), nil)
	render(w, r, http.StatusOK, views.PlanEditorPage(page(r, "Edit "+p.Name), e))
}

func (s *Server) planSave(w http.ResponseWriter, r *http.Request) {
	ctx, u, id := r.Context(), user(r), chi.URLParam(r, "id")
	doc := r.PostFormValue("doc")
	_, reset, err := s.Plans.Save(ctx, u, id, []byte(doc), saveStatus(r), "web", "")
	var ps plan.Problems
	switch {
	case errors.As(err, &ps):
		p, _, perr := s.Plans.Plan(ctx, u, id)
		if perr != nil {
			s.fail(w, r, perr)
			return
		}
		e := s.editor("Edit "+p.Name, id, doc, ps)
		render(w, r, http.StatusUnprocessableEntity, views.PlanEditorPage(page(r, "Edit "+p.Name), e))
	case err != nil:
		s.fail(w, r, err)
	default:
		s.redirectToPlan(w, r, id, reset)
	}
}

func (s *Server) redirectToPlan(w http.ResponseWriter, r *http.Request, id string, reset bool) {
	target := "/plans/" + id
	if reset {
		target += "?reset"
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// version loads {vid} and checks that it belongs to plan {id}.
func (s *Server) version(r *http.Request) (store.PlanVersion, error) {
	v, err := s.Plans.Version(r.Context(), user(r), chi.URLParam(r, "vid"))
	if err == nil && v.PlanID != chi.URLParam(r, "id") {
		return store.PlanVersion{}, store.ErrNotFound
	}
	return v, err
}

func (s *Server) planVersion(w http.ResponseWriter, r *http.Request) {
	v, err := s.version(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	p, _, err := s.Plans.Plan(r.Context(), user(r), v.PlanID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	doc, err := plan.Decode(v.Doc)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	d := views.PlanVersionPage{Plan: p, Version: v, Weeks: s.Plans.Preview(r.Context(), user(r), doc), JSON: plan.Pretty(v.Doc)}
	render(w, r, http.StatusOK, views.PlanVersionView(page(r, p.Name), d))
}

func (s *Server) planCompare(w http.ResponseWriter, r *http.Request) {
	v, err := s.version(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	p, _, err := s.Plans.Plan(r.Context(), user(r), v.PlanID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	cmp, err := s.Plans.Compare(r.Context(), user(r), v.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	render(w, r, http.StatusOK, views.PlanComparePage(page(r, p.Name), p, cmp))
}

func (s *Server) planActivate(w http.ResponseWriter, r *http.Request) {
	v, err := s.version(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	reset, err := s.Plans.Activate(r.Context(), user(r), v.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.redirectToPlan(w, r, v.PlanID, reset)
}

func (s *Server) planDiscard(w http.ResponseWriter, r *http.Request) {
	v, err := s.version(r)
	if err == nil {
		err = s.Plans.Discard(r.Context(), user(r), v.ID)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/plans/"+v.PlanID, http.StatusSeeOther)
}

func (s *Server) planFollow(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := s.Plans.Follow(r.Context(), user(r), id)
	if errors.Is(err, plan.ErrNoActiveVersion) {
		s.renderError(w, r, http.StatusConflict, "Activate a version of this plan before following it.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) planArchive(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.Plans.Archive(r.Context(), user(r), id, r.PostFormValue("archived") == "1"); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/plans/"+id, http.StatusSeeOther)
}

func (s *Server) planSkip(w http.ResponseWriter, r *http.Request) {
	if err := s.Plans.Skip(r.Context(), user(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) planChoose(w http.ResponseWriter, r *http.Request) {
	ws, ds, _ := strings.Cut(r.PostFormValue("position"), ":")
	week, err1 := strconv.Atoi(ws)
	day, err2 := strconv.Atoi(ds)
	err := errors.Join(err1, err2)
	if err == nil {
		err = s.Plans.Choose(r.Context(), user(r), week, day)
	}
	if err != nil {
		s.renderError(w, r, http.StatusUnprocessableEntity, "That day is not in your plan.")
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) planRestart(w http.ResponseWriter, r *http.Request) {
	if err := s.Plans.Restart(r.Context(), user(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) planUnfollow(w http.ResponseWriter, r *http.Request) {
	if err := s.Plans.Unfollow(r.Context(), user(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/plans", http.StatusSeeOther)
}
