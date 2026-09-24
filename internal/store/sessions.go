package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Session is a workout (spec §9).
type Session struct {
	ID            string
	UserID        string
	PlanID        string // "" for ad-hoc sessions
	PlanVersionID string
	Week, Day     int // 0 for ad-hoc sessions
	Name          string
	Snapshot      []byte // plan.ExpandedDay as JSON
	StartedAt     time.Time
	FinishedAt    *time.Time
	Notes         string
	UpdatedAt     time.Time
}

// Set is a logged set. Pointer fields are nil when not recorded.
type Set struct {
	ID          string
	SessionID   string
	UserID      string
	Slug        string
	GroupPos    int
	ExercisePos int
	SetPos      int
	Kind        string
	Prescribed  []byte // plan.PrescribedSet as JSON, nil for unplanned sets
	WeightKg    *float64
	Reps        *int
	RPE         *float64
	DurationS   *int
	DistanceM   *float64
	E1RMKg      *float64
	DoneAt      *time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}

// Outcome of applying a sync operation.
type Outcome string

const (
	Applied   Outcome = "applied"
	Duplicate Outcome = "duplicate" // op_id seen before; nothing changed
	Ignored   Outcome = "ignored"   // older than what is stored, or the set was deleted
)

const sessionColumns = `id, user_id, coalesce(plan_id, ''), coalesce(plan_version_id, ''), week, day, name, snapshot,
	started_at, finished_at, notes, updated_at`

func scanSession(row interface{ Scan(...any) error }) (Session, error) {
	var s Session
	var snapshot, started, updated string
	var finished sql.NullString
	err := row.Scan(&s.ID, &s.UserID, &s.PlanID, &s.PlanVersionID, &s.Week, &s.Day, &s.Name, &snapshot,
		&started, &finished, &s.Notes, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, err
	}
	s.Snapshot = []byte(snapshot)
	if s.StartedAt, err = parseTime(started); err != nil {
		return Session{}, err
	}
	if s.FinishedAt, err = parseNullTime(finished); err != nil {
		return Session{}, err
	}
	s.UpdatedAt, err = parseTime(updated)
	return s, err
}

func parseNullTime(s sql.NullString) (*time.Time, error) {
	if !s.Valid {
		return nil, nil
	}
	t, err := parseTime(s.String)
	return &t, err
}

func nullTime(t *time.Time) sql.NullString {
	if t == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: formatTime(*t), Valid: true}
}

// CreateSession stores a new session; ID, StartedAt and UpdatedAt are set here.
func (db *DB) CreateSession(ctx context.Context, s Session) (Session, error) {
	var err error
	if s.ID, err = newID(); err != nil {
		return Session{}, err
	}
	s.StartedAt = time.Now().UTC()
	s.UpdatedAt = s.StartedAt
	_, err = db.write.ExecContext(ctx, `INSERT INTO sessions (id, user_id, plan_id, plan_version_id, week, day, name,
		snapshot, started_at, notes, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?)`,
		s.ID, s.UserID, nullString(s.PlanID), nullString(s.PlanVersionID), s.Week, s.Day, s.Name, string(s.Snapshot),
		formatTime(s.StartedAt), formatTime(s.UpdatedAt))
	return s, err
}

func (db *DB) SessionByID(ctx context.Context, userID, id string) (Session, error) {
	return scanSession(db.read.QueryRowContext(ctx, `SELECT `+sessionColumns+` FROM sessions
		WHERE user_id = ? AND id = ?`, userID, id))
}

// OpenSession returns the user's most recent unfinished session, or ErrNotFound.
func (db *DB) OpenSession(ctx context.Context, userID string) (Session, error) {
	return scanSession(db.read.QueryRowContext(ctx, `SELECT `+sessionColumns+` FROM sessions
		WHERE user_id = ? AND finished_at IS NULL ORDER BY started_at DESC, id DESC LIMIT 1`, userID))
}

// SessionSummary is a session as listed in the history.
type SessionSummary struct {
	Session
	Sets int
}

