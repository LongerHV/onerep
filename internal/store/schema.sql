-- Desired database schema. Edit this file, then run `task migrate:diff NAME=<name>`
-- to generate a versioned migration in ./migrations.

CREATE TABLE users (
  id               TEXT    NOT NULL PRIMARY KEY,
  oidc_issuer      TEXT    NOT NULL,
  oidc_sub         TEXT    NOT NULL,
  email            TEXT    NOT NULL DEFAULT '',
  name             TEXT    NOT NULL DEFAULT '',
  unit             TEXT    NOT NULL DEFAULT 'kg' CHECK (unit IN ('kg', 'lb')),
  e1rm_window_days INTEGER NOT NULL DEFAULT 30,
  created_at       TEXT    NOT NULL,
  UNIQUE (oidc_issuer, oidc_sub)
);

CREATE TABLE auth_sessions (
  id_hash    TEXT NOT NULL PRIMARY KEY,
  user_id    TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  csrf_token TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE INDEX auth_sessions_user_id ON auth_sessions (user_id);
