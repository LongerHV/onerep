package store

import (
	"context"
	"database/sql"
	"time"
)

// Stats are plain queries over sets (spec §5: no aggregate tables in v1).
// Stored timestamps share one fixed format, so they compare correctly as text.

// prKinds are the set kinds that can be, or count toward, a PR (spec §13).
const prKinds = `kind IN ('working', 'amrap') AND weight_kg IS NOT NULL AND reps >= 1 AND done_at IS NOT NULL AND deleted_at IS NULL`

// RepMax is the heaviest working or AMRAP set of an exercise at a rep count.
type RepMax struct {
	Reps      int
	WeightKg  float64
	DoneAt    time.Time // when that weight was first reached
	SessionID string
}

// RepMaxes returns the best weight per rep count for slug, lowest reps first,
// ignoring sets of excludeSessionID ("" excludes nothing).
func (db *DB) RepMaxes(ctx context.Context, userID, slug, excludeSessionID string) ([]RepMax, error) {
	rows, err := db.read.QueryContext(ctx, `SELECT reps, weight_kg, done_at, session_id FROM (
		SELECT reps, weight_kg, done_at, session_id,
			row_number() OVER (PARTITION BY reps ORDER BY weight_kg DESC, done_at, id) AS rank
		FROM sets WHERE user_id = ? AND slug = ? AND session_id != ? AND `+prKinds+`
	) WHERE rank = 1 ORDER BY reps`, userID, slug, excludeSessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RepMax
	for rows.Next() {
		var m RepMax
		var done string
		if err := rows.Scan(&m.Reps, &m.WeightKg, &done, &m.SessionID); err != nil {
			return nil, err
		}
		if m.DoneAt, err = parseTime(done); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// E1RMPoint is the best e1RM of one session.
type E1RMPoint struct {
	SessionID string
	DoneAt    time.Time
	E1RMKg    float64
	WeightKg  float64
	Reps      int
	RPE       *float64
}

// E1RMSeries returns the session-best e1RM of slug for each session, oldest
// first. Warmups don't count.
func (db *DB) E1RMSeries(ctx context.Context, userID, slug string) ([]E1RMPoint, error) {
	rows, err := db.read.QueryContext(ctx, `SELECT session_id, done_at, e1rm_kg, weight_kg, reps, rpe FROM (
		SELECT session_id, done_at, e1rm_kg, weight_kg, reps, rpe,
			row_number() OVER (PARTITION BY session_id ORDER BY e1rm_kg DESC, done_at, id) AS rank
		FROM sets WHERE user_id = ? AND slug = ? AND kind != 'warmup' AND e1rm_kg IS NOT NULL
			AND weight_kg IS NOT NULL AND reps IS NOT NULL AND done_at IS NOT NULL AND deleted_at IS NULL
	) WHERE rank = 1 ORDER BY done_at, session_id`, userID, slug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []E1RMPoint
	for rows.Next() {
		var p E1RMPoint
		var done string
		var rpe sql.NullFloat64
		if err := rows.Scan(&p.SessionID, &done, &p.E1RMKg, &p.WeightKg, &p.Reps, &rpe); err != nil {
			return nil, err
		}
		if p.DoneAt, err = parseTime(done); err != nil {
			return nil, err
		}
		p.RPE = nullFloat(rpe)
		out = append(out, p)
	}
	return out, rows.Err()
}

// SessionPRs returns the ids of the session's sets that beat the heaviest
// earlier set of the same exercise at the same reps (spec §13). A set with no
// earlier set to beat is not a PR.
func (db *DB) SessionPRs(ctx context.Context, userID, sessionID string) (map[string]bool, error) {
	rows, err := db.read.QueryContext(ctx, `SELECT s.id FROM sets s
		WHERE s.user_id = ? AND s.session_id = ? AND s.`+prKinds+` AND s.weight_kg > (
			SELECT max(p.weight_kg) FROM sets p
			WHERE p.user_id = s.user_id AND p.slug = s.slug AND p.reps = s.reps
				AND p.kind IN ('working', 'amrap') AND p.weight_kg IS NOT NULL AND p.done_at IS NOT NULL AND p.deleted_at IS NULL
				AND (p.done_at < s.done_at OR (p.done_at = s.done_at AND p.id < s.id)))`, userID, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// HardSet is a set that counts toward weekly muscle volume.
type HardSet struct {
	Slug   string
	DoneAt time.Time
}

// HardSets returns the hard sets done in [from, to), oldest first: working,
// drop and AMRAP sets at RPE 7 or more, or with no RPE (spec §13).
func (db *DB) HardSets(ctx context.Context, userID string, from, to time.Time) ([]HardSet, error) {
	rows, err := db.read.QueryContext(ctx, `SELECT slug, done_at FROM sets
		WHERE user_id = ? AND kind IN ('working', 'drop', 'amrap') AND (rpe IS NULL OR rpe >= 7)
			AND deleted_at IS NULL AND done_at >= ? AND done_at < ?
		ORDER BY done_at, id`, userID, formatTime(from), formatTime(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HardSet
	for rows.Next() {
		var h HardSet
		var done string
		if err := rows.Scan(&h.Slug, &done); err != nil {
			return nil, err
		}
		if h.DoneAt, err = parseTime(done); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
