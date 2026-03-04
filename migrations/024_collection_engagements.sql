CREATE TABLE IF NOT EXISTS action.collection_favorites (
  user_id BIGINT NOT NULL,
  collection_id BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, collection_id),
  CONSTRAINT collection_favorites_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES "user".users(id) ON DELETE CASCADE,
  CONSTRAINT collection_favorites_collection_id_fkey
    FOREIGN KEY (collection_id) REFERENCES archive.collections(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_collection_favorites_collection
  ON action.collection_favorites (collection_id);
CREATE INDEX IF NOT EXISTS idx_collection_favorites_created
  ON action.collection_favorites (created_at);

CREATE TABLE IF NOT EXISTS action.collection_likes (
  user_id BIGINT NOT NULL,
  collection_id BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, collection_id),
  CONSTRAINT collection_likes_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES "user".users(id) ON DELETE CASCADE,
  CONSTRAINT collection_likes_collection_id_fkey
    FOREIGN KEY (collection_id) REFERENCES archive.collections(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_collection_likes_collection
  ON action.collection_likes (collection_id);
CREATE INDEX IF NOT EXISTS idx_collection_likes_created
  ON action.collection_likes (created_at);
