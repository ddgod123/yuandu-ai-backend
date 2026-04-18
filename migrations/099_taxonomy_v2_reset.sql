BEGIN;

-- 安全门禁：仅允许在“合集主数据为空”时执行（避免误覆盖线上已有内容）
DO $$
DECLARE
  v_collection_count BIGINT;
BEGIN
  SELECT COUNT(*) INTO v_collection_count
  FROM archive.collections
  WHERE deleted_at IS NULL;

  IF v_collection_count > 0 THEN
    RAISE EXCEPTION 'migration 099 requires archive.collections empty, found=%', v_collection_count;
  END IF;
END $$;

DO $$
DECLARE
  v_root TEXT;
BEGIN
  -- 动态推导当前环境对象根前缀（dev/prod 共用同一 SQL）
  SELECT COALESCE(
    NULLIF(split_part((SELECT key_prefix FROM ops.asset_domain_storage_policies WHERE domain = 'ugc' ORDER BY id LIMIT 1), '/', 1), ''),
    NULLIF(split_part((SELECT key_prefix FROM ops.asset_domain_storage_policies ORDER BY id LIMIT 1), '/', 1), ''),
    NULLIF(split_part((SELECT prefix FROM taxonomy.categories ORDER BY id LIMIT 1), '/', 1), ''),
    'emoji'
  ) INTO v_root;

  -- 1) 清理旧分类/标签体系（当前业务主数据已清空，可安全重建）
  DELETE FROM taxonomy.collection_tags;
  DELETE FROM taxonomy.emoji_tags;
  DELETE FROM taxonomy.collection_auto_tags;
  DELETE FROM taxonomy.emoji_auto_tags;
  DELETE FROM taxonomy.ip_collection_bindings;
  DELETE FROM taxonomy.ips;

  DELETE FROM taxonomy.categories;
  DELETE FROM taxonomy.tags;
  DELETE FROM taxonomy.tag_groups;

  -- 2) 重建分类（一级）
  INSERT INTO taxonomy.categories(name, slug, parent_id, prefix, description, sort, status, created_at, updated_at)
  VALUES
    ('情绪反应', 'reactions', NULL, format('%s/collections/reactions/', v_root), '聊天常用情绪反馈与互动反应', 10, 'active', NOW(), NOW()),
    ('问候祝福', 'greetings', NULL, format('%s/collections/greetings/', v_root), '问候、致谢、祝福等礼貌表达', 20, 'active', NOW(), NOW()),
    ('梗图鬼畜', 'memes', NULL, format('%s/collections/memes/', v_root), '热梗、鬼畜、整活类内容', 30, 'active', NOW(), NOW()),
    ('动漫二次元', 'anime', NULL, format('%s/collections/anime/', v_root), '日漫、国漫、美漫及超级英雄', 40, 'active', NOW(), NOW()),
    ('明星人物', 'celebrity', NULL, format('%s/collections/celebrity/', v_root), '明星、公众人物、角色脸谱', 50, 'active', NOW(), NOW()),
    ('萌宠动物', 'animals', NULL, format('%s/collections/animals/', v_root), '猫狗兔鸭熊猫等动物主题', 60, 'active', NOW(), NOW()),
    ('游戏电竞', 'gaming', NULL, format('%s/collections/gaming/', v_root), '手游/端游/主机/电竞主题', 70, 'active', NOW(), NOW()),
    ('体育运动', 'sports', NULL, format('%s/collections/sports/', v_root), '足球、篮球、格斗、球拍类运动', 80, 'active', NOW(), NOW()),
    ('日常生活', 'lifestyle', NULL, format('%s/collections/lifestyle/', v_root), '工作学习、吃喝睡、通勤社交', 90, 'active', NOW(), NOW()),
    ('情侣恋爱', 'love', NULL, format('%s/collections/love/', v_root), '情侣互动与情感表达', 100, 'active', NOW(), NOW()),
    ('符号表情', 'emoji', NULL, format('%s/collections/emoji/', v_root), '黄脸、符号、物品类表情', 110, 'active', NOW(), NOW()),
    ('节日热点', 'festival', NULL, format('%s/collections/festival/', v_root), '春节、情人节、圣诞等节日主题', 120, 'active', NOW(), NOW());

  -- 3) 重建分类（二级）
  INSERT INTO taxonomy.categories(name, slug, parent_id, prefix, description, sort, status, created_at, updated_at)
  SELECT c.name,
         c.slug,
         p.id,
         format('%s/collections/%s/%s/', v_root, c.parent_slug, c.slug),
         c.description,
         c.sort,
         c.status,
         NOW(), NOW()
  FROM (
    VALUES
      -- reactions
      ('reactions','开心','happy','正向开心反馈',10,'active'),
      ('reactions','难过','sad','难过失落情绪',20,'active'),
      ('reactions','生气','angry','愤怒吐槽表达',30,'active'),
      ('reactions','惊讶','surprised','震惊惊讶表达',40,'active'),
      ('reactions','无语','speechless','无语尴尬表达',50,'active'),
      ('reactions','点赞','approval','认可点赞支持',60,'active'),

      -- greetings
      ('greetings','打招呼','hello','开场问候寒暄',10,'active'),
      ('greetings','早晚安','good-morning-night','早安晚安场景',20,'active'),
      ('greetings','感谢致歉','thanks-sorry','谢谢/对不起',30,'active'),
      ('greetings','祝福语','blessings','生日节日祝福',40,'active'),

      -- memes
      ('memes','熊猫头','panda-head','经典熊猫头梗图',10,'active'),
      ('memes','网络热梗','internet-hot','热点梗图与话题',20,'active'),
      ('memes','沙雕搞怪','weird-funny','无厘头整活类',30,'active'),

      -- anime
      ('anime','日漫','japanese-anime','日本动画漫画相关',10,'active'),
      ('anime','国漫','chinese-anime','中国动画漫画相关',20,'active'),
      ('anime','美漫','american-comics','欧美漫画角色相关',30,'active'),
      ('anime','超级英雄','superhero','漫威/DC 等英雄',40,'active'),

      -- celebrity
      ('celebrity','内娱','c-ent','华语娱乐明星',10,'active'),
      ('celebrity','韩娱','k-ent','韩国娱乐明星',20,'active'),
      ('celebrity','欧美','western-stars','欧美娱乐明星',30,'active'),
      ('celebrity','公众人物','public-figures','政商媒体公众人物',40,'active'),

      -- animals
      ('animals','猫咪','cats','猫咪主题',10,'active'),
      ('animals','狗狗','dogs','狗狗主题',20,'active'),
      ('animals','兔子','rabbits','兔子主题',30,'active'),
      ('animals','鸭子','ducks','鸭子主题',40,'active'),
      ('animals','熊猫','pandas','熊猫主题',50,'active'),
      ('animals','其他动物','others','其余动物主题',60,'active'),

      -- gaming
      ('gaming','手游','mobile-games','移动端游戏',10,'active'),
      ('gaming','端游','pc-games','PC 游戏',20,'active'),
      ('gaming','主机游戏','console-games','主机平台游戏',30,'active'),
      ('gaming','电竞','esports','电竞赛事与选手',40,'active'),

      -- sports
      ('sports','足球','football','足球运动相关',10,'active'),
      ('sports','篮球','basketball','篮球运动相关',20,'active'),
      ('sports','格斗','combat-sports','拳击/搏击等',30,'active'),
      ('sports','球拍运动','racket-sports','羽毛球/网球/乒乓等',40,'active'),
      ('sports','健身','fitness','健身训练场景',50,'active'),

      -- lifestyle
      ('lifestyle','工作学习','work-study','工作学习表达',10,'active'),
      ('lifestyle','吃喝','eat-drink','美食饮料场景',20,'active'),
      ('lifestyle','睡觉躺平','sleep','犯困休息场景',30,'active'),
      ('lifestyle','出行','travel','通勤旅行场景',40,'active'),
      ('lifestyle','社交','social','群聊社交互动',50,'active'),

      -- love
      ('love','情侣日常','couple-daily','情侣日常互动',10,'active'),
      ('love','撒娇','flirting','亲密撒娇表达',20,'active'),
      ('love','告白','confession','表白示爱场景',30,'active'),
      ('love','分手挽回','breakup','分手/复合表达',40,'active'),

      -- emoji
      ('emoji','黄脸','yellow-face','经典黄脸表情',10,'active'),
      ('emoji','符号','symbols','符号类元素',20,'active'),
      ('emoji','物品','objects','物品器具元素',30,'active'),
      ('emoji','文字梗','text-memes','文字内容表情',40,'active'),

      -- festival
      ('festival','春节','spring-festival','春节拜年主题',10,'active'),
      ('festival','情人节','valentine','情人节主题',20,'active'),
      ('festival','圣诞节','christmas','圣诞新年主题',30,'active'),
      ('festival','万圣节','halloween','万圣节主题',40,'active'),
      ('festival','生日','birthday','生日庆祝主题',50,'active')
  ) AS c(parent_slug, name, slug, description, sort, status)
  JOIN taxonomy.categories p
    ON p.parent_id IS NULL
   AND p.slug = c.parent_slug;

  -- 4) 重建标签组（用于运营检索）
  INSERT INTO taxonomy.tag_groups(name, slug, description, sort, status, created_at, updated_at)
  VALUES
    ('主体', 'subject', '主体对象（人物/动物/角色等）', 10, 'active', NOW(), NOW()),
    ('情绪', 'emotion', '情绪倾向标签', 20, 'active', NOW(), NOW()),
    ('动作', 'action', '动作行为标签', 30, 'active', NOW(), NOW()),
    ('风格', 'style', '视觉风格标签', 40, 'active', NOW(), NOW()),
    ('场景', 'scene', '使用场景标签', 50, 'active', NOW(), NOW()),
    ('题材', 'topic', '内容题材标签', 60, 'active', NOW(), NOW()),
    ('人群', 'audience', '目标人群标签', 70, 'active', NOW(), NOW()),
    ('版权', 'rights', '版权与合规风险标签', 80, 'active', NOW(), NOW());

  -- 5) 重建标签（按标签组归类）
  INSERT INTO taxonomy.tags(name, slug, tag_group_id, sort, status, created_at, updated_at)
  SELECT t.name, t.slug, g.id, t.sort, t.status, NOW(), NOW()
  FROM (
    VALUES
      -- subject
      ('subject','人物','person',10,'active'),
      ('subject','动物','animal',20,'active'),
      ('subject','动漫角色','anime-character',30,'active'),
      ('subject','名人','celebrity',40,'active'),
      ('subject','表情符号','emoji-symbol',50,'active'),
      ('subject','游戏角色','game-character',60,'active'),

      -- emotion
      ('emotion','开心','happy',10,'active'),
      ('emotion','难过','sad',20,'active'),
      ('emotion','生气','angry',30,'active'),
      ('emotion','惊讶','surprised',40,'active'),
      ('emotion','无语','speechless',50,'active'),
      ('emotion','害羞','shy',60,'active'),

      -- action
      ('action','点赞','thumbs-up',10,'active'),
      ('action','鼓掌','clap',20,'active'),
      ('action','比心','heart-hand',30,'active'),
      ('action','大笑','laugh',40,'active'),
      ('action','哭泣','cry',50,'active'),
      ('action','打招呼','wave',60,'active'),

      -- style
      ('style','可爱','cute',10,'active'),
      ('style','写实','realistic',20,'active'),
      ('style','手绘','hand-drawn',30,'active'),
      ('style','像素','pixel',40,'active'),
      ('style','简约','minimal',50,'active'),
      ('style','夸张','exaggerated',60,'active'),

      -- scene
      ('scene','聊天回复','chat-reply',10,'active'),
      ('scene','节日祝福','festival-greeting',20,'active'),
      ('scene','工作学习','work-study',30,'active'),
      ('scene','吃喝','eat-drink',40,'active'),
      ('scene','熬夜','night-owl',50,'active'),
      ('scene','斗图','meme-fight',60,'active'),

      -- topic
      ('topic','热梗','hot-meme',10,'active'),
      ('topic','熊猫头','panda-head',20,'active'),
      ('topic','二创','remix',30,'active'),
      ('topic','影视','tv-movie',40,'active'),
      ('topic','体育','sports',50,'active'),
      ('topic','游戏','gaming',60,'active'),

      -- audience
      ('audience','情侣','couple',10,'active'),
      ('audience','朋友','friends',20,'active'),
      ('audience','家人','family',30,'active'),
      ('audience','同事','colleagues',40,'active'),
      ('audience','二次元用户','acg-users',50,'active'),
      ('audience','电竞用户','esports-users',60,'active'),

      -- rights
      ('rights','原创可商用','original-commercial',10,'active'),
      ('rights','二创需授权','remix-license',20,'active'),
      ('rights','品牌风险','brand-risk',30,'active'),
      ('rights','肖像风险','portrait-risk',40,'active'),
      ('rights','来源待核验','source-unverified',50,'active'),
      ('rights','公有领域','public-domain',60,'active')
  ) AS t(group_slug, name, slug, sort, status)
  JOIN taxonomy.tag_groups g
    ON g.slug = t.group_slug;

  -- 6) 序列对齐
  PERFORM setval(pg_get_serial_sequence('taxonomy.categories', 'id'), COALESCE((SELECT MAX(id) FROM taxonomy.categories), 1), true);
  PERFORM setval(pg_get_serial_sequence('taxonomy.tag_groups', 'id'), COALESCE((SELECT MAX(id) FROM taxonomy.tag_groups), 1), true);
  PERFORM setval(pg_get_serial_sequence('taxonomy.tags', 'id'), COALESCE((SELECT MAX(id) FROM taxonomy.tags), 1), true);
  PERFORM setval(pg_get_serial_sequence('taxonomy.ips', 'id'), COALESCE((SELECT MAX(id) FROM taxonomy.ips), 1), true);
END $$;

COMMIT;
