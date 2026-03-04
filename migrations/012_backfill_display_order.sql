BEGIN;

-- 1) Try to parse sequence from /raw/0001.ext in file_url
WITH seqs AS (
  SELECT
    id,
    NULLIF(regexp_replace(file_url, '.*?/raw/([0-9]+)\.[^/]+$', '\1'), '') AS seq
  FROM archive.emojis
  WHERE (display_order IS NULL OR display_order = 0)
    AND file_url ~ '/raw/[0-9]+\.[^/]+$'
)
UPDATE archive.emojis e
SET display_order = seqs.seq::int
FROM seqs
WHERE e.id = seqs.id
  AND seqs.seq IS NOT NULL;

-- 2) Fallback: assign order by id within collection for remaining rows
WITH ranked AS (
  SELECT
    id,
    ROW_NUMBER() OVER (PARTITION BY collection_id ORDER BY id) AS rn
  FROM archive.emojis
  WHERE (display_order IS NULL OR display_order = 0)
)
UPDATE archive.emojis e
SET display_order = ranked.rn
FROM ranked
WHERE e.id = ranked.id
  AND (e.display_order IS NULL OR e.display_order = 0);

COMMIT;
