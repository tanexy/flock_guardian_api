-- Disable the enforcement of foreign-keys constraints
PRAGMA foreign_keys = off;
-- Create "new_brooders" table
CREATE TABLE `new_brooders` (
  `id` integer NULL PRIMARY KEY AUTOINCREMENT,
  `created_at` datetime NULL,
  `updated_at` datetime NULL,
  `deleted_at` datetime NULL,
  `uuid` text NULL,
  `name` text NULL,
  `location` text NULL,
  `flock_size` integer NULL,
  `mortality_count` integer NULL,
  `farm` text NULL,
  `temperature` real NULL,
  `humidity` real NULL,
  `feed_level` real NULL,
  `water_level` real NULL,
  `feed_capacity` real NULL,
  `target_temperature` real NULL,
  `target_humidity` real NULL,
  `fan_on` numeric NULL,
  `dispense_feed` numeric NULL,
  `dispense_water` numeric NULL,
  `heater_on` numeric NULL,
  `auto_mode` numeric NULL,
  `alert_active` numeric NULL DEFAULT true,
  `alert_message` text NULL DEFAULT 'System initialized',
  `last_updated` datetime NULL
);
-- Copy rows from old table "brooders" to new temporary table "new_brooders"
INSERT INTO `new_brooders` (`id`, `created_at`, `updated_at`, `deleted_at`, `uuid`, `name`, `location`, `farm`, `temperature`, `humidity`, `feed_level`, `water_level`, `feed_capacity`, `target_temperature`, `target_humidity`, `fan_on`, `dispense_feed`, `dispense_water`, `heater_on`, `auto_mode`, `alert_active`, `alert_message`, `last_updated`) SELECT `id`, `created_at`, `updated_at`, `deleted_at`, `uuid`, `name`, `location`, `farm`, `temperature`, `humidity`, `feed_level`, `water_level`, `feed_capacity`, `target_temperature`, `target_humidity`, `fan_on`, `dispense_feed`, `dispense_water`, `heater_on`, `auto_mode`, `alert_active`, `alert_message`, `last_updated` FROM `brooders`;
-- Drop "brooders" table after copying rows
DROP TABLE `brooders`;
-- Rename temporary table "new_brooders" to "brooders"
ALTER TABLE `new_brooders` RENAME TO `brooders`;
-- Create index "idx_brooders_uuid" to table: "brooders"
CREATE UNIQUE INDEX `idx_brooders_uuid` ON `brooders` (`uuid`);
-- Create index "idx_brooders_deleted_at" to table: "brooders"
CREATE INDEX `idx_brooders_deleted_at` ON `brooders` (`deleted_at`);
-- Enable back the enforcement of foreign-keys constraints
PRAGMA foreign_keys = on;
