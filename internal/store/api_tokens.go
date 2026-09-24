package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// APIToken is a personal access token for the MCP server (spec §11). Only
// its hash is stored.
type APIToken struct {
	ID         string
	UserID     string
	Name       string
	Scope      string
	LastUsedAt *time.Time
	CreatedAt  time.Time
	RevokedAt  *time.Time
}

func (db *DB) CreateAPIToken(ctx context.Context, userID, name, hash string, at time.Time) (APIToken, error) {
	id, err := newID()
	if err != nil {
		return APIToken{}, err
	}
	t := APIToken{ID: id, UserID: userID, Name: name, Scope: "mcp", CreatedAt: at.UTC()}
	_, err = db.write.ExecContext(ctx, `INSERT INTO api_tokens (id, user_id, name, token_hash, scope, created_at)
		VALUES (?, ?, ?, ?, 'mcp', ?)`, id, userID, name, hash, formatTime(at))
	return t, err
}

// APITokens lists the user's tokens, newest first, revoked ones included.
func (db *DB) APITokens(ctx context.Context, userID string) ([]APIToken, error) {
	rows, err := db.read.QueryContext(ctx, `SELECT id, user_id, name, scope, last_used_at, created_at, revoked_at
		FROM api_tokens WHERE user_id = ? ORDER BY created_at DESC, id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIToken
	for rows.Next() {
		var t APIToken
		var used, revoked sql.NullString
		var created string
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.Scope, &used, &created, &revoked); err != nil {
			return nil, err
		}
		if t.CreatedAt, err = parseTime(created); err != nil {
			return nil, err
		}
		if t.LastUsedAt, err = parseNullTime(used); err != nil {
			return nil, err
		}
		if t.RevokedAt, err = parseNullTime(revoked); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// RevokeAPIToken revokes one of the user's tokens. Revoking twice keeps the
// first revocation time.
func (db *DB) RevokeAPIToken(ctx context.Context, userID, id string, at time.Time) error {
	res, err := db.write.ExecContext(ctx, `UPDATE api_tokens SET revoked_at = coalesce(revoked_at, ?)
		WHERE id = ? AND user_id = ?`, formatTime(at), id, userID)
	if err != nil {
		return err
	}
	return mustAffect(res)
}

// UserByAPIToken returns the owner of an unrevoked token and records the use
// (at most once a minute, to keep MCP calls from writing on every request).
func (db *DB) UserByAPIToken(ctx context.Context, hash string, now time.Time) (User, error) {
	var tokenID, userID string
	var used sql.NullString
	err := db.read.QueryRowContext(ctx, `SELECT id, user_id, last_used_at FROM api_tokens
		WHERE token_hash = ? AND revoked_at IS NULL`, hash).Scan(&tokenID, &userID, &used)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	u, err := db.UserByID(ctx, userID)
	if err != nil {
		return User{}, err
	}
	last, err := parseNullTime(used)
	if err != nil {
		return User{}, err
	}
	if last == nil || now.Sub(*last) >= time.Minute {
		if _, err := db.write.ExecContext(ctx, `UPDATE api_tokens SET last_used_at = ? WHERE id = ?`, formatTime(now), tokenID); err != nil {
			return User{}, err
		}
	}
	return u, nil
}
