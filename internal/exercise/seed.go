package exercise

import (
	"context"
	_ "embed"
	"encoding/json"

	"github.com/LongerHV/onerep/internal/store"
)

//go:embed seed/exercises.json
var seedJSON []byte

type seedEntry struct {
	Slug         string   `json:"slug"`
	Name         string   `json:"name"`
	Measurement  string   `json:"measurement"`
	Equipment    string   `json:"equipment"`
	Primary      []string `json:"primary"`
	Secondary    []string `json:"secondary"`
	Aliases      []string `json:"aliases"`
	Alternatives []string `json:"alternatives"`
}

func loadSeed() ([]store.Exercise, map[string][]string, error) {
	var entries []seedEntry
	if err := json.Unmarshal(seedJSON, &entries); err != nil {
		return nil, nil, err
	}
	exercises := make([]store.Exercise, 0, len(entries))
	alternatives := map[string][]string{}
	for _, e := range entries {
		exercises = append(exercises, store.Exercise{
			Slug: e.Slug, Name: e.Name, Measurement: e.Measurement, EquipmentKind: e.Equipment,
			PrimaryMuscles: e.Primary, SecondaryMuscles: e.Secondary, Aliases: e.Aliases,
		})
		if len(e.Alternatives) > 0 {
			alternatives[e.Slug] = e.Alternatives
		}
	}
	return exercises, alternatives, nil
}

// SeedStore is what Seed needs. *store.DB implements it.
type SeedStore interface {
	SyncSeed(ctx context.Context, exercises []store.Exercise, alternatives map[string][]string) error
}

// Seed syncs the embedded catalog into the database. Run it at startup.
func Seed(ctx context.Context, db SeedStore) error {
	exercises, alternatives, err := loadSeed()
	if err != nil {
		return err
	}
	return db.SyncSeed(ctx, exercises, alternatives)
}
