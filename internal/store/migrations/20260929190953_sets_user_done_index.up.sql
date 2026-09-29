-- create index "sets_user_done" to table: "sets"
CREATE INDEX `sets_user_done` ON `sets` (`user_id`, `done_at`, `id`, `kind`, `rpe`, `deleted_at`, `slug`);
