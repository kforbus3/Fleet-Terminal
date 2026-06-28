-- Host.Sudo gates root (sudo) on connected hosts.
--
-- An authorized user always connects; this permission decides WHICH account
-- they land in on the host:
--   * with Host.Sudo    -> the privileged shared account (host SSH user, NOPASSWD sudo)
--   * without Host.Sudo -> the login-only account (<ssh-user>-login, no sudo)
-- The login-only account is created at host enrollment, so hosts enrolled
-- before this change must be re-enrolled (or re-provisioned) for login-only
-- access to work.
--
-- Granted to Administrator and Operator by default so existing behavior
-- (connect == root) is preserved on upgrade. To make a set of users login-only,
-- give them a role WITHOUT Host.Sudo. (Super Administrator keeps root via the
-- Admin.All wildcard.)

INSERT INTO permissions(key, description) VALUES
    ('Host.Sudo', 'Run sudo (root) on connected hosts; without it, connect as a login-only (no-sudo) account')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions(role_id, permission_key)
SELECT r.id, 'Host.Sudo' FROM roles r
WHERE r.name IN ('Administrator', 'Operator')
ON CONFLICT DO NOTHING;
