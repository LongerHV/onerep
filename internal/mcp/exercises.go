package mcp

import (
	"context"
	"slices"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/store"
)

type exerciseOut struct {
	Slug             string   `json:"slug"`
	Name             string   `json:"name"`
	Measurement      string   `json:"measurement"`
	EquipmentKind    string   `json:"equipment_kind"`
	PrimaryMuscles   []string `json:"primary_muscles"`
	SecondaryMuscles []string `json:"secondary_muscles"`
	Aliases          []string `json:"aliases"`
	Custom           bool     `json:"custom"`
}

func exerciseOf(e store.Exercise) exerciseOut {
	return exerciseOut{Slug: e.Slug, Name: e.Name, Measurement: e.Measurement, EquipmentKind: e.EquipmentKind,
		PrimaryMuscles: e.PrimaryMuscles, SecondaryMuscles: e.SecondaryMuscles, Aliases: e.Aliases, Custom: e.Custom()}
}

type listExercisesIn struct {
	Query  string `json:"query,omitempty" jsonschema:"words that must all appear in the slug, name, aliases, equipment or muscles"`
	Muscle string `json:"muscle,omitempty" jsonschema:"only exercises that work this muscle (primary or secondary), from the muscles list"`
}

type listExercisesOut struct {
	Exercises []exerciseOut `json:"exercises"`
	Muscles   []string      `json:"muscles" jsonschema:"the muscle vocabulary"`
}

type createExerciseIn struct {
	Slug             string   `json:"slug" jsonschema:"permanent id: lowercase letters, digits and dashes"`
	Name             string   `json:"name"`
	Measurement      string   `json:"measurement" jsonschema:"weight_reps, bw_reps, reps, time or distance_time"`
	EquipmentKind    string   `json:"equipment_kind" jsonschema:"barbell, dumbbell, machine, cable or bodyweight"`
	PrimaryMuscles   []string `json:"primary_muscles"`
	SecondaryMuscles []string `json:"secondary_muscles,omitempty"`
	Aliases          []string `json:"aliases,omitempty"`
}

type setTrainingMaxIn struct {
	Slug          string   `json:"slug"`
	TrainingMaxKg *float64 `json:"training_max_kg" jsonschema:"new training max in kg; null clears it"`
}

type setTrainingMaxOut struct {
	Slug          string   `json:"slug"`
	TrainingMaxKg *float64 `json:"training_max_kg"`
	PreviousKg    *float64 `json:"previous_kg"`
}

func (s *Server) addExerciseTools(srv *sdk.Server) {
	tool(s, srv, &sdk.Tool{Name: "list_exercises", Description: "Search the user's exercise catalog (built-in and custom)."},
		func(ctx context.Context, u store.User, in listExercisesIn) (listExercisesOut, error) {
			all, err := s.Exercises.Catalog(ctx, u.ID, in.Query)
			if err != nil {
				return listExercisesOut{}, err
			}
			out := listExercisesOut{Exercises: []exerciseOut{}, Muscles: exercise.Muscles}
			for _, e := range all {
				if in.Muscle == "" || slices.Contains(e.PrimaryMuscles, in.Muscle) || slices.Contains(e.SecondaryMuscles, in.Muscle) {
					out.Exercises = append(out.Exercises, exerciseOf(e))
				}
			}
			return out, nil
		})
	tool(s, srv, &sdk.Tool{Name: "create_exercise", Description: "Create a custom exercise for the user. Check list_exercises first: the catalog is large."},
		func(ctx context.Context, u store.User, in createExerciseIn) (exerciseOut, error) {
			e, err := s.Exercises.Create(ctx, u.ID, exercise.Input{Slug: in.Slug, Name: in.Name, Measurement: in.Measurement,
				EquipmentKind: in.EquipmentKind, PrimaryMuscles: in.PrimaryMuscles, SecondaryMuscles: in.SecondaryMuscles, Aliases: in.Aliases})
			return exerciseOf(e), err
		})
	tool(s, srv, &sdk.Tool{Name: "set_training_max", Description: "Set (or clear) the training max of an exercise, in kg. Plans use it for pct_tm loads; the change is logged as made by the AI."},
		func(ctx context.Context, u store.User, in setTrainingMaxIn) (setTrainingMaxOut, error) {
			ex, err := s.Exercises.Get(ctx, u.ID, in.Slug)
			if err != nil {
				return setTrainingMaxOut{}, err
			}
			before, err := s.Exercises.Settings(ctx, u.ID, ex)
			if err != nil {
				return setTrainingMaxOut{}, err
			}
			if err := s.Exercises.SetTrainingMax(ctx, u.ID, in.Slug, in.TrainingMaxKg, "mcp"); err != nil {
				return setTrainingMaxOut{}, err
			}
			return setTrainingMaxOut{Slug: in.Slug, TrainingMaxKg: in.TrainingMaxKg, PreviousKg: before.TrainingMaxKg}, nil
		})
}
