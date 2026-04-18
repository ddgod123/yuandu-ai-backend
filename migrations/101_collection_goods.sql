-- 101_collection_goods.sql
-- 基于表情包合集的商品库（用于闲鱼/闲管家等对接场景）

BEGIN;

CREATE TABLE IF NOT EXISTS archive.collection_goods (
  id BIGSERIAL PRIMARY KEY,
  collection_id BIGINT NOT NULL,
  goods_no VARCHAR(128) NOT NULL,
  goods_type SMALLINT NOT NULL DEFAULT 2,
  goods_name VARCHAR(255) NOT NULL,
  price BIGINT NOT NULL DEFAULT 0,
  stock INTEGER NOT NULL DEFAULT 0,
  status SMALLINT NOT NULL DEFAULT 2,
  image_count INTEGER NOT NULL DEFAULT 6,
  image_start INTEGER NOT NULL DEFAULT 1,
  template_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  last_sync_at TIMESTAMPTZ NULL,
  last_sync_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'chk_collection_goods_goods_type'
  ) THEN
    ALTER TABLE archive.collection_goods
      ADD CONSTRAINT chk_collection_goods_goods_type CHECK (goods_type IN (1, 2, 3));
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'chk_collection_goods_status'
  ) THEN
    ALTER TABLE archive.collection_goods
      ADD CONSTRAINT chk_collection_goods_status CHECK (status IN (1, 2));
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'chk_collection_goods_price_non_negative'
  ) THEN
    ALTER TABLE archive.collection_goods
      ADD CONSTRAINT chk_collection_goods_price_non_negative CHECK (price >= 0);
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'chk_collection_goods_stock_non_negative'
  ) THEN
    ALTER TABLE archive.collection_goods
      ADD CONSTRAINT chk_collection_goods_stock_non_negative CHECK (stock >= 0);
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'chk_collection_goods_image_count_positive'
  ) THEN
    ALTER TABLE archive.collection_goods
      ADD CONSTRAINT chk_collection_goods_image_count_positive CHECK (image_count > 0);
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'chk_collection_goods_image_start_positive'
  ) THEN
    ALTER TABLE archive.collection_goods
      ADD CONSTRAINT chk_collection_goods_image_start_positive CHECK (image_start > 0);
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_collection_goods_collection_id
  ON archive.collection_goods(collection_id)
  WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_collection_goods_status
  ON archive.collection_goods(status)
  WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_collection_goods_goods_type
  ON archive.collection_goods(goods_type)
  WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_collection_goods_updated_at
  ON archive.collection_goods(updated_at DESC)
  WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_collection_goods_goods_no
  ON archive.collection_goods(goods_no)
  WHERE deleted_at IS NULL;

COMMENT ON TABLE archive.collection_goods IS '基于表情包合集的商品库';
COMMENT ON COLUMN archive.collection_goods.collection_id IS '关联 archive.collections.id';
COMMENT ON COLUMN archive.collection_goods.goods_no IS '商品编码（对外唯一）';
COMMENT ON COLUMN archive.collection_goods.goods_type IS '商品类型：1直充，2卡密，3券码';
COMMENT ON COLUMN archive.collection_goods.goods_name IS '商品名称';
COMMENT ON COLUMN archive.collection_goods.price IS '商品价格，单位分';
COMMENT ON COLUMN archive.collection_goods.stock IS '库存';
COMMENT ON COLUMN archive.collection_goods.status IS '商品状态：1在架，2下架';
COMMENT ON COLUMN archive.collection_goods.image_count IS '从合集顺序取图时的图片数量';
COMMENT ON COLUMN archive.collection_goods.image_start IS '从合集顺序取图时的起始位置（从1开始）';
COMMENT ON COLUMN archive.collection_goods.template_json IS '直充/卡密等业务模板字段';
COMMENT ON COLUMN archive.collection_goods.last_sync_at IS '最近同步到外部平台的时间';
COMMENT ON COLUMN archive.collection_goods.last_sync_error IS '最近同步错误信息';

COMMIT;

