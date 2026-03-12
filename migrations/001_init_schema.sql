CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS db_version (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    version VARCHAR(32) NOT NULL DEFAULT '1.0',
    revision INTEGER NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username VARCHAR(64) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(20) NOT NULL DEFAULT 'user',
    is_permanent_ban BOOLEAN NOT NULL DEFAULT FALSE,
    banned_until TIMESTAMPTZ,
    ban_duration_days INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS entities (
    id VARCHAR(128) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(64) NOT NULL,
    deleted BOOLEAN NOT NULL DEFAULT FALSE,
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision >= 1),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_entities_type ON entities(type);
CREATE INDEX IF NOT EXISTS idx_entities_deleted ON entities(deleted);

CREATE TABLE IF NOT EXISTS template_revisions (
    id VARCHAR(128) PRIMARY KEY,
    entity_id VARCHAR(128) NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    revision INTEGER NOT NULL CHECK (revision >= 1),
    template TEXT NOT NULL,
    visualization TEXT NOT NULL DEFAULT '',
    author_username VARCHAR(64) NOT NULL,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (entity_id, revision)
);

CREATE INDEX IF NOT EXISTS idx_template_revisions_entity ON template_revisions(entity_id);
CREATE INDEX IF NOT EXISTS idx_template_revisions_revision ON template_revisions(entity_id, revision DESC);

INSERT INTO db_version (id, version, revision, updated_at)
VALUES (1, '1.0', 1, NOW())
ON CONFLICT (id) DO NOTHING;
