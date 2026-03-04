BEGIN;

-- Upgrade users table (legacy public schema)
ALTER TABLE IF EXISTS public.users
  ADD COLUMN IF NOT EXISTS username VARCHAR(64),
  ADD COLUMN IF NOT EXISTS bio TEXT,
  ADD COLUMN IF NOT EXISTS website_url VARCHAR(512),
  ADD COLUMN IF NOT EXISTS location VARCHAR(64),
  ADD COLUMN IF NOT EXISTS is_designer BOOLEAN DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS verified_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS last_login_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS last_login_ip VARCHAR(64);

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username ON public.users(username);
CREATE INDEX IF NOT EXISTS idx_users_is_designer ON public.users(is_designer);

-- Upgrade users table (user schema)
ALTER TABLE IF EXISTS "user".users
  ADD COLUMN IF NOT EXISTS username VARCHAR(64),
  ADD COLUMN IF NOT EXISTS bio TEXT,
  ADD COLUMN IF NOT EXISTS website_url VARCHAR(512),
  ADD COLUMN IF NOT EXISTS location VARCHAR(64),
  ADD COLUMN IF NOT EXISTS is_designer BOOLEAN DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS verified_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS last_login_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS last_login_ip VARCHAR(64);

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_users_username ON "user".users(username);
CREATE INDEX IF NOT EXISTS idx_user_users_is_designer ON "user".users(is_designer);

-- Migrate legacy public.users into user.users (keep ids aligned)
DO $$
BEGIN
  IF to_regclass('public.users') IS NOT NULL THEN
    INSERT INTO "user".users (
      id, email, phone, password_hash, display_name, avatar_url, role, status,
      created_at, updated_at, deleted_at, username, bio, website_url, location,
      is_designer, verified_at, last_login_at, last_login_ip
    )
    SELECT
      id, email, phone, password_hash, display_name, avatar_url, role, status,
      created_at, updated_at, deleted_at, username, bio, website_url, location,
      is_designer, verified_at, last_login_at, last_login_ip
    FROM public.users
    ON CONFLICT DO NOTHING;

    PERFORM setval(
      pg_get_serial_sequence('"user".users','id'),
      GREATEST((SELECT COALESCE(MAX(id), 1) FROM "user".users), 1),
      true
    );
  END IF;
END $$;

-- Ensure collections have owner_id and index
ALTER TABLE IF EXISTS archive.collections
  ADD COLUMN IF NOT EXISTS owner_id BIGINT;

CREATE INDEX IF NOT EXISTS idx_archive_collections_owner ON archive.collections(owner_id);

-- Drop FK on archive.collections.owner_id that points to public.users
DO $$
DECLARE r record;
BEGIN
  FOR r IN
    SELECT c.conname
    FROM pg_constraint c
    JOIN pg_class t ON t.oid = c.conrelid
    JOIN pg_namespace n ON n.oid = t.relnamespace
    JOIN pg_class rt ON rt.oid = c.confrelid
    JOIN pg_namespace rn ON rn.oid = rt.relnamespace
    WHERE n.nspname = 'archive'
      AND t.relname = 'collections'
      AND rn.nspname = 'public'
      AND rt.relname = 'users'
      AND c.contype = 'f'
  LOOP
    EXECUTE format('ALTER TABLE archive.collections DROP CONSTRAINT IF EXISTS %I', r.conname);
  END LOOP;
END $$;

-- Add FK to user.users
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'fk_archive_collections_owner_user_schema'
  ) THEN
    ALTER TABLE archive.collections
      ADD CONSTRAINT fk_archive_collections_owner_user_schema
      FOREIGN KEY (owner_id) REFERENCES "user".users(id) ON DELETE RESTRICT;
  END IF;
END $$;

-- Allow multiple designers to maintain a collection
CREATE TABLE IF NOT EXISTS archive.collection_users (
  collection_id BIGINT NOT NULL REFERENCES archive.collections(id) ON DELETE CASCADE,
  user_id BIGINT NOT NULL,
  role VARCHAR(32) DEFAULT 'owner',
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW(),
  PRIMARY KEY (collection_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_archive_collection_users_user ON archive.collection_users(user_id);

-- Drop FK on collection_users.user_id that points to public.users (if any)
DO $$
DECLARE r record;
BEGIN
  FOR r IN
    SELECT c.conname
    FROM pg_constraint c
    JOIN pg_class t ON t.oid = c.conrelid
    JOIN pg_namespace n ON n.oid = t.relnamespace
    JOIN pg_class rt ON rt.oid = c.confrelid
    JOIN pg_namespace rn ON rn.oid = rt.relnamespace
    WHERE n.nspname = 'archive'
      AND t.relname = 'collection_users'
      AND rn.nspname = 'public'
      AND rt.relname = 'users'
      AND c.contype = 'f'
  LOOP
    EXECUTE format('ALTER TABLE archive.collection_users DROP CONSTRAINT IF EXISTS %I', r.conname);
  END LOOP;
END $$;

-- Ensure FK to user.users exists for collection_users
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint c
    JOIN pg_class t ON t.oid = c.conrelid
    JOIN pg_namespace n ON n.oid = t.relnamespace
    JOIN pg_class rt ON rt.oid = c.confrelid
    JOIN pg_namespace rn ON rn.oid = rt.relnamespace
    WHERE n.nspname = 'archive'
      AND t.relname = 'collection_users'
      AND rn.nspname = 'user'
      AND rt.relname = 'users'
      AND c.contype = 'f'
  ) THEN
    ALTER TABLE archive.collection_users
      ADD CONSTRAINT fk_archive_collection_users_user_schema
      FOREIGN KEY (user_id) REFERENCES "user".users(id) ON DELETE CASCADE;
  END IF;
END $$;

-- Backfill owners into collection_users
INSERT INTO archive.collection_users (collection_id, user_id, role)
SELECT id, owner_id, 'owner'
FROM archive.collections
WHERE owner_id IS NOT NULL
ON CONFLICT DO NOTHING;

COMMIT;
