-- create "api_tokens" table
CREATE TABLE `api_tokens` (`id` text NOT NULL, `user_id` text NOT NULL, `name` text NOT NULL, `token_hash` text NOT NULL, `scope` text NOT NULL DEFAULT 'mcp', `last_used_at` text NULL, `created_at` text NOT NULL, `revoked_at` text NULL, PRIMARY KEY (`id`), CONSTRAINT `0` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON UPDATE NO ACTION ON DELETE CASCADE, CHECK (scope IN ('mcp')));
-- create index "api_tokens_hash" to table: "api_tokens"
CREATE UNIQUE INDEX `api_tokens_hash` ON `api_tokens` (`token_hash`);
-- create index "api_tokens_user" to table: "api_tokens"
CREATE INDEX `api_tokens_user` ON `api_tokens` (`user_id`);
