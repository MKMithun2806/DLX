-- migrations/20260624_vnext_create_videos_channels_playlists.sql
-- Migration: vNext - create videos, channels, playlists and migrate existing downloads into videos
-- NOTE: This migration is written for SQLite. Review before applying to other DBs.
PRAGMA foreign_keys = OFF;
BEGIN TRANSACTION;

-- 1. Create channels table
CREATE TABLE IF NOT EXISTS channels (
  channel_id TEXT PRIMARY KEY,
  name TEXT,
  thumbnail_s3_key TEXT,
  channel_url TEXT,
  metadata_json TEXT,
  slug TEXT,
  created_at TEXT DEFAULT (datetime('now')),
  updated_at TEXT DEFAULT (datetime('now'))
);

-- 2. Create playlists table
CREATE TABLE IF NOT EXISTS playlists (
  playlist_id TEXT PRIMARY KEY,
  title TEXT,
  description TEXT,
  thumbnail_s3_key TEXT,
  metadata_json TEXT,
  slug TEXT,
  created_at TEXT DEFAULT (datetime('now')),
  updated_at TEXT DEFAULT (datetime('now'))
);

-- 3. Create videos table
CREATE TABLE IF NOT EXISTS videos (
  video_id TEXT PRIMARY KEY,
  title TEXT,
  description TEXT,

  channel_id TEXT,
  channel_name TEXT,

  duration INTEGER,

  thumbnail_s3_key TEXT,

  video_s3_key TEXT,

  upload_date TEXT,

  webpage_url TEXT,

  extractor TEXT,

  video_type TEXT,

  filesize INTEGER,

  metadata_json TEXT,

  slug TEXT,

  created_at TEXT DEFAULT (datetime('now')),
  updated_at TEXT DEFAULT (datetime('now')),

  FOREIGN KEY(channel_id) REFERENCES channels(channel_id)
);

-- 4. Create playlist_videos join table
CREATE TABLE IF NOT EXISTS playlist_videos (
  playlist_id TEXT,
  video_id TEXT,
  position INTEGER,
  PRIMARY KEY(playlist_id, video_id),
  FOREIGN KEY(playlist_id) REFERENCES playlists(playlist_id),
  FOREIGN KEY(video_id) REFERENCES videos(video_id)
);

-- 5. Add video_id to downloads to link job records to videos (if not present)
ALTER TABLE downloads ADD COLUMN video_id TEXT;

-- 6. Create FTS5 virtual tables for search (title, description, channel, tags)
-- Videos FTS will index title and description and channel_name
CREATE VIRTUAL TABLE IF NOT EXISTS videos_fts USING fts5(
  video_id UNINDEXED,
  title,
  description,
  channel_name,
  metadata_json,
  content='videos', content_rowid='rowid'
);

CREATE VIRTUAL TABLE IF NOT EXISTS channels_fts USING fts5(
  channel_id UNINDEXED,
  name,
  metadata_json,
  content='channels', content_rowid='rowid'
);

CREATE VIRTUAL TABLE IF NOT EXISTS playlists_fts USING fts5(
  playlist_id UNINDEXED,
  title,
  description,
  metadata_json,
  content='playlists', content_rowid='rowid'
);

-- 7. Populate videos from existing downloads. We try to preserve fields:
--   title -> title
--   uploader -> channel_name
--   thumbnail -> thumbnail_s3_key (we store original URL in metadata_json; thumbnail files will be re-downloaded later)
--   duration -> duration
--   filesize -> filesize
--   s3_key -> video_s3_key
-- This block is conservative: it only migrates rows that look like completed downloads (s3_key IS NOT NULL OR local path present)

INSERT INTO videos (video_id, title, channel_name, duration, thumbnail_s3_key, video_s3_key, filesize, webpage_url, created_at, updated_at, metadata_json, video_type)
SELECT
  'dl_' || rowid AS video_id,
  COALESCE(title, '') AS title,
  COALESCE(uploader, '') AS channel_name,
  CASE WHEN duration IS NOT NULL AND typeof(duration) IN ('integer','real') THEN CAST(duration AS INTEGER) ELSE NULL END AS duration,
  COALESCE(thumbnail, '') AS thumbnail_s3_key,
  COALESCE(s3_key, '') AS video_s3_key,
  CASE WHEN filesize IS NOT NULL THEN CAST(filesize AS INTEGER) ELSE NULL END AS filesize,
  COALESCE(webpage_url, '') AS webpage_url,
  COALESCE(created_at, datetime('now')) AS created_at,
  COALESCE(updated_at, datetime('now')) AS updated_at,
  json_object(
    'migrated_from_download_rowid', rowid,
    'original_thumbnail_url', COALESCE(thumbnail, ''),
    'original_uploader', COALESCE(uploader, ''),
    'note', 'migrated during vnext migration'
  ) AS metadata_json,
  'unknown' AS video_type
FROM downloads
WHERE (s3_key IS NOT NULL AND s3_key != '') OR (webpage_url IS NOT NULL AND webpage_url != '');

-- 8. Backfill downloads.video_id with the generated video ids for migrated rows
UPDATE downloads
SET video_id = 'dl_' || rowid
WHERE (s3_key IS NOT NULL AND s3_key != '') OR (webpage_url IS NOT NULL AND webpage_url != '');

-- 9. Populate FTS tables from the current rows
INSERT INTO videos_fts (rowid, video_id, title, description, channel_name, metadata_json)
SELECT rowid, video_id, title, description, channel_name, COALESCE(metadata_json, '') FROM videos;

INSERT INTO channels_fts (rowid, channel_id, name, metadata_json)
SELECT rowid, channel_id, name, COALESCE(metadata_json, '') FROM channels;

INSERT INTO playlists_fts (rowid, playlist_id, title, description, metadata_json)
SELECT rowid, playlist_id, title, description, COALESCE(metadata_json, '') FROM playlists;

COMMIT;
PRAGMA foreign_keys = ON;
