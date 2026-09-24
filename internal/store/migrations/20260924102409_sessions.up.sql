-- create "sessions" table
CREATE TABLE `sessions` (`id` text NOT NULL, `user_id` text NOT NULL, `plan_id` text NULL, `plan_version_id` text NULL, `week` integer NOT NULL DEFAULT 0, `day` integer NOT NULL DEFAULT 0, `name` text NOT NULL, `snapshot` text NOT NULL, `started_at` text NOT NULL, `finished_at` text NULL, `notes` text NOT NULL DEFAULT '', `updated_at` text NOT NULL, PRIMARY KEY (`id`), CONSTRAINT `0` FOREIGN KEY (`plan_version_id`) REFERENCES `plan_versions` (`id`) ON UPDATE NO ACTION ON DELETE SET NULL, CONSTRAINT `1` FOREIGN KEY (`plan_id`) REFERENCES `plans` (`id`) ON UPDATE NO ACTION ON DELETE SET NULL, CONSTRAINT `2` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON UPDATE NO ACTION ON DELETE CASCADE);
-- create index "sessions_user_started" to table: "sessions"
CREATE INDEX `sessions_user_started` ON `sessions` (`user_id`, `started_at`);
-- create "sets" table
CREATE TABLE `sets` (`id` text NOT NULL, `session_id` text NOT NULL, `user_id` text NOT NULL, `slug` text NOT NULL, `group_pos` integer NOT NULL, `exercise_pos` integer NOT NULL, `set_pos` integer NOT NULL, `kind` text NOT NULL, `prescribed` text NULL, `weight_kg` real NULL, `reps` integer NULL, `rpe` real NULL, `duration_s` integer NULL, `distance_m` real NULL, `e1rm_kg` real NULL, `done_at` text NULL, `updated_at` text NOT NULL, `deleted_at` text NULL, PRIMARY KEY (`id`), CONSTRAINT `0` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT `1` FOREIGN KEY (`session_id`) REFERENCES `sessions` (`id`) ON UPDATE NO ACTION ON DELETE CASCADE, CHECK (kind IN ('warmup', 'working', 'drop', 'amrap')));
-- create index "sets_session" to table: "sets"
CREATE INDEX `sets_session` ON `sets` (`session_id`);
-- create index "sets_user_slug_done" to table: "sets"
CREATE INDEX `sets_user_slug_done` ON `sets` (`user_id`, `slug`, `done_at`);
-- create "applied_ops" table
CREATE TABLE `applied_ops` (`op_id` text NOT NULL, `user_id` text NOT NULL, `result` text NOT NULL, `applied_at` text NOT NULL, PRIMARY KEY (`op_id`), CONSTRAINT `0` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON UPDATE NO ACTION ON DELETE CASCADE);
