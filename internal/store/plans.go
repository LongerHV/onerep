package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Plan statuses of a version.
const (
	PlanDraft      = "draft"
	PlanActive     = "active"
	PlanSuperseded = "superseded"
)

type Plan struct {
	ID        string
	UserID    string
	Name      string
	Archived  bool
	CreatedAt time.Time
}

// PlanVersion is one saved state of a plan document.
type PlanVersion struct {
	ID        string
	PlanID    string
	Version   int
	Doc       []byte
	Status    string // PlanDraft, PlanActive or PlanSuperseded
	Source    string // "web" or "mcp"
	Note      string
	CreatedAt time.Time
}

// ActivePlan is the plan a user follows and the next day to train.
type ActivePlan struct {
	PlanID string
	Week   int // 1-based; past the plan's last week = complete
	Day    int // index among the days of Week
}

const planColumns = `id, user_id, name, archived, created_at`

func scanPlan(row interface{ Scan(...any) error }) (Plan, error) {
	var p Plan
	var created string
	err := row.Scan(&p.ID, &p.UserID, &p.Name, &p.Archived, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return Plan{}, ErrNotFound
	}
	if err != nil {
		return Plan{}, err
	}
	p.CreatedAt, err = parseTime(created)
	return p, err
}

const versionColumns = `v.id, v.plan_id, v.version, v.doc, v.status, v.source, v.note, v.created_at`

func scanVersion(row interface{ Scan(...any) error }) (PlanVersion, error) {
	var v PlanVersion
	var doc, created string
	err := row.Scan(&v.ID, &v.PlanID, &v.Version, &doc, &v.Status, &v.Source, &v.Note, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return PlanVersion{}, ErrNotFound
	}
	if err != nil {
		return PlanVersion{}, err
	}
	v.Doc = []byte(doc)
	v.CreatedAt, err = parseTime(created)
	return v, err
}

// CreatePlan creates a plan with doc as version 1.
func (db *DB) CreatePlan(ctx context.Context, userID, name string, doc []byte, status, source, note string) (Plan, PlanVersion, error) {
	var p Plan
	var v PlanVersion
	err := db.tx(ctx, func(tx *sql.Tx) error {
		id, err := newID()
		if err != nil {
			return err
		}
		p = Plan{ID: id, UserID: userID, Name: name, CreatedAt: time.Now().UTC()}
		if _, err := tx.ExecContext(ctx, `INSERT INTO plans (`+planColumns+`) VALUES (?, ?, ?, 0, ?)`,
			p.ID, userID, name, formatTime(p.CreatedAt)); err != nil {
			return err
		}
		v, err = insertVersion(ctx, tx, p.ID, doc, status, source, note)
		return err
	})
	return p, v, err
}

