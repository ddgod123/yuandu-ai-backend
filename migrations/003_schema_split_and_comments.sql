BEGIN;

CREATE SCHEMA IF NOT EXISTS archive;
CREATE SCHEMA IF NOT EXISTS taxonomy;
CREATE SCHEMA IF NOT EXISTS action;
CREATE SCHEMA IF NOT EXISTS audit;
CREATE SCHEMA IF NOT EXISTS ops;
CREATE SCHEMA IF NOT EXISTS system;

COMMENT ON SCHEMA archive IS '表情包档案库相关表';
COMMENT ON SCHEMA taxonomy IS '主题/分类/标签体系相关表';
COMMENT ON SCHEMA action IS '用户行为与互动相关表';
COMMENT ON SCHEMA audit IS '审核与合规相关表';
COMMENT ON SCHEMA ops IS '运营与推荐相关表';
COMMENT ON SCHEMA system IS '系统与配置相关表';

ALTER TABLE IF EXISTS public.collections SET SCHEMA archive;
ALTER TABLE IF EXISTS public.emojis SET SCHEMA archive;

ALTER TABLE IF EXISTS public.tags SET SCHEMA taxonomy;
ALTER TABLE IF EXISTS public.emoji_tags SET SCHEMA taxonomy;
ALTER TABLE IF EXISTS public.collection_tags SET SCHEMA taxonomy;

ALTER TABLE IF EXISTS public.favorites SET SCHEMA action;
ALTER TABLE IF EXISTS public.likes SET SCHEMA action;
ALTER TABLE IF EXISTS public.downloads SET SCHEMA action;

ALTER TABLE IF EXISTS public.reports SET SCHEMA audit;
ALTER TABLE IF EXISTS public.audit_logs SET SCHEMA audit;

COMMENT ON TABLE archive.collections IS '表情包合集';
COMMENT ON COLUMN archive.collections.id IS '主键';
COMMENT ON COLUMN archive.collections.title IS '合集标题';
COMMENT ON COLUMN archive.collections.description IS '合集描述';
COMMENT ON COLUMN archive.collections.cover_url IS '封面地址';
COMMENT ON COLUMN archive.collections.owner_id IS '创建者ID';
COMMENT ON COLUMN archive.collections.visibility IS '可见性';
COMMENT ON COLUMN archive.collections.status IS '状态';
COMMENT ON COLUMN archive.collections.created_at IS '创建时间';
COMMENT ON COLUMN archive.collections.updated_at IS '更新时间';
COMMENT ON COLUMN archive.collections.deleted_at IS '删除时间';

COMMENT ON TABLE archive.emojis IS '单个表情素材';
COMMENT ON COLUMN archive.emojis.id IS '主键';
COMMENT ON COLUMN archive.emojis.collection_id IS '所属合集ID';
COMMENT ON COLUMN archive.emojis.title IS '表情标题';
COMMENT ON COLUMN archive.emojis.file_url IS '原始文件地址/Key';
COMMENT ON COLUMN archive.emojis.thumb_url IS '缩略图地址';
COMMENT ON COLUMN archive.emojis.format IS '文件格式/类型';
COMMENT ON COLUMN archive.emojis.width IS '宽度';
COMMENT ON COLUMN archive.emojis.height IS '高度';
COMMENT ON COLUMN archive.emojis.size_bytes IS '文件大小（字节）';
COMMENT ON COLUMN archive.emojis.status IS '状态';
COMMENT ON COLUMN archive.emojis.created_at IS '创建时间';
COMMENT ON COLUMN archive.emojis.updated_at IS '更新时间';
COMMENT ON COLUMN archive.emojis.deleted_at IS '删除时间';

COMMENT ON TABLE taxonomy.tags IS '标签';
COMMENT ON COLUMN taxonomy.tags.id IS '主键';
COMMENT ON COLUMN taxonomy.tags.name IS '标签名称';
COMMENT ON COLUMN taxonomy.tags.slug IS '标签标识';
COMMENT ON COLUMN taxonomy.tags.created_at IS '创建时间';
COMMENT ON COLUMN taxonomy.tags.updated_at IS '更新时间';

COMMENT ON TABLE taxonomy.emoji_tags IS '表情-标签关联';
COMMENT ON COLUMN taxonomy.emoji_tags.emoji_id IS '表情ID';
COMMENT ON COLUMN taxonomy.emoji_tags.tag_id IS '标签ID';

COMMENT ON TABLE taxonomy.collection_tags IS '合集-标签关联';
COMMENT ON COLUMN taxonomy.collection_tags.collection_id IS '合集ID';
COMMENT ON COLUMN taxonomy.collection_tags.tag_id IS '标签ID';

COMMENT ON TABLE action.favorites IS '收藏记录';
COMMENT ON COLUMN action.favorites.user_id IS '用户ID';
COMMENT ON COLUMN action.favorites.emoji_id IS '表情ID';
COMMENT ON COLUMN action.favorites.created_at IS '创建时间';

