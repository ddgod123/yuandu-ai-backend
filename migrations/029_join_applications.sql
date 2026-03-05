BEGIN;

CREATE TABLE IF NOT EXISTS audit.join_applications (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(64) NOT NULL,
    phone VARCHAR(32) NOT NULL,
    gender VARCHAR(16) NOT NULL,
    age INTEGER NOT NULL,
    email VARCHAR(255) NOT NULL,
    occupation VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_join_applications_created_at
    ON audit.join_applications (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_join_applications_name
    ON audit.join_applications (name);
CREATE INDEX IF NOT EXISTS idx_join_applications_phone
    ON audit.join_applications (phone);
CREATE INDEX IF NOT EXISTS idx_join_applications_email
    ON audit.join_applications (email);

COMMENT ON TABLE audit.join_applications IS '加入档案馆申请信息';
COMMENT ON COLUMN audit.join_applications.name IS '申请人姓名';
COMMENT ON COLUMN audit.join_applications.phone IS '联系电话';
COMMENT ON COLUMN audit.join_applications.gender IS '性别';
COMMENT ON COLUMN audit.join_applications.age IS '年龄';
COMMENT ON COLUMN audit.join_applications.email IS '邮箱';
COMMENT ON COLUMN audit.join_applications.occupation IS '职业';
COMMENT ON COLUMN audit.join_applications.created_at IS '创建时间';
COMMENT ON COLUMN audit.join_applications.updated_at IS '更新时间';

COMMIT;