// SavePlanVersion adds the next version of the user's plan. Saving as active
// supersedes the current active version and renames the plan to name; a
// draft leaves the name alone (the plan is named after its active version).
func (db *DB) SavePlanVersion(ctx context.Context, userID, planID, name string, doc []byte, status, source, note string) (PlanVersion, error) {
	var v PlanVersion
	err := db.tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE plans SET name = CASE WHEN ? THEN ? ELSE name END WHERE id = ? AND user_id = ?`,
			status == PlanActive, name, planID, userID)
		if err != nil {
			return err
		}
		if err := mustAffect(res); err != nil {
			return err
		}
		v, err = insertVersion(ctx, tx, planID, doc, status, source, note)
		return err
	})
	return v, err
}

func insertVersion(ctx context.Context, tx *sql.Tx, planID string, doc []byte, status, source, note string) (PlanVersion, error) {
	if status == PlanActive {
		if _, err := tx.ExecContext(ctx, `UPDATE plan_versions SET status = 'superseded'
			WHERE plan_id = ? AND status = 'active'`, planID); err != nil {
			return PlanVersion{}, err
		}
	}
	id, err := newID()
	if err != nil {
		return PlanVersion{}, err
	}
	v := PlanVersion{ID: id, PlanID: planID, Doc: doc, Status: status, Source: source, Note: note, CreatedAt: time.Now().UTC()}
	err = tx.QueryRowContext(ctx, `INSERT INTO plan_versions (id, plan_id, version, doc, status, source, note, created_at)
		VALUES (?, ?, (SELECT coalesce(max(version), 0) + 1 FROM plan_versions WHERE plan_id = ?), ?, ?, ?, ?, ?)
		RETURNING version`,
		v.ID, planID, planID, string(doc), status, source, note, formatTime(v.CreatedAt)).Scan(&v.Version)
	return v, err
}

// ListPlans returns the user's plans, unarchived first, by name.
func (db *DB) ListPlans(ctx context.Context, userID string) ([]Plan, error) {
	rows, err := db.read.QueryContext(ctx, `SELECT `+planColumns+` FROM plans WHERE user_id = ?
		ORDER BY archived, name COLLATE NOCASE`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Plan
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (db *DB) PlanByID(ctx context.Context, userID, id string) (Plan, error) {
	return scanPlan(db.read.QueryRowContext(ctx, `SELECT `+planColumns+` FROM plans WHERE user_id = ? AND id = ?`, userID, id))
}

func (db *DB) SetPlanArchived(ctx context.Context, userID, id string, archived bool) error {
	res, err := db.write.ExecContext(ctx, `UPDATE plans SET archived = ? WHERE user_id = ? AND id = ?`, archived, userID, id)
	if err != nil {
		return err
	}
	return mustAffect(res)
}

// PlanVersions lists a plan's versions, newest first.
func (db *DB) PlanVersions(ctx context.Context, userID, planID string) ([]PlanVersion, error) {
	rows, err := db.read.QueryContext(ctx, `SELECT `+versionColumns+` FROM plan_versions v
		JOIN plans p ON p.id = v.plan_id WHERE p.user_id = ? AND v.plan_id = ? ORDER BY v.version DESC`, userID, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlanVersion
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (db *DB) PlanVersionByID(ctx context.Context, userID, versionID string) (PlanVersion, error) {
	return scanVersion(db.read.QueryRowContext(ctx, `SELECT `+versionColumns+` FROM plan_versions v
		JOIN plans p ON p.id = v.plan_id WHERE p.user_id = ? AND v.id = ?`, userID, versionID))
}

// ActivePlanVersion returns the plan's active version, or ErrNotFound.
func (db *DB) ActivePlanVersion(ctx context.Context, userID, planID string) (PlanVersion, error) {
	return scanVersion(db.read.QueryRowContext(ctx, `SELECT `+versionColumns+` FROM plan_versions v
		JOIN plans p ON p.id = v.plan_id WHERE p.user_id = ? AND v.plan_id = ? AND v.status = 'active'`, userID, planID))
}

// ActivateVersion makes a draft (or an older superseded version) the plan's
// active version and renames the plan to name, the version's document name.
func (db *DB) ActivateVersion(ctx context.Context, userID, versionID, name string) error {
	return db.tx(ctx, func(tx *sql.Tx) error {
		var planID string
		err := tx.QueryRowContext(ctx, `SELECT v.plan_id FROM plan_versions v JOIN plans p ON p.id = v.plan_id
			WHERE p.user_id = ? AND v.id = ?`, userID, versionID).Scan(&planID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE plan_versions SET status = 'superseded'
			WHERE plan_id = ? AND status = 'active' AND id != ?`, planID, versionID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE plan_versions SET status = 'active' WHERE id = ?`, versionID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE plans SET name = ? WHERE id = ?`, name, planID)
		return err
	})
}

// DeleteDraft deletes a draft version. Other versions are ErrNotFound.
func (db *DB) DeleteDraft(ctx context.Context, userID, versionID string) error {
	res, err := db.write.ExecContext(ctx, `DELETE FROM plan_versions WHERE id = ? AND status = 'draft'
		AND plan_id IN (SELECT id FROM plans WHERE user_id = ?)`, versionID, userID)
	if err != nil {
		return err
	}
	return mustAffect(res)
}

// ActivePlan returns the plan the user follows, or ErrNotFound.
func (db *DB) ActivePlan(ctx context.Context, userID string) (ActivePlan, error) {
	var a ActivePlan
	err := db.read.QueryRowContext(ctx, `SELECT plan_id, cursor_week, cursor_day FROM active_plan WHERE user_id = ?`,
		userID).Scan(&a.PlanID, &a.Week, &a.Day)
	if errors.Is(err, sql.ErrNoRows) {
		return ActivePlan{}, ErrNotFound
	}
	return a, err
}

// SetActivePlan makes the user follow planID at (week, day).
func (db *DB) SetActivePlan(ctx context.Context, userID string, a ActivePlan) error {
	res, err := db.write.ExecContext(ctx, `INSERT INTO active_plan (user_id, plan_id, cursor_week, cursor_day, updated_at)
		SELECT ?, id, ?, ?, ? FROM plans WHERE id = ? AND user_id = ?
		ON CONFLICT (user_id) DO UPDATE SET plan_id = excluded.plan_id, cursor_week = excluded.cursor_week,
			cursor_day = excluded.cursor_day, updated_at = excluded.updated_at`,
		userID, a.Week, a.Day, formatTime(time.Now()), a.PlanID, userID)
	if err != nil {
		return err
	}
	return mustAffect(res)
}

func (db *DB) ClearActivePlan(ctx context.Context, userID string) error {
	_, err := db.write.ExecContext(ctx, `DELETE FROM active_plan WHERE user_id = ?`, userID)
	return err
}
