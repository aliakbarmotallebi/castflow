-- Add a stable variant identifier for playback outputs (HLS/DASH).
-- This allows URL versioning without breaking existing videos when transcode settings change.
ALTER TABLE videos
  ADD COLUMN IF NOT EXISTS playback_variant TEXT NOT NULL DEFAULT '';

