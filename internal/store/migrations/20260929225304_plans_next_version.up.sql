-- add column "next_version" to table: "plans"
ALTER TABLE `plans` ADD COLUMN `next_version` integer NOT NULL DEFAULT 0;
