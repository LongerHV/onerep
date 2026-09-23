package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID             string
	OIDCIssuer     string
	OIDCSubject    string
	Email          string
	Name           string
	Unit           string
	E1RMWindowDays int
	CreatedAt      time.Time
}

const userColumns = `id, oidc_issuer, oidc_sub, email, name, unit, e1rm_window_days, created_at`

func scanUser(row interface{ Scan(...any) error }) (User, error) {
	var u User
	var created string
	err := row.Scan(&u.ID, &u.OIDCIssuer, &u.OIDCSubject, &u.Email, &u.Name, &u.Unit, &u.E1RMWindowDays, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	u.CreatedAt, err = parseTime(created)
	return u, err
}

// UpsertOIDCUser returns the user identified by (issuer, subject), creating it on
// first login. Email and name are refreshed from the identity provider every time.
func (db *DB) UpsertOIDCUser(ctx context.Context, issuer, subject, email, name string) (User, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return User{}, err
	}
	row := db.write.QueryRowContext(ctx, `
		INSERT INTO users (id, oidc_issuer, oidc_sub, email, name, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (oidc_issuer, oidc_sub) DO UPDATE SET email = excluded.email, name = excluded.name
		RETURNING `+userColumns,
		id.String(), issuer, subject, email, name, formatTime(time.Now()))
	return scanUser(row)
}

func (db *DB) UserByID(ctx context.Context, id string) (User, error) {
	return scanUser(db.read.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = ?`, id))
}
