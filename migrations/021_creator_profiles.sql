BEGIN;

CREATE SCHEMA IF NOT EXISTS ops;

CREATE TABLE IF NOT EXISTS ops.creator_profiles (
  id BIGSERIAL PRIMARY KEY,
  name_zh VARCHAR(64) NOT NULL,
  name_en VARCHAR(64) NOT NULL,
  avatar_url VARCHAR(512) NOT NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ops_creator_profiles_name_zh
  ON ops.creator_profiles (name_zh);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ops_creator_profiles_name_en
  ON ops.creator_profiles (name_en);

CREATE INDEX IF NOT EXISTS idx_ops_creator_profiles_status
  ON ops.creator_profiles (status);

ALTER TABLE archive.collections
  ADD COLUMN IF NOT EXISTS creator_profile_id BIGINT;

ALTER TABLE archive.collections
  ADD CONSTRAINT fk_collections_creator_profile
  FOREIGN KEY (creator_profile_id) REFERENCES ops.creator_profiles(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_collections_creator_profile_id
  ON archive.collections (creator_profile_id);

INSERT INTO ops.creator_profiles (id, name_zh, name_en, avatar_url, status)
VALUES
  (1, '星河旅人', 'Starlit Rover', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-01', 'active'),
  (2, '蓝莓汽水', 'Blueberry Soda', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-02', 'active'),
  (3, '山茶日记', 'Camellia Notes', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-03', 'active'),
  (4, '云上行者', 'Cloud Walker', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-04', 'active'),
  (5, '星光邮差', 'Starlight Postman', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-05', 'active'),
  (6, '森林漫游', 'Forest Roamer', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-06', 'active'),
  (7, '月影捕手', 'Moonshadow Catcher', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-07', 'active'),
  (8, '暖风岛民', 'Warm Breeze Islander', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-08', 'active'),
  (9, '柠檬航线', 'Lemon Route', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-09', 'active'),
  (10, '雨后纸鸢', 'After-Rain Kite', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-10', 'active'),
  (11, '电波小子', 'Radio Kid', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-11', 'active'),
  (12, '极光猫', 'Aurora Cat', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-12', 'active'),
  (13, '风铃先生', 'Windchime Mister', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-13', 'active'),
  (14, '琥珀星球', 'Amber Planet', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-14', 'active'),
  (15, '日落列车', 'Sunset Express', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-15', 'active'),
  (16, '迷雾灯塔', 'Misty Lighthouse', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-16', 'active'),
  (17, '星尘旅社', 'Stardust Inn', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-17', 'active'),
  (18, '沙丘探险', 'Dune Explorer', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-18', 'active'),
  (19, '银河观测', 'Galaxy Observer', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-19', 'active'),
  (20, '云朵信使', 'Cloud Messenger', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-20', 'active'),
  (21, '萤火邮局', 'Firefly Mail', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-21', 'active'),
  (22, '霓虹散步', 'Neon Stroll', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-22', 'active'),
  (23, '海风地图', 'Sea Breeze Map', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-23', 'active'),
  (24, '晨光笔记', 'Morning Note', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-24', 'active'),
  (25, '小行星驿站', 'Asteroid Station', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-25', 'active'),
  (26, '像素飞行员', 'Pixel Pilot', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-26', 'active'),
  (27, '量子电台', 'Quantum Radio', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-27', 'active'),
  (28, '气泡旅店', 'Bubble Inn', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-28', 'active'),
  (29, '星海旅票', 'Starsea Ticket', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-29', 'active'),
  (30, '时光速递', 'Time Courier', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-30', 'active'),
  (31, '冰川观鲸', 'Glacier Whale', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-31', 'active'),
  (32, '橘子飞船', 'Orange Shuttle', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-32', 'active'),
  (33, '星野露营', 'Starfield Camp', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-33', 'active'),
  (34, '潮汐唱片', 'Tidal Records', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-34', 'active'),
  (35, '黑糖拿铁', 'Brown Sugar Latte', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-35', 'active'),
  (36, '雪原小镇', 'Snowfield Town', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-36', 'active'),
  (37, '迷航指南', 'Lost Compass', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-37', 'active'),
  (38, '落日邮报', 'Sunset Bulletin', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-38', 'active'),
  (39, '星球采样', 'Planet Sampler', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-39', 'active'),
  (40, '雨林巡游', 'Rainforest Cruise', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-40', 'active'),
  (41, '霜白剧场', 'Frost Theater', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-41', 'active'),
  (42, '远方邮局', 'Faraway Post', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-42', 'active'),
  (43, '流光画室', 'Gleam Studio', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-43', 'active'),
  (44, '星桥旅馆', 'Starbridge Hotel', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-44', 'active'),
  (45, '纸飞机社', 'Paper Plane Club', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-45', 'active'),
  (46, '轻风旅队', 'Softwind Crew', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-46', 'active'),
  (47, '甜橙信号', 'Tangerine Signal', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-47', 'active'),
  (48, '蓝鲸航线', 'Blue Whale Route', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-48', 'active'),
  (49, '银河钟表', 'Galaxy Clock', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-49', 'active'),
  (50, '暮色照相馆', 'Dusk Photo Studio', 'https://api.dicebear.com/7.x/avataaars/svg?seed=creator-50', 'active')
ON CONFLICT (id) DO NOTHING;

SELECT setval(
  pg_get_serial_sequence('ops.creator_profiles', 'id'),
  COALESCE((SELECT MAX(id) FROM ops.creator_profiles), 1)
);

UPDATE archive.collections
SET creator_profile_id = ((id - 1) % 50) + 1
WHERE creator_profile_id IS NULL;

COMMIT;
