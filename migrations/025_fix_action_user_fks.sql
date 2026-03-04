BEGIN;

ALTER TABLE action.favorites
  DROP CONSTRAINT IF EXISTS favorites_user_id_fkey;
ALTER TABLE action.favorites
  ADD CONSTRAINT favorites_user_id_fkey
  FOREIGN KEY (user_id) REFERENCES "user".users(id) ON DELETE CASCADE;

ALTER TABLE action.likes
  DROP CONSTRAINT IF EXISTS likes_user_id_fkey;
ALTER TABLE action.likes
  ADD CONSTRAINT likes_user_id_fkey
  FOREIGN KEY (user_id) REFERENCES "user".users(id) ON DELETE CASCADE;

ALTER TABLE action.downloads
  DROP CONSTRAINT IF EXISTS downloads_user_id_fkey;
ALTER TABLE action.downloads
  ADD CONSTRAINT downloads_user_id_fkey
  FOREIGN KEY (user_id) REFERENCES "user".users(id) ON DELETE SET NULL;

COMMIT;

