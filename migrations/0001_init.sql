CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TYPE group_source AS ENUM ('platform', 'scim');
CREATE TYPE group_member_type AS ENUM ('user', 'agent', 'app');

CREATE TABLE groups (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    organization_id UUID NOT NULL,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    source          group_source NOT NULL,
    external_id     TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, source, name),
    UNIQUE (organization_id, source, external_id)
);

CREATE INDEX idx_groups_organization_id ON groups (organization_id);

CREATE TABLE group_memberships (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    group_id    UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    member_type group_member_type NOT NULL,
    member_id   UUID NOT NULL,
    source      group_source NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (group_id, member_id)
);

CREATE INDEX idx_group_memberships_group_type ON group_memberships (group_id, member_type);
CREATE INDEX idx_group_memberships_member ON group_memberships (member_type, member_id);
