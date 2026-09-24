-- create "plans" table
CREATE TABLE `plans` (`id` text NOT NULL, `user_id` text NOT NULL, `name` text NOT NULL, `archived` integer NOT NULL DEFAULT 0, `created_at` text NOT NULL, PRIMARY KEY (`id`), CONSTRAINT `0` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON UPDATE NO ACTION ON DELETE CASCADE);
-- create index "plans_user_id" to table: "plans"
CREATE INDEX `plans_user_id` ON `plans` (`user_id`);
-- create "plan_versions" table
CREATE TABLE `plan_versions` (`id` text NOT NULL, `plan_id` text NOT NULL, `version` integer NOT NULL, `doc` text NOT NULL, `status` text NOT NULL, `source` text NOT NULL, `note` text NOT NULL DEFAULT '', `created_at` text NOT NULL, PRIMARY KEY (`id`), CONSTRAINT `0` FOREIGN KEY (`plan_id`) REFERENCES `plans` (`id`) ON UPDATE NO ACTION ON DELETE CASCADE, CHECK (status IN ('draft', 'active', 'superseded')), CHECK (source IN ('web', 'mcp')));
-- create index "plan_versions_plan_id_version" to table: "plan_versions"
CREATE UNIQUE INDEX `plan_versions_plan_id_version` ON `plan_versions` (`plan_id`, `version`);
-- create index "plan_versions_one_active" to table: "plan_versions"
CREATE UNIQUE INDEX `plan_versions_one_active` ON `plan_versions` (`plan_id`) WHERE status = 'active';
-- create "active_plan" table
CREATE TABLE `active_plan` (`user_id` text NOT NULL, `plan_id` text NOT NULL, `cursor_week` integer NOT NULL, `cursor_day` integer NOT NULL, `updated_at` text NOT NULL, PRIMARY KEY (`user_id`), CONSTRAINT `0` FOREIGN KEY (`plan_id`) REFERENCES `plans` (`id`) ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT `1` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON UPDATE NO ACTION ON DELETE CASCADE);
