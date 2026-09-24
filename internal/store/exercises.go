package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Exercise is a catalog entry. Global (seeded) exercises have an empty UserID.
type Exercise struct {
	ID               string
	UserID           string
	Slug             string
	Name             string
	Measurement      string
	EquipmentKind    string
	PrimaryMuscles   []string
	SecondaryMuscles []string
	Aliases          []string
	Hidden           bool
	// Overrides is true for a user row that shadows a global exercise.
	Overrides bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Custom reports whether the exercise belongs to a user (created or customized).
func (e Exercise) Custom() bool { return e.UserID != "" }

const exerciseColumns = `e.id, coalesce(e.user_id, ''), e.slug, e.name, e.measurement, e.equipment_kind,
	e.primary_muscles, e.secondary_muscles, e.aliases, e.hidden,
	e.user_id IS NOT NULL AND EXISTS (SELECT 1 FROM exercises g WHERE g.user_id IS NULL AND g.slug = e.slug),
	e.created_at, e.updated_at`

func scanExercise(row interface{ Scan(...any) error }) (Exercise, error) {
	var e Exercise
	var primary, secondary, aliases, created, updated string
	err := row.Scan(&e.ID, &e.UserID, &e.Slug, &e.Name, &e.Measurement, &e.EquipmentKind,
		&primary, &secondary, &aliases, &e.Hidden, &e.Overrides, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Exercise{}, ErrNotFound
	}
	if err != nil {
		return Exercise{}, err
	}
	if e.PrimaryMuscles, err = parseList(primary); err != nil {
		return Exercise{}, err
	}
	if e.SecondaryMuscles, err = parseList(secondary); err != nil {
		return Exercise{}, err
	}
	if e.Aliases, err = parseList(aliases); err != nil {
		return Exercise{}, err
	}
	if e.CreatedAt, err = parseTime(created); err != nil {
		return Exercise{}, err
	}
	e.UpdatedAt, err = parseTime(updated)
	return e, err
}

func scanExercises(rows *sql.Rows, err error) ([]Exercise, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Exercise
	for rows.Next() {
		e, err := scanExercise(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// CatalogExercises returns the user's effective catalog: their own exercises
// plus visible global ones they have not customized, ordered by name.
func (db *DB) CatalogExercises(ctx context.Context, userID string) ([]Exercise, error) {
	return scanExercises(db.read.QueryContext(ctx, `SELECT `+exerciseColumns+` FROM exercises e
		WHERE e.user_id = ?
		   OR (e.user_id IS NULL AND e.hidden = 0 AND NOT EXISTS (
		       SELECT 1 FROM exercises u WHERE u.user_id = ? AND u.slug = e.slug))
		ORDER BY e.name COLLATE NOCASE`, userID, userID))
}

// ExerciseBySlug returns the user's own exercise with slug, else the global
// one (hidden global exercises included, since history may refer to them).
func (db *DB) ExerciseBySlug(ctx context.Context, userID, slug string) (Exercise, error) {
	return scanExercise(db.read.QueryRowContext(ctx, `SELECT `+exerciseColumns+` FROM exercises e
		WHERE e.slug = ? AND (e.user_id = ? OR e.user_id IS NULL)
		ORDER BY e.user_id IS NULL LIMIT 1`, slug, userID))
}

// SaveUserExercise creates or replaces the user's own exercise with e.Slug.
func (db *DB) SaveUserExercise(ctx context.Context, userID string, e Exercise) (Exercise, error) {
	id, err := newID()
	if err != nil {
		return Exercise{}, err
	}
	now := formatTime(time.Now())
	_, err = db.write.ExecContext(ctx, `
		INSERT INTO exercises (id, user_id, slug, name, measurement, equipment_kind,
			primary_muscles, secondary_muscles, aliases, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id, slug) WHERE user_id IS NOT NULL DO UPDATE SET
			name = excluded.name, measurement = excluded.measurement, equipment_kind = excluded.equipment_kind,
			primary_muscles = excluded.primary_muscles, secondary_muscles = excluded.secondary_muscles,
			aliases = excluded.aliases, updated_at = excluded.updated_at`,
		id, userID, e.Slug, e.Name, e.Measurement, e.EquipmentKind,
		jsonList(e.PrimaryMuscles), jsonList(e.SecondaryMuscles), jsonList(e.Aliases), now, now)
	if err != nil {
		return Exercise{}, err
	}
	return db.ExerciseBySlug(ctx, userID, e.Slug)
}

// DeleteUserExercise deletes the user's own exercise with slug. For a
// customized global exercise this restores the global version.
func (db *DB) DeleteUserExercise(ctx context.Context, userID, slug string) error {
	res, err := db.write.ExecContext(ctx, `DELETE FROM exercises WHERE user_id = ? AND slug = ?`, userID, slug)
	if err != nil {
		return err
	}
	return mustAffect(res)
}

// SyncSeed makes the global catalog match the seed: exercises are upserted by
// slug, global exercises missing from the seed are hidden (never deleted), and
// global alternatives are replaced.
func (db *DB) SyncSeed(ctx context.Context, exercises []Exercise, alternatives map[string][]string) error {
	return db.tx(ctx, func(tx *sql.Tx) error {
		now := formatTime(time.Now())
		if _, err := tx.ExecContext(ctx, `UPDATE exercises SET hidden = 1 WHERE user_id IS NULL`); err != nil {
			return err
		}
		for _, e := range exercises {
			id, err := newID()
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `
				INSERT INTO exercises (id, user_id, slug, name, measurement, equipment_kind,
					primary_muscles, secondary_muscles, aliases, hidden, created_at, updated_at)
				VALUES (?, NULL, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)
				ON CONFLICT (slug) WHERE user_id IS NULL DO UPDATE SET
					name = excluded.name, measurement = excluded.measurement, equipment_kind = excluded.equipment_kind,
					primary_muscles = excluded.primary_muscles, secondary_muscles = excluded.secondary_muscles,
					aliases = excluded.aliases, hidden = 0, updated_at = excluded.updated_at`,
				id, e.Slug, e.Name, e.Measurement, e.EquipmentKind,
				jsonList(e.PrimaryMuscles), jsonList(e.SecondaryMuscles), jsonList(e.Aliases), now, now)
			if err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM exercise_alternatives WHERE user_id IS NULL`); err != nil {
			return err
		}
		for slug, alts := range alternatives {
			for _, alt := range alts {
				if _, err := tx.ExecContext(ctx, `INSERT INTO exercise_alternatives (user_id, slug, alt_slug)
					VALUES (NULL, ?, ?) ON CONFLICT DO NOTHING`, slug, alt); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// Alternative is an exercise that can replace another one.
type Alternative struct {
	Slug      string
	UserAdded bool // false for alternatives from the seed
}

// Alternatives lists alternatives for slug: the seed's plus the user's own.
func (db *DB) Alternatives(ctx context.Context, userID, slug string) ([]Alternative, error) {
	rows, err := db.read.QueryContext(ctx, `SELECT alt_slug, max(user_id IS NOT NULL) FROM exercise_alternatives
		WHERE slug = ? AND (user_id IS NULL OR user_id = ?)
		GROUP BY alt_slug ORDER BY alt_slug`, slug, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Alternative
	for rows.Next() {
		var a Alternative
		if err := rows.Scan(&a.Slug, &a.UserAdded); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (db *DB) AddAlternative(ctx context.Context, userID, slug, altSlug string) error {
	_, err := db.write.ExecContext(ctx, `INSERT INTO exercise_alternatives (user_id, slug, alt_slug)
		VALUES (?, ?, ?) ON CONFLICT DO NOTHING`, userID, slug, altSlug)
	return err
}

// RemoveAlternative removes an alternative the user added (seeded ones stay).
func (db *DB) RemoveAlternative(ctx context.Context, userID, slug, altSlug string) error {
	res, err := db.write.ExecContext(ctx, `DELETE FROM exercise_alternatives
		WHERE user_id = ? AND slug = ? AND alt_slug = ?`, userID, slug, altSlug)
	if err != nil {
		return err
	}
	return mustAffect(res)
}
