BEGIN;

-- Emoji interaction statistics (single image metrics + trend windows)
CREATE INDEX IF NOT EXISTS idx_action_favorites_emoji_created
  ON action.favorites (emoji_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_action_likes_emoji_created
  ON action.likes (emoji_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_action_downloads_emoji_created
  ON action.downloads (emoji_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_action_favorites_created
  ON action.favorites (created_at DESC);

CREATE INDEX IF NOT EXISTS idx_action_likes_created
  ON action.likes (created_at DESC);

-- Collection interaction statistics (creator overview + trend windows)
CREATE INDEX IF NOT EXISTS idx_collection_favorites_collection_created
  ON action.collection_favorites (collection_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_collection_likes_collection_created
  ON action.collection_likes (collection_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_collection_downloads_collection_created
  ON action.collection_downloads (collection_id, created_at DESC);

COMMIT;

