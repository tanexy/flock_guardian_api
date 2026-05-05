-- Add column "uuid" to table: "brooders"
ALTER TABLE `brooders` ADD COLUMN `uuid` text NULL;
-- Add column "target_temperature" to table: "brooders"
ALTER TABLE `brooders` ADD COLUMN `target_temperature` real NULL;
-- Add column "target_humidity" to table: "brooders"
ALTER TABLE `brooders` ADD COLUMN `target_humidity` real NULL;
