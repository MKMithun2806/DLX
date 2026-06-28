-- 0002_package_layout.sql
-- Add package-aware storage fields and metadata persistence.

ALTER TABLE downloads ADD COLUMN video_s3_key TEXT NOT NULL DEFAULT '';
ALTER TABLE downloads ADD COLUMN thumbnail_s3_key TEXT NOT NULL DEFAULT '';
ALTER TABLE downloads ADD COLUMN metadata_s3_key TEXT NOT NULL DEFAULT '';
ALTER TABLE downloads ADD COLUMN metadata_json TEXT NOT NULL DEFAULT '';
