package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
)

// tx runs fn in a write transaction, committing if fn returns nil.
func (db *DB) tx(ctx context.Context, fn func(*sql.Tx) error) error {
	t, err := db.write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(t); err != nil {
		return errors.Join(err, t.Rollback())
	}
	return t.Commit()
}

// mustAffect turns "no rows changed" into ErrNotFound.
func mustAffect(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// newID returns a new UUIDv7 string.
func newID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

// jsonList encodes a string list for a JSON TEXT column; nil becomes [].
func jsonList(vs []string) string {
	if vs == nil {
		vs = []string{}
	}
	b, _ := json.Marshal(vs)
	return string(b)
}

func parseList(s string) ([]string, error) {
	var vs []string
	return vs, json.Unmarshal([]byte(s), &vs)
}

// nullString maps "" to NULL.
func nullString(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }
