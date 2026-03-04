BEGIN;

ALTER TABLE IF EXISTS archive.collections
  ADD COLUMN IF NOT EXISTS download_code VARCHAR(16);

CREATE UNIQUE INDEX IF NOT EXISTS idx_archive_collections_download_code
  ON archive.collections(download_code);

-- Backfill missing codes (8 chars, uppercase base32 without I/L/O/0)
DO $$
DECLARE
  letters TEXT := 'ABCDEFGHJKMNPQRSTUVWXYZ23456789';
  new_code TEXT;
  rec RECORD;
BEGIN
  FOR rec IN
    SELECT id FROM archive.collections
    WHERE download_code IS NULL OR trim(download_code) = ''
  LOOP
    LOOP
      new_code := '';
      FOR i IN 1..8 LOOP
        new_code := new_code || substr(letters, floor(random() * length(letters))::int + 1, 1);
      END LOOP;
      EXIT WHEN NOT EXISTS (
        SELECT 1 FROM archive.collections WHERE download_code = new_code
      );
    END LOOP;

    UPDATE archive.collections
      SET download_code = new_code
      WHERE id = rec.id;
  END LOOP;
END $$;

COMMIT;
