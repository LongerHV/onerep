-- reverse: create index "training_max_log_user_slug" to table: "training_max_log"
DROP INDEX `training_max_log_user_slug`;
-- reverse: create "training_max_log" table
DROP TABLE `training_max_log`;
-- reverse: create index "user_exercise_equipment_id" to table: "user_exercise"
DROP INDEX `user_exercise_equipment_id`;
-- reverse: create "user_exercise" table
DROP TABLE `user_exercise`;
-- reverse: create index "exercise_alternatives_user" to table: "exercise_alternatives"
DROP INDEX `exercise_alternatives_user`;
-- reverse: create index "exercise_alternatives_global" to table: "exercise_alternatives"
DROP INDEX `exercise_alternatives_global`;
-- reverse: create "exercise_alternatives" table
DROP TABLE `exercise_alternatives`;
-- reverse: create index "exercises_user_slug" to table: "exercises"
DROP INDEX `exercises_user_slug`;
-- reverse: create index "exercises_global_slug" to table: "exercises"
DROP INDEX `exercises_global_slug`;
-- reverse: create "exercises" table
DROP TABLE `exercises`;
-- reverse: create index "equipment_one_default_per_kind" to table: "equipment"
DROP INDEX `equipment_one_default_per_kind`;
-- reverse: create index "equipment_user_id" to table: "equipment"
DROP INDEX `equipment_user_id`;
-- reverse: create "equipment" table
DROP TABLE `equipment`;
-- reverse: add column "equipment_initialized" to table: "users"
ALTER TABLE `users` DROP COLUMN `equipment_initialized`;
