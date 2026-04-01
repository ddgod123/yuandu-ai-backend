\set ON_ERROR_STOP on

\if :{?old_prefix}
\else
\set old_prefix emoji/
\endif

\if :{?new_prefix}
\else
\set new_prefix emoji-prod/
\endif

\if :{?old_domain}
\else
\set old_domain ''
\endif

\if :{?new_domain}
\else
\set new_domain ''
\endif

\if :{?apply}
\else
\set apply 0
\endif

\echo [migrate_qiniu_root_prefix] old_prefix=:old_prefix new_prefix=:new_prefix apply=:apply
\echo [migrate_qiniu_root_prefix] old_domain=:old_domain new_domain=:new_domain

DROP TABLE IF EXISTS _qiniu_prefix_params;
CREATE TEMP TABLE _qiniu_prefix_params (
  old_prefix text NOT NULL,
  new_prefix text NOT NULL,
  old_domain text NOT NULL,
  new_domain text NOT NULL
);
INSERT INTO _qiniu_prefix_params(old_prefix, new_prefix, old_domain, new_domain)
VALUES (:'old_prefix', :'new_prefix', :'old_domain', :'new_domain');

DROP TABLE IF EXISTS _qiniu_prefix_targets;
CREATE TEMP TABLE _qiniu_prefix_targets (
  table_name  text NOT NULL,
  column_name text NOT NULL,
  PRIMARY KEY (table_name, column_name)
);

INSERT INTO _qiniu_prefix_targets(table_name, column_name) VALUES
  ('archive.collections', 'qiniu_prefix'),
  ('archive.collections', 'latest_zip_key'),
  ('archive.collections', 'cover_url'),
  ('archive.emojis', 'file_url'),
  ('archive.emojis', 'thumb_url'),
  ('archive.collection_zips', 'zip_key'),
  ('archive.video_jobs', 'source_video_key'),
  ('archive.video_job_artifacts', 'qiniu_key'),
  ('taxonomy.categories', 'prefix'),
  ('taxonomy.categories', 'cover_url'),
  ('taxonomy.ips', 'cover_url'),
  ('video_asset.collections', 'qiniu_prefix'),
  ('video_asset.collections', 'latest_zip_key'),
  ('video_asset.collections', 'cover_url'),
  ('video_asset.emojis', 'file_url'),
  ('video_asset.emojis', 'thumb_url'),
  ('video_asset.collection_zips', 'zip_key'),
  ('public.video_image_jobs', 'source_video_key'),
  ('public.video_image_outputs', 'object_key'),
  ('public.video_image_packages', 'zip_object_key');

INSERT INTO _qiniu_prefix_targets(table_name, column_name)
SELECT table_schema || '.' || table_name, 'source_video_key'
FROM information_schema.columns
WHERE table_schema = 'public'
  AND table_name LIKE 'video_image_jobs\_%'
  AND column_name = 'source_video_key'
ON CONFLICT DO NOTHING;

INSERT INTO _qiniu_prefix_targets(table_name, column_name)
SELECT table_schema || '.' || table_name, 'object_key'
FROM information_schema.columns
WHERE table_schema = 'public'
  AND table_name LIKE 'video_image_outputs\_%'
  AND column_name = 'object_key'
ON CONFLICT DO NOTHING;

INSERT INTO _qiniu_prefix_targets(table_name, column_name)
SELECT table_schema || '.' || table_name, 'zip_object_key'
FROM information_schema.columns
WHERE table_schema = 'public'
  AND table_name LIKE 'video_image_packages\_%'
  AND column_name = 'zip_object_key'
ON CONFLICT DO NOTHING;

DROP TABLE IF EXISTS _qiniu_prefix_summary;
CREATE TEMP TABLE _qiniu_prefix_summary (
  table_name     text NOT NULL,
  column_name    text NOT NULL,
  old_key_rows   bigint NOT NULL DEFAULT 0,
  old_url_rows   bigint NOT NULL DEFAULT 0,
  total_rows     bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (table_name, column_name)
);

DO $$
DECLARE
  r             record;
  key_rows      bigint;
  url_rows      bigint;
  total_rows    bigint;
  count_sql     text;
  old_prefix    text;
  old_prefix_no text;
