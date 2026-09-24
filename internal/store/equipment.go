package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/LongerHV/onerep/internal/calc"
)

// Equipment is a user's equipment profile.
type Equipment struct {
	ID        string
	UserID    string
	Name      string
	IsDefault bool // the default profile for Spec.Kind
	Spec      calc.Equipment
	CreatedAt time.Time
}

const equipmentColumns = `id, user_id, name, kind, unit, config, is_default, created_at`

func scanEquipment(row interface{ Scan(...any) error }) (Equipment, error) {
	var e Equipment
	var config, created string
	err := row.Scan(&e.ID, &e.UserID, &e.Name, &e.Spec.Kind, &e.Spec.Unit, &config, &e.IsDefault, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return Equipment{}, ErrNotFound
	}
	if err != nil {
		return Equipment{}, err
	}
	if err := json.Unmarshal([]byte(config), &e.Spec.Config); err != nil {
		return Equipment{}, err
	}
	e.CreatedAt, err = parseTime(created)
	return e, err
}

func (db *DB) ListEquipment(ctx context.Context, userID string) ([]Equipment, error) {
	rows, err := db.read.QueryContext(ctx, `SELECT `+equipmentColumns+` FROM equipment
		WHERE user_id = ? ORDER BY kind, name COLLATE NOCASE`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Equipment
	for rows.Next() {
		e, err := scanEquipment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (db *DB) EquipmentByID(ctx context.Context, userID, id string) (Equipment, error) {
	return scanEquipment(db.read.QueryRowContext(ctx, `SELECT `+equipmentColumns+` FROM equipment
		WHERE user_id = ? AND id = ?`, userID, id))
}

// DefaultEquipment returns the user's default profile for kind, or ErrNotFound.
func (db *DB) DefaultEquipment(ctx context.Context, userID, kind string) (Equipment, error) {
	return scanEquipment(db.read.QueryRowContext(ctx, `SELECT `+equipmentColumns+` FROM equipment
		WHERE user_id = ? AND kind = ? AND is_default = 1`, userID, kind))
}

// SaveEquipment inserts e (empty ID) or updates it. Marking a profile as
// default clears the flag on the user's other profiles of the same kind.
func (db *DB) SaveEquipment(ctx context.Context, e Equipment) (Equipment, error) {
	err := db.tx(ctx, func(tx *sql.Tx) error { return saveEquipment(ctx, tx, &e) })
	return e, err
}

func saveEquipment(ctx context.Context, tx *sql.Tx, e *Equipment) error {
	config, err := json.Marshal(e.Spec.Config)
	if err != nil {
		return err
	}
	if e.IsDefault {
		if _, err := tx.ExecContext(ctx, `UPDATE equipment SET is_default = 0
			WHERE user_id = ? AND kind = ? AND id != ?`, e.UserID, e.Spec.Kind, e.ID); err != nil {
			return err
		}
	}
	if e.ID != "" {
		res, err := tx.ExecContext(ctx, `UPDATE equipment SET name = ?, kind = ?, unit = ?, config = ?, is_default = ?
			WHERE id = ? AND user_id = ?`,
			e.Name, e.Spec.Kind, e.Spec.Unit, string(config), e.IsDefault, e.ID, e.UserID)
		if err != nil {
			return err
		}
		return mustAffect(res)
	}
	if e.ID, err = newID(); err != nil {
		return err
	}
	e.CreatedAt = time.Now().UTC()
	_, err = tx.ExecContext(ctx, `INSERT INTO equipment (`+equipmentColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.UserID, e.Name, e.Spec.Kind, e.Spec.Unit, string(config), e.IsDefault, formatTime(e.CreatedAt))
	return err
}

func (db *DB) DeleteEquipment(ctx context.Context, userID, id string) error {
	res, err := db.write.ExecContext(ctx, `DELETE FROM equipment WHERE user_id = ? AND id = ?`, userID, id)
	if err != nil {
		return err
	}
	return mustAffect(res)
}

// InitStarterEquipment creates the given profiles unless the user's starter
// profiles were already created. It reports whether it created them; exactly
// one of several concurrent callers does.
func (db *DB) InitStarterEquipment(ctx context.Context, userID string, items []Equipment) (bool, error) {
	created := false
	err := db.tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE users SET equipment_initialized = 1
			WHERE id = ? AND equipment_initialized = 0`, userID)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil || n == 0 {
			return err
		}
		for _, e := range items {
			e.ID, e.UserID = "", userID
			if err := saveEquipment(ctx, tx, &e); err != nil {
				return err
			}
		}
		created = true
		return nil
	})
	return created, err
}
