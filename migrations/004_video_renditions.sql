-- Per-profile playback renditions (default, mobile, …) with revision versioning.
CREATE TABLE IF NOT EXISTS video_renditions (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  video_id      UUID NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
  profile       TEXT NOT NULL,
  revision      TEXT NOT NULL,
  status        TEXT NOT NULL DEFAULT 'processing',
  duration_sec  INT NOT NULL DEFAULT 0,
  qualities     TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (video_id, profile, revision)
);

CREATE INDEX IF NOT EXISTS idx_renditions_video_id ON video_renditions (video_id);
CREATE INDEX IF NOT EXISTS idx_renditions_video_profile ON video_renditions (video_id, profile);
