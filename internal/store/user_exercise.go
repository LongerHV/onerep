package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// UserExercise is a user's settings for one exercise.
type UserExercise struct {
	Slug          string
	EquipmentID   string   // "" = use the default profile for the exercise's kind
	TrainingMaxKg *float64 // nil = not set
}

// TrainingMaxChange is one entry of an exercise's training max history.
type TrainingMaxChange struct {
	ID        string
	Slug      string
	OldKg     *float64
	NewKg     *float64
	Source    string // "web" or "mcp"
	Note      string
	CreatedAt time.Time
}

// UserExercise returns the user's settings for slug (zero settings if none).
func (db *DB) UserExercise(ctx context.Context, userID, slug string) (UserExercise, error) {
	ue := UserExercise{Slug: slug}
	var equipmentID sql.NullString
	var tm sql.NullFloat64
	err := db.read.QueryRowContext(ctx, `SELECT equipment_id, training_max_kg FROM user_exercise
		WHERE user_id = ? AND slug = ?`, userID, slug).Scan(&equipmentID, &tm)
	if errors.Is(err, sql.ErrNoRows) {
		return ue, nil
	}
	if err != nil {
		return UserExercise{}, err
	}
	ue.EquipmentID = equipmentID.String
	if tm.Valid {
		ue.TrainingMaxKg = &tm.Float64
	}
	return ue, nil
}

// SetExerciseEquipment links slug to one of the user's equipment profiles
// ("" removes the link). A profile of another user is ErrNotFound.
func (db *DB) SetExerciseEquipment(ctx context.Context, userID, slug, equipmentID string) error {
	return db.tx(ctx, func(tx *sql.Tx) error {
		if equipmentID != "" {
			var one int
			err := tx.QueryRowContext(ctx, `SELECT 1 FROM equipment WHERE id = ? AND user_id = ?`,
				equipmentID, userID).Scan(&one)
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			if err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO user_exercise (user_id, slug, equipment_id, updated_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT (user_id, slug) DO UPDATE SET equipment_id = excluded.equipment_id, updated_at = excluded.updated_at`,
			userID, slug, nullString(equipmentID), formatTime(time.Now()))
		return err
	})
}

// SetTrainingMax sets (or with nil clears) the training max for slug and
// records the change. Setting the current value again records nothing.
func (db *DB) SetTrainingMax(ctx context.Context, userID, slug string, newKg *float64, source, note string) error {
	return db.tx(ctx, func(tx *sql.Tx) error {
		var old sql.NullFloat64
		err := tx.QueryRowContext(ctx, `SELECT training_max_kg FROM user_exercise WHERE user_id = ? AND slug = ?`,
			userID, slug).Scan(&old)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var oldKg *float64
		if old.Valid {
			oldKg = &old.Float64
		}
		if (oldKg == nil && newKg == nil) || (oldKg != nil && newKg != nil && *oldKg == *newKg) {
			return nil
		}
		now := formatTime(time.Now())
		if _, err := tx.ExecContext(ctx, `INSERT INTO user_exercise (user_id, slug, training_max_kg, updated_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT (user_id, slug) DO UPDATE SET training_max_kg = excluded.training_max_kg, updated_at = excluded.updated_at`,
			userID, slug, newKg, now); err != nil {
			return err
		}
		id, err := newID()
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO training_max_log (id, user_id, slug, old_kg, new_kg, source, note, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, id, userID, slug, oldKg, newKg, source, note, now)
		return err
	})
}

// TrainingMaxHistory lists training max changes for slug, newest first.
func (db *DB) TrainingMaxHistory(ctx context.Context, userID, slug string) ([]TrainingMaxChange, error) {
	rows, err := db.read.QueryContext(ctx, `SELECT id, slug, old_kg, new_kg, source, note, created_at
		FROM training_max_log WHERE user_id = ? AND slug = ? ORDER BY created_at DESC, id DESC`, userID, slug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TrainingMaxChange
	for rows.Next() {
		var c TrainingMaxChange
		var oldKg, newKg sql.NullFloat64
		var created string
		if err := rows.Scan(&c.ID, &c.Slug, &oldKg, &newKg, &c.Source, &c.Note, &created); err != nil {
			return nil, err
		}
		if oldKg.Valid {
			c.OldKg = &oldKg.Float64
		}
		if newKg.Valid {
			c.NewKg = &newKg.Float64
		}
		if c.CreatedAt, err = parseTime(created); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
