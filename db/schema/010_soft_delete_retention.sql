-- Migration 010: platform soft delete and per-kind retention settings.
-- Deleted actor identifiers are informational and intentionally have no foreign key.

ALTER TABLE identities ADD COLUMN deleted_at TIMESTAMP NULL;
ALTER TABLE identities ADD COLUMN deleted_by INTEGER NULL;

ALTER TABLE assets ADD COLUMN deleted_at TIMESTAMP NULL;
ALTER TABLE assets ADD COLUMN deleted_by INTEGER NULL;

ALTER TABLE groups ADD COLUMN deleted_at TIMESTAMP NULL;
ALTER TABLE groups ADD COLUMN deleted_by INTEGER NULL;

ALTER TABLE configuration_groups ADD COLUMN deleted_at TIMESTAMP NULL;
ALTER TABLE configuration_groups ADD COLUMN deleted_by INTEGER NULL;

ALTER TABLE dependency_groups ADD COLUMN deleted_at TIMESTAMP NULL;
ALTER TABLE dependency_groups ADD COLUMN deleted_by INTEGER NULL;

ALTER TABLE policy_modules ADD COLUMN deleted_at TIMESTAMP NULL;
ALTER TABLE policy_modules ADD COLUMN deleted_by INTEGER NULL;

CREATE TABLE retention_settings (
    entity_kind TEXT PRIMARY KEY,
    purge_after_days INTEGER NULL CHECK(purge_after_days IS NULL OR purge_after_days >= 0),
    mode TEXT NOT NULL DEFAULT 'soft' CHECK(mode IN ('soft', 'immediate')),
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by INTEGER NULL
);

INSERT INTO retention_settings (entity_kind) VALUES
    ('identity'),
    ('asset'),
    ('group'),
    ('configuration_group'),
    ('dependency_group'),
    ('policy_module');

CREATE INDEX idx_identities_deleted_at ON identities(deleted_at);
CREATE INDEX idx_assets_deleted_at ON assets(deleted_at);
CREATE INDEX idx_groups_deleted_at ON groups(deleted_at);
CREATE INDEX idx_configuration_groups_deleted_at ON configuration_groups(deleted_at);
CREATE INDEX idx_dependency_groups_deleted_at ON dependency_groups(deleted_at);
CREATE INDEX idx_policy_modules_deleted_at ON policy_modules(deleted_at);
