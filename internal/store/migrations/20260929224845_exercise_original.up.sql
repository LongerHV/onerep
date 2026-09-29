-- add column "original" to table: "exercises"
ALTER TABLE `exercises` ADD COLUMN `original` integer NOT NULL DEFAULT 0;
