package mcp

import (
	"context"
	_ "embed"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/LongerHV/onerep/internal/plan"
	"github.com/LongerHV/onerep/internal/store"
)

//go:embed plan_format.md
var planFormat string

type schemaOut struct {
	Schema any    `json:"schema" jsonschema:"the plan document JSON Schema"`
	Guide  string `json:"guide" jsonschema:"how plans work, in prose"`
}

type versionOut struct {
	ID        string `json:"id"`
	Version   int    `json:"version"`
	Status    string `json:"status"`
	Source    string `json:"source"`
	Note      string `json:"note,omitempty"`
	CreatedAt string `json:"created_at"`
}

type planOut struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Archived  bool         `json:"archived"`
	Following bool         `json:"following" jsonschema:"the user follows this plan"`
	Versions  []versionOut `json:"versions" jsonschema:"newest first"`
}

type listPlansOut struct {
	Plans []planOut `json:"plans"`
}

type getPlanIn struct {
	PlanID    string `json:"plan_id,omitempty" jsonschema:"default: the plan the user follows"`
	VersionID string `json:"version_id,omitempty" jsonschema:"default: the plan's active version"`
}

type previewDay struct {
	Name  string   `json:"name"`
	Lines []string `json:"lines"`
}

type previewWeek struct {
	Week int          `json:"week"`
	Days []previewDay `json:"days"`
}

type getPlanOut struct {
	PlanID    string        `json:"plan_id"`
	Name      string        `json:"name"`
	VersionID string        `json:"version_id"`
	Version   int           `json:"version"`
	Status    string        `json:"status"`
	Unit      string        `json:"unit" jsonschema:"the user's preferred unit, used in the preview"`
	Doc       any           `json:"doc" jsonschema:"the authored plan document"`
	Preview   []previewWeek `json:"preview" jsonschema:"every week and day with loads resolved from the user's training maxes, e1RMs and equipment"`
}

type validateIn struct {
	Doc any `json:"doc" jsonschema:"the plan document (a JSON object, or a string containing one)"`
}

type validateOut struct {
	Valid    bool           `json:"valid"`
	Problems []plan.Problem `json:"problems" jsonschema:"errors and warnings with JSON Pointers into the document"`
}

type saveDraftIn struct {
	Doc       any    `json:"doc" jsonschema:"the plan document (a JSON object, or a string containing one)"`
	PlanID    string `json:"plan_id,omitempty" jsonschema:"add a new draft version to this plan"`
	VersionID string `json:"version_id,omitempty" jsonschema:"replace this draft version (only drafts can be replaced)"`
	Note      string `json:"note,omitempty" jsonschema:"what changed and why, shown to the user"`
}

type saveDraftOut struct {
	PlanID    string         `json:"plan_id"`
	VersionID string         `json:"version_id"`
	Version   int            `json:"version"`
	Status    string         `json:"status"`
	ReviewURL string         `json:"review_url" jsonschema:"where the user compares and activates the draft"`
	Warnings  []plan.Problem `json:"warnings"`
}

func versionOf(v store.PlanVersion) versionOut {
	return versionOut{ID: v.ID, Version: v.Version, Status: v.Status, Source: v.Source, Note: v.Note, CreatedAt: ts(v.CreatedAt)}
}

