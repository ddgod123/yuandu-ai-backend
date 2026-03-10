BEGIN;

CREATE TABLE IF NOT EXISTS ops.risk_blacklists (
    id BIGSERIAL PRIMARY KEY,
    scope VARCHAR(16) NOT NULL,
    target VARCHAR(191) NOT NULL,
    action VARCHAR(32) NOT NULL DEFAULT 'all',
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    reason TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NULL,
    created_by BIGINT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_risk_blacklists_scope_target
  ON ops.risk_blacklists(scope, target);
CREATE INDEX IF NOT EXISTS idx_risk_blacklists_action_status
  ON ops.risk_blacklists(action, status);
CREATE INDEX IF NOT EXISTS idx_risk_blacklists_expires
  ON ops.risk_blacklists(expires_at);
CREATE INDEX IF NOT EXISTS idx_risk_blacklists_deleted_at
  ON ops.risk_blacklists(deleted_at);

CREATE TABLE IF NOT EXISTS ops.risk_events (
    id BIGSERIAL PRIMARY KEY,
    event_type VARCHAR(64) NOT NULL,
    action VARCHAR(32) NOT NULL DEFAULT '',
    scope VARCHAR(16) NOT NULL DEFAULT '',
    target VARCHAR(191) NOT NULL DEFAULT '',
    severity VARCHAR(16) NOT NULL DEFAULT 'info',
    message TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_risk_events_created_at
  ON ops.risk_events(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_risk_events_event_action
  ON ops.risk_events(event_type, action);
CREATE INDEX IF NOT EXISTS idx_risk_events_target
  ON ops.risk_events(target);
CREATE INDEX IF NOT EXISTS idx_risk_events_severity
  ON ops.risk_events(severity);

COMMENT ON TABLE ops.risk_blacklists IS '风控黑名单（按IP/设备/用户/手机号进行封禁）';
COMMENT ON TABLE ops.risk_events IS '风控事件日志（限流、黑名单命中、ticket异常等）';

COMMIT;
