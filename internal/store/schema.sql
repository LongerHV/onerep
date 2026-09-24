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
  -- set once the starter equipment profiles have been created
  equipment_initialized INTEGER NOT NULL DEFAULT 0,
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

CREATE TABLE equipment (
  id         TEXT    NOT NULL PRIMARY KEY,
  user_id    TEXT    NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  name       TEXT    NOT NULL,
  kind       TEXT    NOT NULL CHECK (kind IN ('barbell', 'dumbbell', 'machine', 'cable', 'bodyweight')),
  unit       TEXT    NOT NULL CHECK (unit IN ('kg', 'lb')),
  config     TEXT    NOT NULL, -- calc.EquipmentConfig as JSON, values in unit
  is_default INTEGER NOT NULL DEFAULT 0,
  created_at TEXT    NOT NULL
);

CREATE INDEX equipment_user_id ON equipment (user_id);
CREATE UNIQUE INDEX equipment_one_default_per_kind ON equipment (user_id, kind) WHERE is_default = 1;

-- Global rows (user_id NULL) are the seeded catalog. A user row with the same
-- slug shadows the global one for that user.
CREATE TABLE exercises (
  id                TEXT    NOT NULL PRIMARY KEY,
  user_id           TEXT    REFERENCES users (id) ON DELETE CASCADE,
  slug              TEXT    NOT NULL,
  name              TEXT    NOT NULL,
  measurement       TEXT    NOT NULL CHECK (measurement IN ('weight_reps', 'bw_reps', 'reps', 'time', 'distance_time')),
  equipment_kind    TEXT    NOT NULL CHECK (equipment_kind IN ('barbell', 'dumbbell', 'machine', 'cable', 'bodyweight')),
  primary_muscles   TEXT    NOT NULL DEFAULT '[]',
  secondary_muscles TEXT    NOT NULL DEFAULT '[]',
  aliases           TEXT    NOT NULL DEFAULT '[]',
  hidden            INTEGER NOT NULL DEFAULT 0, -- global rows dropped from the seed file
  created_at        TEXT    NOT NULL,
  updated_at        TEXT    NOT NULL
);

CREATE UNIQUE INDEX exercises_global_slug ON exercises (slug) WHERE user_id IS NULL;
CREATE UNIQUE INDEX exercises_user_slug ON exercises (user_id, slug) WHERE user_id IS NOT NULL;

-- Global rows come from the seed file; user rows add to them.
CREATE TABLE exercise_alternatives (
  user_id  TEXT REFERENCES users (id) ON DELETE CASCADE,
  slug     TEXT NOT NULL,
  alt_slug TEXT NOT NULL
);

CREATE UNIQUE INDEX exercise_alternatives_global ON exercise_alternatives (slug, alt_slug) WHERE user_id IS NULL;
CREATE UNIQUE INDEX exercise_alternatives_user ON exercise_alternatives (user_id, slug, alt_slug) WHERE user_id IS NOT NULL;

CREATE TABLE user_exercise (
  user_id         TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  slug            TEXT NOT NULL,
  equipment_id    TEXT REFERENCES equipment (id) ON DELETE SET NULL,
  training_max_kg REAL,
  updated_at      TEXT NOT NULL,
  PRIMARY KEY (user_id, slug)
);

CREATE INDEX user_exercise_equipment_id ON user_exercise (equipment_id);

CREATE TABLE training_max_log (
  id         TEXT NOT NULL PRIMARY KEY,
  user_id    TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  slug       TEXT NOT NULL,
  old_kg     REAL,
  new_kg     REAL,
  source     TEXT NOT NULL CHECK (source IN ('web', 'mcp')),
  note       TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE INDEX training_max_log_user_slug ON training_max_log (user_id, slug, created_at);

CREATE TABLE plans (
  id         TEXT    NOT NULL PRIMARY KEY,
  user_id    TEXT    NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  name       TEXT    NOT NULL,
  archived   INTEGER NOT NULL DEFAULT 0,
  created_at TEXT    NOT NULL
);

CREATE INDEX plans_user_id ON plans (user_id);

-- Every save creates a version. Active and superseded versions are immutable;
-- at most one version of a plan is active.
CREATE TABLE plan_versions (
  id         TEXT    NOT NULL PRIMARY KEY,
  plan_id    TEXT    NOT NULL REFERENCES plans (id) ON DELETE CASCADE,
  version    INTEGER NOT NULL,
  doc        TEXT    NOT NULL, -- the plan as authored (JSON)
  status     TEXT    NOT NULL CHECK (status IN ('draft', 'active', 'superseded')),
  source     TEXT    NOT NULL CHECK (source IN ('web', 'mcp')),
  note       TEXT    NOT NULL DEFAULT '',
  created_at TEXT    NOT NULL,
  UNIQUE (plan_id, version)
);

CREATE UNIQUE INDEX plan_versions_one_active ON plan_versions (plan_id) WHERE status = 'active';

-- The plan a user is following and the next day to train (spec §8).
-- cursor_week past the plan's last week means the block is complete.
CREATE TABLE active_plan (
  user_id     TEXT    NOT NULL PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
  plan_id     TEXT    NOT NULL REFERENCES plans (id) ON DELETE CASCADE,
  cursor_week INTEGER NOT NULL,
  cursor_day  INTEGER NOT NULL,
  updated_at  TEXT    NOT NULL
);
