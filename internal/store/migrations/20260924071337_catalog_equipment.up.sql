-- add column "equipment_initialized" to table: "users"
ALTER TABLE `users` ADD COLUMN `equipment_initialized` integer NOT NULL DEFAULT 0;
-- create "equipment" table
CREATE TABLE `equipment` (`id` text NOT NULL, `user_id` text NOT NULL, `name` text NOT NULL, `kind` text NOT NULL, `unit` text NOT NULL, `config` text NOT NULL, `is_default` integer NOT NULL DEFAULT 0, `created_at` text NOT NULL, PRIMARY KEY (`id`), CONSTRAINT `0` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON UPDATE NO ACTION ON DELETE CASCADE, CHECK (kind IN ('barbell', 'dumbbell', 'machine', 'cable', 'bodyweight')), CHECK (unit IN ('kg', 'lb')));
-- create index "equipment_user_id" to table: "equipment"
CREATE INDEX `equipment_user_id` ON `equipment` (`user_id`);
-- create index "equipment_one_default_per_kind" to table: "equipment"
CREATE UNIQUE INDEX `equipment_one_default_per_kind` ON `equipment` (`user_id`, `kind`) WHERE is_default = 1;
-- create "exercises" table
CREATE TABLE `exercises` (`id` text NOT NULL, `user_id` text NULL, `slug` text NOT NULL, `name` text NOT NULL, `measurement` text NOT NULL, `equipment_kind` text NOT NULL, `primary_muscles` text NOT NULL DEFAULT '[]', `secondary_muscles` text NOT NULL DEFAULT '[]', `aliases` text NOT NULL DEFAULT '[]', `hidden` integer NOT NULL DEFAULT 0, `created_at` text NOT NULL, `updated_at` text NOT NULL, PRIMARY KEY (`id`), CONSTRAINT `0` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON UPDATE NO ACTION ON DELETE CASCADE, CHECK (measurement IN ('weight_reps', 'bw_reps', 'reps', 'time', 'distance_time')), CHECK (equipment_kind IN ('barbell', 'dumbbell', 'machine', 'cable', 'bodyweight')));
-- create index "exercises_global_slug" to table: "exercises"
CREATE UNIQUE INDEX `exercises_global_slug` ON `exercises` (`slug`) WHERE user_id IS NULL;
-- create index "exercises_user_slug" to table: "exercises"
CREATE UNIQUE INDEX `exercises_user_slug` ON `exercises` (`user_id`, `slug`) WHERE user_id IS NOT NULL;
-- create "exercise_alternatives" table
CREATE TABLE `exercise_alternatives` (`user_id` text NULL, `slug` text NOT NULL, `alt_slug` text NOT NULL, CONSTRAINT `0` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON UPDATE NO ACTION ON DELETE CASCADE);
-- create index "exercise_alternatives_global" to table: "exercise_alternatives"
CREATE UNIQUE INDEX `exercise_alternatives_global` ON `exercise_alternatives` (`slug`, `alt_slug`) WHERE user_id IS NULL;
-- create index "exercise_alternatives_user" to table: "exercise_alternatives"
CREATE UNIQUE INDEX `exercise_alternatives_user` ON `exercise_alternatives` (`user_id`, `slug`, `alt_slug`) WHERE user_id IS NOT NULL;
-- create "user_exercise" table
CREATE TABLE `user_exercise` (`user_id` text NOT NULL, `slug` text NOT NULL, `equipment_id` text NULL, `training_max_kg` real NULL, `updated_at` text NOT NULL, PRIMARY KEY (`user_id`, `slug`), CONSTRAINT `0` FOREIGN KEY (`equipment_id`) REFERENCES `equipment` (`id`) ON UPDATE NO ACTION ON DELETE SET NULL, CONSTRAINT `1` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON UPDATE NO ACTION ON DELETE CASCADE);
-- create index "user_exercise_equipment_id" to table: "user_exercise"
CREATE INDEX `user_exercise_equipment_id` ON `user_exercise` (`equipment_id`);
-- create "training_max_log" table
CREATE TABLE `training_max_log` (`id` text NOT NULL, `user_id` text NOT NULL, `slug` text NOT NULL, `old_kg` real NULL, `new_kg` real NULL, `source` text NOT NULL, `note` text NOT NULL DEFAULT '', `created_at` text NOT NULL, PRIMARY KEY (`id`), CONSTRAINT `0` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON UPDATE NO ACTION ON DELETE CASCADE, CHECK (source IN ('web', 'mcp')));
-- create index "training_max_log_user_slug" to table: "training_max_log"
CREATE INDEX `training_max_log_user_slug` ON `training_max_log` (`user_id`, `slug`, `created_at`);
