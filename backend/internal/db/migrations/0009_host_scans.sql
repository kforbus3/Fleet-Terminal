-- OpenSCAP security/compliance scans per host. Each row is one scan run; the
-- HTML report is stored on disk (report_path) like session recordings, with a
-- parsed summary (score + pass/fail counts) kept here for listing.

CREATE TABLE IF NOT EXISTS host_scans (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    host_id       UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    requested_by  UUID REFERENCES users(id) ON DELETE SET NULL,
    requester     TEXT NOT NULL DEFAULT '',
    profile       TEXT NOT NULL DEFAULT '',  -- SCAP profile id evaluated
    profile_title TEXT NOT NULL DEFAULT '',
    benchmark     TEXT NOT NULL DEFAULT '',  -- datastream file used
    status        TEXT NOT NULL DEFAULT 'pending', -- pending|running|completed|failed
    score         DOUBLE PRECISION,          -- XCCDF score 0..100
    pass_count    INT NOT NULL DEFAULT 0,
    fail_count    INT NOT NULL DEFAULT 0,
    other_count   INT NOT NULL DEFAULT 0,     -- notapplicable/notchecked/error/etc.
    total_rules   INT NOT NULL DEFAULT 0,
    report_path   TEXT NOT NULL DEFAULT '',
    error         TEXT NOT NULL DEFAULT '',
    started_at    TIMESTAMPTZ,
    finished_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_host_scans_host ON host_scans(host_id, created_at DESC);

-- Permission to run scans and view reports.
INSERT INTO permissions(key, description) VALUES
    ('Host.Scan', 'Run OpenSCAP security scans on hosts and view/export reports')
ON CONFLICT (key) DO NOTHING;

-- Grant to Administrator, Operator, and Auditor (Super Admin has the wildcard).
INSERT INTO role_permissions(role_id, permission_key)
SELECT r.id, 'Host.Scan' FROM roles r
WHERE r.name IN ('Administrator', 'Operator', 'Auditor')
ON CONFLICT DO NOTHING;
