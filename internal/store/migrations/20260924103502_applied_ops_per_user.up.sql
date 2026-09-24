-- disable the enforcement of foreign-keys constraints
PRAGMA foreign_keys = off;
-- create "new_applied_ops" table
CREATE TABLE `new_applied_ops` (`user_id` text NOT NULL, `op_id` text NOT NULL, `result` text NOT NULL, `applied_at` text NOT NULL, PRIMARY KEY (`user_id`, `op_id`), CONSTRAINT `0` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON UPDATE NO ACTION ON DELETE CASCADE);
-- copy rows from old table "applied_ops" to new temporary table "new_applied_ops"
INSERT INTO `new_applied_ops` (`user_id`, `op_id`, `result`, `applied_at`) SELECT `user_id`, `op_id`, `result`, `applied_at` FROM `applied_ops`;
-- drop "applied_ops" table after copying rows
DROP TABLE `applied_ops`;
-- rename temporary table "new_applied_ops" to "applied_ops"
ALTER TABLE `new_applied_ops` RENAME TO `applied_ops`;
-- enable back the enforcement of foreign-keys constraints
PRAGMA foreign_keys = on;
