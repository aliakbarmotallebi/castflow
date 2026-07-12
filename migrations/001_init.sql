-- Castflow VOD platform schema
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TYPE video_status AS ENUM (
  'uploading',
  'uploaded',
  'processing',
  'ready',
  'error'
);

CREATE TABLE IF NOT EXISTS videos (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  title         TEXT NOT NULL,
  description   TEXT NOT NULL DEFAULT '',
  status        video_status NOT NULL DEFAULT 'uploading',
  duration_sec  INT NOT NULL DEFAULT 0,
  file_size     BIGINT NOT NULL DEFAULT 0,
  content_type  TEXT NOT NULL DEFAULT 'video/mp4',
  origin_key    TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_videos_status ON videos (status);
CREATE INDEX IF NOT EXISTS idx_videos_created_at ON videos (created_at DESC);
