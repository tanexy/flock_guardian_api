-- Create "users" table
CREATE TABLE `users` (
  `id` integer NULL PRIMARY KEY AUTOINCREMENT,
  `created_at` datetime NULL,
  `updated_at` datetime NULL,
  `deleted_at` datetime NULL,
  `username` text NULL,
  `password` text NULL,
  `email` text NULL,
  `farm` text NULL
);
-- Create index "users_email" to table: "users"
CREATE UNIQUE INDEX `users_email` ON `users` (`email`);
-- Create index "idx_users_deleted_at" to table: "users"
CREATE INDEX `idx_users_deleted_at` ON `users` (`deleted_at`);
-- Create "brooders" table
CREATE TABLE `brooders` (
  `id` integer NULL PRIMARY KEY AUTOINCREMENT,
  `created_at` datetime NULL,
  `updated_at` datetime NULL,
  `deleted_at` datetime NULL,
  `name` text NULL,
  `location` text NULL,
  `farm` integer NULL,
  `temperature` real NULL,
  `humidity` real NULL,
  `feed_level` real NULL,
  `water_level` real NULL,
  `feed_capacity` real NULL,
  `fan_on` numeric NULL,
  `dispense_feed` numeric NULL,
  `dispense_water` numeric NULL,
  `heater_on` numeric NULL,
  `auto_mode` numeric NULL,
  `alert_active` numeric NULL DEFAULT true,
  `alert_message` text NULL DEFAULT 'System initialized',
  `last_updated` datetime NULL
);
-- Create index "idx_brooders_deleted_at" to table: "brooders"
CREATE INDEX `idx_brooders_deleted_at` ON `brooders` (`deleted_at`);