// ListSessions returns sessions newest first.
func (db *DB) ListSessions(ctx context.Context, userID string, limit, offset int) ([]SessionSummary, error) {
	rows, err := db.read.QueryContext(ctx, `SELECT `+sessionColumns+`,
		(SELECT count(*) FROM sets WHERE sets.session_id = sessions.id AND deleted_at IS NULL)
		FROM sessions WHERE user_id = ? ORDER BY started_at DESC, id DESC LIMIT ? OFFSET ?`, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SessionSummary
	for rows.Next() {
		var sum SessionSummary
		var snapshot, started, updated string
		var finished sql.NullString
		if err := rows.Scan(&sum.ID, &sum.UserID, &sum.PlanID, &sum.PlanVersionID, &sum.Week, &sum.Day, &sum.Name,
			&snapshot, &started, &finished, &sum.Notes, &updated, &sum.Sets); err != nil {
			return nil, err
		}
		sum.Snapshot = []byte(snapshot)
		if sum.StartedAt, err = parseTime(started); err != nil {
			return nil, err
		}
		if sum.FinishedAt, err = parseNullTime(finished); err != nil {
			return nil, err
		}
		if sum.UpdatedAt, err = parseTime(updated); err != nil {
			return nil, err
		}
		out = append(out, sum)
	}
	return out, rows.Err()
}

func (db *DB) DeleteSession(ctx context.Context, userID, id string) error {
	res, err := db.write.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND id = ?`, userID, id)
	if err != nil {
		return err
	}
	return mustAffect(res)
}

const setColumns = `id, session_id, user_id, slug, group_pos, exercise_pos, set_pos, kind, prescribed,
	weight_kg, reps, rpe, duration_s, distance_m, e1rm_kg, done_at, updated_at, deleted_at`

func scanSet(row interface{ Scan(...any) error }) (Set, error) {
	var s Set
	var prescribed, done, deleted sql.NullString
	var updated string
	var weight, rpe, distance, e1rm sql.NullFloat64
	var reps, duration sql.NullInt64
	err := row.Scan(&s.ID, &s.SessionID, &s.UserID, &s.Slug, &s.GroupPos, &s.ExercisePos, &s.SetPos, &s.Kind,
		&prescribed, &weight, &reps, &rpe, &duration, &distance, &e1rm, &done, &updated, &deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return Set{}, ErrNotFound
	}
	if err != nil {
		return Set{}, err
	}
	if prescribed.Valid {
		s.Prescribed = []byte(prescribed.String)
	}
	s.WeightKg, s.RPE, s.DistanceM, s.E1RMKg = nullFloat(weight), nullFloat(rpe), nullFloat(distance), nullFloat(e1rm)
	s.Reps, s.DurationS = nullInt(reps), nullInt(duration)
	if s.DoneAt, err = parseNullTime(done); err != nil {
		return Set{}, err
	}
	if s.DeletedAt, err = parseNullTime(deleted); err != nil {
		return Set{}, err
	}
	s.UpdatedAt, err = parseTime(updated)
	return s, err
}

func nullFloat(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	return &v.Float64
}

func nullInt(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int64)
	return &n
}

func scanSets(rows *sql.Rows, err error) ([]Set, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Set
	for rows.Next() {
		s, err := scanSet(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SessionSets returns a session's sets that are not deleted, in plan order.
func (db *DB) SessionSets(ctx context.Context, userID, sessionID string) ([]Set, error) {
	return scanSets(db.read.QueryContext(ctx, `SELECT `+setColumns+` FROM sets
		WHERE user_id = ? AND session_id = ? AND deleted_at IS NULL
		ORDER BY group_pos, set_pos, exercise_pos, done_at`, userID, sessionID))
}

// LastSets returns the sets of slug from the most recent other session that has any.
func (db *DB) LastSets(ctx context.Context, userID, slug, excludeSessionID string) ([]Set, error) {
	return scanSets(db.read.QueryContext(ctx, `SELECT `+setColumns+` FROM sets WHERE session_id = (
			SELECT s.session_id FROM sets s JOIN sessions ss ON ss.id = s.session_id
			WHERE s.user_id = ? AND s.slug = ? AND s.deleted_at IS NULL AND s.session_id != ?
			ORDER BY ss.started_at DESC, ss.id DESC LIMIT 1)
		AND slug = ? AND deleted_at IS NULL ORDER BY set_pos, done_at`, userID, slug, excludeSessionID, slug))
}

// BestE1RM is the highest e1RM of slug logged since since, or nil.
func (db *DB) BestE1RM(ctx context.Context, userID, slug string, since time.Time) (*float64, error) {
	var best sql.NullFloat64
	err := db.read.QueryRowContext(ctx, `SELECT max(e1rm_kg) FROM sets WHERE user_id = ? AND slug = ?
		AND deleted_at IS NULL AND done_at >= ?`, userID, slug, formatTime(since)).Scan(&best)
	return nullFloat(best), err
}

// recordOp runs apply once per opID ("" = no dedupe) inside a transaction.
func (db *DB) recordOp(ctx context.Context, userID, opID string, apply func(*sql.Tx) (Outcome, error)) (Outcome, error) {
	var out Outcome
	err := db.tx(ctx, func(tx *sql.Tx) error {
		if opID != "" {
			var prev string
			err := tx.QueryRowContext(ctx, `SELECT result FROM applied_ops WHERE op_id = ? AND user_id = ?`, opID, userID).Scan(&prev)
			if err == nil {
				out = Duplicate
				return nil
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		var err error
		if out, err = apply(tx); err != nil {
			return err
		}
		if opID == "" {
			return nil
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO applied_ops (op_id, user_id, result, applied_at) VALUES (?, ?, ?, ?)`,
			opID, userID, string(out), formatTime(time.Now()))
		return err
	})
	return out, err
}

