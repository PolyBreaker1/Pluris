-- Migration 005: role hierarchy (parent_role_id) and group-role assignment.
-- Builtin roles never have a parent (enforced in service code, not the
-- schema). Custom roles may inherit from a builtin or custom role in the
-- same tenant; cycles and depth are rejected at write time. Contains no
-- PRAGMA, so it runs inside the tracker transaction.

ALTER TABLE roles ADD COLUMN parent_role_id INTEGER REFERENCES roles(id) ON DELETE SET NULL;

-- Assignment of roles to groups (many to many). Members of a group
-- inherit the group's roles in addition to any roles assigned directly.
CREATE TABLE IF NOT EXISTS group_roles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    group_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    role_id INTEGER NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    assigned_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    assigned_by INTEGER REFERENCES identities(id) ON DELETE SET NULL,
    UNIQUE(group_id, role_id)
);

CREATE INDEX IF NOT EXISTS idx_group_roles_group ON group_roles(group_id);
CREATE INDEX IF NOT EXISTS idx_group_roles_role ON group_roles(role_id);
