-- reverse: create index "auth_sessions_user_id" to table: "auth_sessions"
DROP INDEX `auth_sessions_user_id`;
-- reverse: create "auth_sessions" table
DROP TABLE `auth_sessions`;
-- reverse: create index "users_oidc_issuer_oidc_sub" to table: "users"
DROP INDEX `users_oidc_issuer_oidc_sub`;
-- reverse: create "users" table
DROP TABLE `users`;