func ownSession(ctx context.Context, tx *sql.Tx, userID, sessionID string) error {
	var one int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM sessions WHERE id = ? AND user_id = ?`, sessionID, userID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// UpsertSet creates or updates a set. The newer UpdatedAt wins; a deleted set
// stays deleted. A set of another user or session is ErrNotFound.
func (db *DB) UpsertSet(ctx context.Context, userID string, s Set, opID string) (Outcome, error) {
	return db.recordOp(ctx, userID, opID, func(tx *sql.Tx) (Outcome, error) {
		if err := ownSession(ctx, tx, userID, s.SessionID); err != nil {
			return "", err
		}
		existing, err := scanSet(tx.QueryRowContext(ctx, `SELECT `+setColumns+` FROM sets WHERE id = ?`, s.ID))
		switch {
		case errors.Is(err, ErrNotFound):
			_, err = tx.ExecContext(ctx, `INSERT INTO sets (`+setColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
				s.ID, s.SessionID, userID, s.Slug, s.GroupPos, s.ExercisePos, s.SetPos, s.Kind, nullBytes(s.Prescribed),
				s.WeightKg, s.Reps, s.RPE, s.DurationS, s.DistanceM, s.E1RMKg, nullTime(s.DoneAt), formatTime(s.UpdatedAt))
			return Applied, err
		case err != nil:
			return "", err
		case existing.UserID != userID || existing.SessionID != s.SessionID:
			return "", ErrNotFound
		case existing.DeletedAt != nil || !s.UpdatedAt.After(existing.UpdatedAt):
			return Ignored, nil
		}
		_, err = tx.ExecContext(ctx, `UPDATE sets SET slug = ?, group_pos = ?, exercise_pos = ?, set_pos = ?, kind = ?,
			prescribed = ?, weight_kg = ?, reps = ?, rpe = ?, duration_s = ?, distance_m = ?, e1rm_kg = ?, done_at = ?,
			updated_at = ? WHERE id = ?`,
			s.Slug, s.GroupPos, s.ExercisePos, s.SetPos, s.Kind, nullBytes(s.Prescribed), s.WeightKg, s.Reps, s.RPE,
			s.DurationS, s.DistanceM, s.E1RMKg, nullTime(s.DoneAt), formatTime(s.UpdatedAt), s.ID)
		return Applied, err
	})
}

func nullBytes(b []byte) sql.NullString {
	return sql.NullString{String: string(b), Valid: b != nil}
}

// DeleteSet soft-deletes a set of the user's session.
func (db *DB) DeleteSet(ctx context.Context, userID, sessionID, setID string, at time.Time, opID string) (Outcome, error) {
	return db.recordOp(ctx, userID, opID, func(tx *sql.Tx) (Outcome, error) {
		if err := ownSession(ctx, tx, userID, sessionID); err != nil {
			return "", err
		}
		res, err := tx.ExecContext(ctx, `UPDATE sets SET deleted_at = ?, updated_at = ? WHERE id = ? AND session_id = ?
			AND user_id = ? AND deleted_at IS NULL`, formatTime(at), formatTime(at), setID, sessionID, userID)
		if err != nil {
			return "", err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			// Deleting a set the server never saw: remember it as deleted so
			// its late upsert cannot create it.
			if _, err := tx.ExecContext(ctx, `INSERT INTO sets (id, session_id, user_id, slug, group_pos, exercise_pos,
				set_pos, kind, updated_at, deleted_at) VALUES (?, ?, ?, '', 0, 0, 0, 'working', ?, ?)
				ON CONFLICT (id) DO NOTHING`, setID, sessionID, userID, formatTime(at), formatTime(at)); err != nil {
				return "", err
			}
			return Ignored, nil
		}
		return Applied, nil
	})
}

// SetSessionNotes replaces the notes when at is newer than the last change.
func (db *DB) SetSessionNotes(ctx context.Context, userID, sessionID, notes string, at time.Time, opID string) (Outcome, error) {
	return db.recordOp(ctx, userID, opID, func(tx *sql.Tx) (Outcome, error) {
		if err := ownSession(ctx, tx, userID, sessionID); err != nil {
			return "", err
		}
		res, err := tx.ExecContext(ctx, `UPDATE sessions SET notes = ?, updated_at = ? WHERE id = ? AND updated_at < ?`,
			notes, formatTime(at), sessionID, formatTime(at))
		if err != nil {
			return "", err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return Ignored, nil
		}
		return Applied, nil
	})
}

// FinishSession marks the session finished at at. Finishing an already
// finished session is Ignored, so callers act on Applied only once.
func (db *DB) FinishSession(ctx context.Context, userID, sessionID string, at time.Time, opID string) (Outcome, error) {
	return db.recordOp(ctx, userID, opID, func(tx *sql.Tx) (Outcome, error) {
		if err := ownSession(ctx, tx, userID, sessionID); err != nil {
			return "", err
		}
		res, err := tx.ExecContext(ctx, `UPDATE sessions SET finished_at = ? WHERE id = ? AND finished_at IS NULL`,
			formatTime(at), sessionID)
		if err != nil {
			return "", err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return Ignored, nil
		}
		return Applied, nil
	})
}