COMMENT ON TABLE action.likes IS '点赞记录';
COMMENT ON COLUMN action.likes.user_id IS '用户ID';
COMMENT ON COLUMN action.likes.emoji_id IS '表情ID';
COMMENT ON COLUMN action.likes.created_at IS '创建时间';

COMMENT ON TABLE action.downloads IS '下载记录';
COMMENT ON COLUMN action.downloads.id IS '主键';
COMMENT ON COLUMN action.downloads.user_id IS '用户ID';
COMMENT ON COLUMN action.downloads.emoji_id IS '表情ID';
COMMENT ON COLUMN action.downloads.ip IS '下载IP';
COMMENT ON COLUMN action.downloads.created_at IS '创建时间';

COMMENT ON TABLE audit.reports IS '举报记录';
COMMENT ON COLUMN audit.reports.id IS '主键';
COMMENT ON COLUMN audit.reports.user_id IS '举报用户ID';
COMMENT ON COLUMN audit.reports.emoji_id IS '被举报表情ID';
COMMENT ON COLUMN audit.reports.reason IS '举报原因';
COMMENT ON COLUMN audit.reports.status IS '处理状态';
COMMENT ON COLUMN audit.reports.created_at IS '创建时间';
COMMENT ON COLUMN audit.reports.updated_at IS '更新时间';

COMMENT ON TABLE audit.audit_logs IS '审计日志';
COMMENT ON COLUMN audit.audit_logs.id IS '主键';
COMMENT ON COLUMN audit.audit_logs.admin_id IS '管理员ID';
COMMENT ON COLUMN audit.audit_logs.target_type IS '对象类型';
COMMENT ON COLUMN audit.audit_logs.target_id IS '对象ID';
COMMENT ON COLUMN audit.audit_logs.action IS '操作动作';
COMMENT ON COLUMN audit.audit_logs.meta IS '元数据';
COMMENT ON COLUMN audit.audit_logs.created_at IS '创建时间';

COMMENT ON TABLE "user".users IS '系统用户';
COMMENT ON COLUMN "user".users.id IS '主键';
COMMENT ON COLUMN "user".users.email IS '邮箱';
COMMENT ON COLUMN "user".users.phone IS '手机号';
COMMENT ON COLUMN "user".users.password_hash IS '密码哈希';
COMMENT ON COLUMN "user".users.display_name IS '显示名称';
COMMENT ON COLUMN "user".users.avatar_url IS '头像地址';
COMMENT ON COLUMN "user".users.role IS '角色';
COMMENT ON COLUMN "user".users.status IS '状态';
COMMENT ON COLUMN "user".users.created_at IS '创建时间';
COMMENT ON COLUMN "user".users.updated_at IS '更新时间';
COMMENT ON COLUMN "user".users.deleted_at IS '删除时间';

COMMENT ON TABLE "user".admin_roles IS '管理员角色表';
COMMENT ON COLUMN "user".admin_roles.id IS '主键';
COMMENT ON COLUMN "user".admin_roles.user_id IS '用户ID';
COMMENT ON COLUMN "user".admin_roles.role IS '角色';
COMMENT ON COLUMN "user".admin_roles.created_at IS '创建时间';
COMMENT ON COLUMN "user".admin_roles.updated_at IS '更新时间';

COMMENT ON TABLE "user".refresh_tokens IS '刷新令牌';
COMMENT ON COLUMN "user".refresh_tokens.id IS '主键';
COMMENT ON COLUMN "user".refresh_tokens.user_id IS '用户ID';
COMMENT ON COLUMN "user".refresh_tokens.token_hash IS '令牌哈希';
COMMENT ON COLUMN "user".refresh_tokens.expires_at IS '过期时间';
COMMENT ON COLUMN "user".refresh_tokens.revoked_at IS '撤销时间';
COMMENT ON COLUMN "user".refresh_tokens.created_at IS '创建时间';

COMMENT ON TABLE public.users IS '旧用户表（待迁移）';
COMMENT ON COLUMN public.users.id IS '主键';
COMMENT ON COLUMN public.users.email IS '邮箱';
COMMENT ON COLUMN public.users.phone IS '手机号';
COMMENT ON COLUMN public.users.password_hash IS '密码哈希';
COMMENT ON COLUMN public.users.display_name IS '显示名称';
COMMENT ON COLUMN public.users.avatar_url IS '头像地址';
COMMENT ON COLUMN public.users.role IS '角色';
COMMENT ON COLUMN public.users.status IS '状态';
COMMENT ON COLUMN public.users.created_at IS '创建时间';
COMMENT ON COLUMN public.users.updated_at IS '更新时间';
COMMENT ON COLUMN public.users.deleted_at IS '删除时间';

COMMIT;