BEGIN
  SELECT p.old_prefix, trim(both '/' from p.old_prefix)
  INTO old_prefix, old_prefix_no
  FROM _qiniu_prefix_params p
  LIMIT 1;

  FOR r IN
    SELECT table_name, column_name
    FROM _qiniu_prefix_targets
    ORDER BY table_name, column_name
  LOOP
    IF to_regclass(r.table_name) IS NULL THEN
      CONTINUE;
    END IF;

    count_sql := format(
      'SELECT
         COUNT(*) FILTER (WHERE COALESCE(%1$I, '''') LIKE %2$L),
         COUNT(*) FILTER (WHERE COALESCE(%1$I, '''') LIKE ''http%%'' AND COALESCE(%1$I, '''') LIKE %3$L),
         COUNT(*)
       FROM %4$s',
      r.column_name,
      old_prefix || '%',
      '%/' || old_prefix_no || '/%',
      r.table_name
    );

    EXECUTE count_sql INTO key_rows, url_rows, total_rows;
    INSERT INTO _qiniu_prefix_summary(table_name, column_name, old_key_rows, old_url_rows, total_rows)
    VALUES (r.table_name, r.column_name, COALESCE(key_rows, 0), COALESCE(url_rows, 0), COALESCE(total_rows, 0))
    ON CONFLICT (table_name, column_name) DO UPDATE
    SET old_key_rows = EXCLUDED.old_key_rows,
        old_url_rows = EXCLUDED.old_url_rows,
        total_rows = EXCLUDED.total_rows;
  END LOOP;
END $$;

\echo [migrate_qiniu_root_prefix] preview counts (before)
SELECT table_name, column_name, old_key_rows, old_url_rows, total_rows
FROM _qiniu_prefix_summary
WHERE old_key_rows > 0 OR old_url_rows > 0
ORDER BY old_key_rows DESC, old_url_rows DESC, table_name, column_name;

\if :apply
\echo [migrate_qiniu_root_prefix] APPLY MODE ENABLED
BEGIN;
DO $$
DECLARE
  r                record;
  key_updated      bigint;
  url_updated      bigint;
  old_prefix       text;
  new_prefix       text;
  old_prefix_no    text;
  new_prefix_no    text;
  old_domain       text;
  new_domain       text;
  update_sql       text;
BEGIN
  SELECT p.old_prefix,
         p.new_prefix,
         trim(both '/' from p.old_prefix),
         trim(both '/' from p.new_prefix),
         btrim(p.old_domain),
         btrim(p.new_domain)
  INTO old_prefix, new_prefix, old_prefix_no, new_prefix_no, old_domain, new_domain
  FROM _qiniu_prefix_params p
  LIMIT 1;

  IF old_prefix = '' OR new_prefix = '' THEN
    RAISE EXCEPTION 'old_prefix/new_prefix cannot be empty';
  END IF;
  IF right(old_prefix, 1) <> '/' OR right(new_prefix, 1) <> '/' THEN
    RAISE EXCEPTION 'old_prefix/new_prefix must end with "/"';
  END IF;

  FOR r IN
    SELECT table_name, column_name
    FROM _qiniu_prefix_targets
    ORDER BY table_name, column_name
  LOOP
    IF to_regclass(r.table_name) IS NULL THEN
      CONTINUE;
    END IF;

    update_sql := format(
      'UPDATE %1$s
       SET %2$I = %3$L || SUBSTRING(%2$I FROM %4$s)
       WHERE COALESCE(%2$I, '''') LIKE %5$L',
      r.table_name,
      r.column_name,
      new_prefix,
      length(old_prefix) + 1,
      old_prefix || '%'
    );
    EXECUTE update_sql;
    GET DIAGNOSTICS key_updated = ROW_COUNT;

    url_updated := 0;
    IF old_domain <> '' AND new_domain <> '' THEN
      update_sql := format(
        'UPDATE %1$s
         SET %2$I = REPLACE(REPLACE(%2$I, %3$L, %4$L), %5$L, %6$L)
         WHERE COALESCE(%2$I, '''') LIKE ''http%%''
           AND COALESCE(%2$I, '''') LIKE %7$L
           AND COALESCE(%2$I, '''') LIKE %8$L',
        r.table_name,
        r.column_name,
        old_domain,
        new_domain,
        '/' || old_prefix_no || '/',
        '/' || new_prefix_no || '/',
        '%' || old_domain || '%',
        '%/' || old_prefix_no || '/%'
      );
      EXECUTE update_sql;
      GET DIAGNOSTICS url_updated = ROW_COUNT;
    END IF;

    RAISE NOTICE '[%] key_updated=% url_updated=%',
      r.table_name || '.' || r.column_name, key_updated, url_updated;
  END LOOP;
END $$;
COMMIT;
\else
\echo [migrate_qiniu_root_prefix] DRY RUN ONLY (no changes)
\endif

TRUNCATE _qiniu_prefix_summary;
DO $$
DECLARE
  r             record;
  key_rows      bigint;
  url_rows      bigint;
  total_rows    bigint;
  count_sql     text;
  old_prefix    text;
  old_prefix_no text;
BEGIN
  SELECT p.old_prefix, trim(both '/' from p.old_prefix)
  INTO old_prefix, old_prefix_no
  FROM _qiniu_prefix_params p
  LIMIT 1;

  FOR r IN
    SELECT table_name, column_name
    FROM _qiniu_prefix_targets
    ORDER BY table_name, column_name
  LOOP
    IF to_regclass(r.table_name) IS NULL THEN
      CONTINUE;
    END IF;

    count_sql := format(
      'SELECT
         COUNT(*) FILTER (WHERE COALESCE(%1$I, '''') LIKE %2$L),
         COUNT(*) FILTER (WHERE COALESCE(%1$I, '''') LIKE ''http%%'' AND COALESCE(%1$I, '''') LIKE %3$L),
         COUNT(*)
       FROM %4$s',
      r.column_name,
      old_prefix || '%',
      '%/' || old_prefix_no || '/%',
      r.table_name
    );

    EXECUTE count_sql INTO key_rows, url_rows, total_rows;
    INSERT INTO _qiniu_prefix_summary(table_name, column_name, old_key_rows, old_url_rows, total_rows)
    VALUES (r.table_name, r.column_name, COALESCE(key_rows, 0), COALESCE(url_rows, 0), COALESCE(total_rows, 0))
    ON CONFLICT (table_name, column_name) DO UPDATE
    SET old_key_rows = EXCLUDED.old_key_rows,
        old_url_rows = EXCLUDED.old_url_rows,
        total_rows = EXCLUDED.total_rows;
  END LOOP;
END $$;

\echo [migrate_qiniu_root_prefix] remaining old-prefix rows (after)
SELECT table_name, column_name, old_key_rows, old_url_rows, total_rows
FROM _qiniu_prefix_summary
WHERE old_key_rows > 0 OR old_url_rows > 0
ORDER BY old_key_rows DESC, old_url_rows DESC, table_name, column_name;
