-- Direct user-to-host access grants, complementing group-based access. A user
-- may be given access to an individual host without being placed in a group.
-- Access to a host is therefore the union of: shared group membership,
-- a direct host_users grant, and active temporary (JIT) permissions.
CREATE TABLE IF NOT EXISTS host_users (
    host_id    uuid NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (host_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_host_users_user ON host_users(user_id);
CREATE INDEX IF NOT EXISTS idx_host_users_host ON host_users(host_id);
