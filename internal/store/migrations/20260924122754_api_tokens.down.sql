-- reverse: create index "api_tokens_user" to table: "api_tokens"
DROP INDEX `api_tokens_user`;
-- reverse: create index "api_tokens_hash" to table: "api_tokens"
DROP INDEX `api_tokens_hash`;
-- reverse: create "api_tokens" table
DROP TABLE `api_tokens`;
