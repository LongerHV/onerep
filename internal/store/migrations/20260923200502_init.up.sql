-- create "users" table
CREATE TABLE `users` (`id` text NOT NULL, `oidc_issuer` text NOT NULL, `oidc_sub` text NOT NULL, `email` text NOT NULL DEFAULT '', `name` text NOT NULL DEFAULT '', `unit` text NOT NULL DEFAULT 'kg', `e1rm_window_days` integer NOT NULL DEFAULT 30, `created_at` text NOT NULL, PRIMARY KEY (`id`), CHECK (unit IN ('kg', 'lb')));
-- create index "users_oidc_issuer_oidc_sub" to table: "users"
CREATE UNIQUE INDEX `users_oidc_issuer_oidc_sub` ON `users` (`oidc_issuer`, `oidc_sub`);
-- create "auth_sessions" table
CREATE TABLE `auth_sessions` (`id_hash` text NOT NULL, `user_id` text NOT NULL, `csrf_token` text NOT NULL, `expires_at` text NOT NULL, `created_at` text NOT NULL, PRIMARY KEY (`id_hash`), CONSTRAINT `0` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON UPDATE NO ACTION ON DELETE CASCADE);
-- create index "auth_sessions_user_id" to table: "auth_sessions"
CREATE INDEX `auth_sessions_user_id` ON `auth_sessions` (`user_id`);
