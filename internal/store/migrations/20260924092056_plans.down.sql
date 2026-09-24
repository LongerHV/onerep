-- reverse: create "active_plan" table
DROP TABLE `active_plan`;
-- reverse: create index "plan_versions_one_active" to table: "plan_versions"
DROP INDEX `plan_versions_one_active`;
-- reverse: create index "plan_versions_plan_id_version" to table: "plan_versions"
DROP INDEX `plan_versions_plan_id_version`;
-- reverse: create "plan_versions" table
DROP TABLE `plan_versions`;
-- reverse: create index "plans_user_id" to table: "plans"
DROP INDEX `plans_user_id`;
-- reverse: create "plans" table
DROP TABLE `plans`;
