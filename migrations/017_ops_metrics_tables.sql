BEGIN;

CREATE TABLE IF NOT EXISTS audit.category_daily_stats (
  id BIGSERIAL PRIMARY KEY,
  category_id BIGINT NOT NULL,
  stat_date DATE NOT NULL,
  download_count INTEGER NOT NULL DEFAULT 0,
  search_count INTEGER NOT NULL DEFAULT 0,
  view_count INTEGER NOT NULL DEFAULT 0,
  heat_score NUMERIC(12,2) NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_audit_category_daily_stats
  ON audit.category_daily_stats (category_id, stat_date);

CREATE INDEX IF NOT EXISTS idx_audit_category_daily_stats_date
  ON audit.category_daily_stats (stat_date);

COMMENT ON TABLE audit.category_daily_stats IS '分类日度统计（下载/搜索/浏览/热度）';
COMMENT ON COLUMN audit.category_daily_stats.category_id IS '分类ID';
COMMENT ON COLUMN audit.category_daily_stats.stat_date IS '统计日期';
COMMENT ON COLUMN audit.category_daily_stats.download_count IS '下载量';
COMMENT ON COLUMN audit.category_daily_stats.search_count IS '搜索量';
COMMENT ON COLUMN audit.category_daily_stats.view_count IS '浏览量';
COMMENT ON COLUMN audit.category_daily_stats.heat_score IS '热度得分';

CREATE TABLE IF NOT EXISTS audit.search_term_daily_stats (
  id BIGSERIAL PRIMARY KEY,
  term VARCHAR(255) NOT NULL,
  normalized_term VARCHAR(255) NOT NULL,
  stat_date DATE NOT NULL,
  search_count INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_audit_search_term_daily_stats
  ON audit.search_term_daily_stats (normalized_term, stat_date);

CREATE INDEX IF NOT EXISTS idx_audit_search_term_daily_stats_date
  ON audit.search_term_daily_stats (stat_date);

COMMENT ON TABLE audit.search_term_daily_stats IS '搜索词日度统计';
COMMENT ON COLUMN audit.search_term_daily_stats.term IS '搜索词';
COMMENT ON COLUMN audit.search_term_daily_stats.normalized_term IS '标准化搜索词';
COMMENT ON COLUMN audit.search_term_daily_stats.stat_date IS '统计日期';
COMMENT ON COLUMN audit.search_term_daily_stats.search_count IS '搜索次数';

COMMIT;
