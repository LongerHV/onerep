package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type AuthSession struct {
	IDHash    string
	UserID    string
	CSRFToken string
	ExpiresAt time.Time
	CreatedAt time.Time
}

func (db *DB) CreateAuthSession(ctx context.Context, s AuthSession) error {
	_, err := db.write.ExecContext(ctx, `
		INSERT INTO auth_sessions (id_hash, user_id, csrf_token, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		s.IDHash, s.UserID, s.CSRFToken, formatTime(s.ExpiresAt), formatTime(s.CreatedAt))
	return err
}

// AuthSessionByHash returns the session and its user. Expired sessions are
// reported as ErrNotFound.
func (db *DB) AuthSessionByHash(ctx context.Context, idHash string, now time.Time) (AuthSession, User, error) {
	row := db.read.QueryRowContext(ctx, `
		SELECT s.id_hash, s.user_id, s.csrf_token, s.expires_at, s.created_at,
		       u.id, u.oidc_issuer, u.oidc_sub, u.email, u.name, u.unit, u.e1rm_window_days, u.created_at
		FROM auth_sessions s JOIN users u ON u.id = s.user_id
		WHERE s.id_hash = ? AND s.expires_at > ?`, idHash, formatTime(now))
	var s AuthSession
	var u User
	var sExp, sCreated, uCreated string
	err := row.Scan(&s.IDHash, &s.UserID, &s.CSRFToken, &sExp, &sCreated,
		&u.ID, &u.OIDCIssuer, &u.OIDCSubject, &u.Email, &u.Name, &u.Unit, &u.E1RMWindowDays, &uCreated)
	if errors.Is(err, sql.ErrNoRows) {
		return AuthSession{}, User{}, ErrNotFound
	}
	if err != nil {
		return AuthSession{}, User{}, err
	}
	if s.ExpiresAt, err = parseTime(sExp); err != nil {
		return AuthSession{}, User{}, err
	}
	if s.CreatedAt, err = parseTime(sCreated); err != nil {
		return AuthSession{}, User{}, err
	}
	if u.CreatedAt, err = parseTime(uCreated); err != nil {
		return AuthSession{}, User{}, err
	}
	return s, u, nil
}

func (db *DB) ExtendAuthSession(ctx context.Context, idHash string, expiresAt time.Time) error {
	_, err := db.write.ExecContext(ctx, `UPDATE auth_sessions SET expires_at = ? WHERE id_hash = ?`,
		formatTime(expiresAt), idHash)
	return err
}

func (db *DB) DeleteAuthSession(ctx context.Context, idHash string) error {
	_, err := db.write.ExecContext(ctx, `DELETE FROM auth_sessions WHERE id_hash = ?`, idHash)
	return err
}

// DeleteExpiredAuthSessions removes sessions that expired before now.
func (db *DB) DeleteExpiredAuthSessions(ctx context.Context, now time.Time) (int64, error) {
	res, err := db.write.ExecContext(ctx, `DELETE FROM auth_sessions WHERE expires_at <= ?`, formatTime(now))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
