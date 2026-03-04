BEGIN;

ALTER TABLE IF EXISTS archive.collection_zips
  ADD COLUMN IF NOT EXISTS zip_hash VARCHAR(128);

CREATE UNIQUE INDEX IF NOT EXISTS idx_collection_zips_collection_hash_unique
  ON archive.collection_zips (collection_id, zip_hash)
  WHERE zip_hash IS NOT NULL AND zip_hash <> '';

CREATE INDEX IF NOT EXISTS idx_archive_collections_public_listing
  ON archive.collections (status, visibility, is_pinned, id DESC);

CREATE INDEX IF NOT EXISTS idx_archive_collections_public_category_listing
  ON archive.collections (status, visibility, category_id, id DESC);

CREATE INDEX IF NOT EXISTS idx_archive_emojis_collection_status_order_id
  ON archive.emojis (collection_id, status, display_order, id);

CREATE INDEX IF NOT EXISTS idx_taxonomy_collection_tags_tag_collection
  ON taxonomy.collection_tags (tag_id, collection_id);

CREATE INDEX IF NOT EXISTS idx_action_favorites_emoji
  ON action.favorites (emoji_id);

CREATE INDEX IF NOT EXISTS idx_action_likes_emoji
  ON action.likes (emoji_id);

CREATE INDEX IF NOT EXISTS idx_action_downloads_emoji
  ON action.downloads (emoji_id);

COMMIT;