func (s *Server) addPlanTools(srv *sdk.Server) {
	tool(s, srv, &sdk.Tool{Name: "get_plan_schema", Description: "The plan document JSON Schema and a guide to the format. Read before writing a plan."},
		func(context.Context, store.User, struct{}) (schemaOut, error) {
			return schemaOut{Schema: jsonValue(plan.Schema()), Guide: planFormat}, nil
		})
	tool(s, srv, &sdk.Tool{Name: "list_plans", Description: "The user's plans with their versions and statuses."},
		func(ctx context.Context, u store.User, _ struct{}) (listPlansOut, error) {
			sums, err := s.Plans.Plans(ctx, u)
			if err != nil {
				return listPlansOut{}, err
			}
			out := listPlansOut{Plans: []planOut{}}
			for _, sum := range sums {
				_, versions, err := s.Plans.Plan(ctx, u, sum.ID)
				if err != nil {
					return listPlansOut{}, err
				}
				p := planOut{ID: sum.ID, Name: sum.Name, Archived: sum.Archived, Following: sum.Following, Versions: []versionOut{}}
				for _, v := range versions {
					p.Versions = append(p.Versions, versionOf(v))
				}
				out.Plans = append(out.Plans, p)
			}
			return out, nil
		})
	tool(s, srv, &sdk.Tool{Name: "get_plan", Description: "A plan version's document and a resolved preview of every week. Defaults to the active version of the plan the user follows."},
		func(ctx context.Context, u store.User, in getPlanIn) (getPlanOut, error) {
			v, err := s.pickVersion(ctx, u, in)
			if err != nil {
				return getPlanOut{}, err
			}
			p, _, err := s.Plans.Plan(ctx, u, v.PlanID)
			if err != nil {
				return getPlanOut{}, err
			}
			doc, err := plan.Decode(v.Doc)
			if err != nil {
				return getPlanOut{}, err
			}
			out := getPlanOut{PlanID: p.ID, Name: p.Name, VersionID: v.ID, Version: v.Version, Status: v.Status, Unit: u.Unit,
				Doc: jsonValue(v.Doc), Preview: []previewWeek{}}
			for w, days := range s.Plans.Preview(ctx, u, doc) {
				week := previewWeek{Week: w + 1, Days: []previewDay{}}
				for _, d := range days {
					week.Days = append(week.Days, previewDay{Name: d.Name, Lines: plan.DayLines(d, u.Unit)})
				}
				out.Preview = append(out.Preview, week)
			}
			return out, nil
		})
	tool(s, srv, &sdk.Tool{Name: "validate_plan", Description: "Check a plan document without saving it."},
		func(ctx context.Context, u store.User, in validateIn) (validateOut, error) {
			raw, err := docBytes(in.Doc)
			if err != nil {
				return validateOut{}, err
			}
			_, ps := s.Plans.Validate(ctx, u, raw)
			return validateOut{Valid: !ps.HasErrors(), Problems: append([]plan.Problem{}, ps...)}, nil
		})
	tool(s, srv, &sdk.Tool{Name: "save_plan_draft", Description: "Save a plan document as a draft: a new plan, a new version of plan_id, or a replacement for the draft version_id. Drafts never take effect until the user activates them at review_url."},
		func(ctx context.Context, u store.User, in saveDraftIn) (saveDraftOut, error) {
			raw, err := docBytes(in.Doc)
			if err != nil {
				return saveDraftOut{}, err
			}
			d, err := s.Plans.SaveDraft(ctx, u, plan.DraftInput{PlanID: in.PlanID, VersionID: in.VersionID, Doc: raw, Source: "mcp", Note: in.Note})
			if err != nil {
				return saveDraftOut{}, err
			}
			return saveDraftOut{PlanID: d.Plan.ID, VersionID: d.Version.ID, Version: d.Version.Version, Status: d.Version.Status,
				ReviewURL: strings.TrimSuffix(s.BaseURL, "/") + "/plans/" + d.Plan.ID + "/versions/" + d.Version.ID + "/compare",
				Warnings:  append([]plan.Problem{}, d.Warnings...)}, nil
		})
}

// pickVersion resolves get_plan's arguments to a version of the user's.
func (s *Server) pickVersion(ctx context.Context, u store.User, in getPlanIn) (store.PlanVersion, error) {
	if in.VersionID != "" {
		return s.Plans.Version(ctx, u, in.VersionID)
	}
	planID := in.PlanID
	if planID == "" {
		next, err := s.Plans.Next(ctx, u)
		if err != nil {
			return store.PlanVersion{}, err
		}
		if next == nil {
			return store.PlanVersion{}, inputError("the user doesn't follow a plan; pass plan_id (see list_plans)")
		}
		return next.Version, nil
	}
	_, versions, err := s.Plans.Plan(ctx, u, planID)
	if err != nil {
		return store.PlanVersion{}, err
	}
	for _, v := range versions {
		if v.Status == store.PlanActive {
			return v, nil
		}
	}
	return store.PlanVersion{}, inputError("the plan has no active version; pass version_id (see list_plans)")
}
