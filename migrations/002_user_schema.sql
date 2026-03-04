BEGIN;

CREATE SCHEMA IF NOT EXISTS "user";

CREATE TABLE IF NOT EXISTS "user".users (
    id BIGSERIAL PRIMARY KEY,
    email VARCHAR(255) UNIQUE,
    phone VARCHAR(32) UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    display_name VARCHAR(64),
    avatar_url VARCHAR(512),
    role VARCHAR(32) DEFAULT 'user',
    status VARCHAR(32) DEFAULT 'active',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS "user".admin_roles (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES "user".users(id) ON DELETE CASCADE,
    role VARCHAR(32) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (user_id, role)
);

CREATE TABLE IF NOT EXISTS "user".refresh_tokens (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES "user".users(id) ON DELETE CASCADE,
    token_hash VARCHAR(64) NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_users_role ON "user".users(role);
CREATE INDEX IF NOT EXISTS idx_user_users_status ON "user".users(status);
CREATE INDEX IF NOT EXISTS idx_user_refresh_tokens_user ON "user".refresh_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_user_refresh_tokens_expires ON "user".refresh_tokens(expires_at);

INSERT INTO "user".users (email, phone, password_hash, display_name, role, status)
VALUES ('admin@emoji.local', '18800000000', '$2a$10$9k8NwkOU2mgTZmxdl2iv8uc8iu6QN6zHGsYVZZwQDQKEjpvkN94.O', 'Super Admin', 'super_admin', 'active')
ON CONFLICT (phone) DO NOTHING;

INSERT INTO "user".admin_roles (user_id, role)
SELECT id, 'super_admin' FROM "user".users WHERE phone = '18800000000'
ON CONFLICT DO NOTHING;

COMMIT;
