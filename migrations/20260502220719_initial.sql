-- Create "historical_sensor_data" table
CREATE TABLE IF NOT EXISTS `historical_sensor_data` (
  `id` integer NULL PRIMARY KEY AUTOINCREMENT,
  `created_at` datetime NULL,
  `updated_at` datetime NULL,
  `deleted_at` datetime NULL,
  `brooder_id` integer NULL,
  `temperature` real NULL,
  `humidity` real NULL,
  `feed_level` real NULL,
  `water_level` real NULL,
  `recorded_at` datetime NULL
);
-- Create index "idx_historical_sensor_data_deleted_at" to table: "historical_sensor_data"
CREATE INDEX `idx_historical_sensor_data_deleted_at` ON `historical_sensor_data` (`deleted_at`);
