# Fleet Terminal — Administrator Guide

This guide is for operators who deploy and administer Fleet Terminal: bootstrap,
users, roles, groups, hosts, settings, and day-to-day operations.

## 1. Deploy the stack

Fleet Terminal runs entirely in Docker. From the repository root:

```sh
make env      # create .env from .env.example
# edit .env: set strong secrets for production (see below)
make up       # build & start the full stack (+ local test fabric)
make ps       # confirm services are healthy
make logs     # tail logs
```

`make up` starts PostgreSQL, Redis, the backend, the frontend, and a local test
fabric (jump host + sample managed hosts). For a production-style stack without
the test fabric, use `make up-app`. Kubernetes manifests are in `deploy/k8s/`.

Check health any time:

```sh
curl -s http://localhost:8080/health   # {"status":"ok"}
curl -s http://localhost:8080/ready    # {"status":"ready"} once the DB is up
```

### Production secrets

Set these in `.env` (generate with `openssl rand -hex 32`). In `production`
(`FLEET_ENV=production`) the backend refuses to start without them:

| Variable | Requirement |
|----------|-------------|
| `FLEET_JWT_SECRET` | ≥ 32 bytes — signs access tokens |
| `FLEET_CSRF_SECRET` | ≥ 16 bytes — CSRF double-submit |
| `FLEET_CA_PASSPHRASE` | ≥ 16 bytes — encrypts the CA private key at rest |
| `FLEET_COOKIE_SECURE` | `true` when served over HTTPS |
| `FLEET_PUBLIC_URL` | your external base URL (cookies/CORS) |

## 2. Bootstrap the first administrator

On first run, no users exist and the **bootstrap wizard** is open. This is a
one-time flow that creates the initial **Super Administrator**.

1. Open the frontend; it detects `GET /api/v1/bootstrap/status` →
   `{"bootstrapAvailable": true}` and shows the wizard.
2. Enter a username, email, display name, and a password that satisfies the
   password policy (default: ≥12 chars, upper/lower/digit/symbol).
3. Submit. The backend creates the user with `is_super_admin = true`, grants the
   built-in **Super Administrator** role, and writes a `bootstrap.init` audit
   event.

The wizard **permanently closes** as soon as any user exists — subsequent calls
to `/api/v1/bootstrap/init` return `409`. It can only be reopened through the
offline recovery procedure (see [Disaster Recovery](./disaster-recovery.md)).

> You can also disable bootstrap entirely with `FLEET_ALLOW_BOOTSTRAP=false`.

CLI equivalent (useful for headless setup):

```sh
curl -s http://localhost:8080/api/v1/bootstrap/init \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","email":"admin@example.com","displayName":"Admin","password":"<strong>"}'
```

## 3. Built-in roles & permissions

RBAC is backend-authoritative. The following roles are seeded:

| Role | Capabilities |
|------|--------------|
| **Super Administrator** | `Admin.All` — wildcard, unrestricted |
| **Administrator** | every permission except the `Admin.All` wildcard |
| **Operator** | `Host.View`, `Host.Connect`, `Session.Start`, `Session.Replay`, `File.Transfer`, `Approval.Request` |
| **Auditor** | `Host.View`, `Audit.View`, `Audit.Export`, `Session.Replay` |
| **Read-Only** | `Host.View` |

The full permission catalog is in [database.md](./database.md#permissions). You
can create custom roles and assign any subset of permissions.

## 4. Manage users

(Requires `User.*` / `Role.Edit` / `Group.Edit` permissions; the admin module
endpoints are under `/api/v1/users`, `/roles`, `/groups`.)

- **Create:** `POST /users` with `username`, `email`, `displayName`, `password`,
  optional `isSuperAdmin`, `mustChangePassword`.
- **Edit / disable / unlock:** `PUT /users/{id}`, `POST /users/{id}/disable`,
  `POST /users/{id}/unlock` (clears lockout from failed logins).
- **Reset password:** `POST /users/{id}/reset-password` (set `mustChangePassword`
  to force a change at next login).
- **Assign roles / groups:** `POST /users/{id}/roles/{roleId}` and
  `POST /users/{id}/groups/{groupId}` (and the matching `DELETE`s).

## 5. Manage roles & groups

- **Roles:** `POST /roles`, `DELETE /roles/{id}` (built-in roles are protected),
  `PUT /roles/{id}/permissions` with `{"permissions": ["Host.View", …]}`.
- **Permissions catalog:** `GET /permissions`.
- **Groups:** `POST /groups`, `DELETE /groups/{id}`. **Group membership is how
  host access is granted** — a user can connect to a host when they share a group
  with it (or hold a temporary grant). Add hosts to groups via
  `POST /hosts/{id}/groups/{groupId}`.

## 6. Manage hosts

Add hosts to the inventory (`POST /hosts`, requires `Host.Enroll`), then enroll
them so they trust the Fleet user CA — see the
[Host Enrollment Guide](./host-enrollment-guide.md). Authorize users by placing
hosts and users in shared groups.

`GET /hosts/stats/status` returns live counts (online / offline / unknown) for
dashboards.

## 7. System settings

`System.Configure` holders manage settings via `/api/v1/settings`:

| Key | Default | Controls |
|-----|---------|----------|
| `password_policy` | min 12, upper/lower/digit/symbol, history 5 | password complexity + reuse |
| `lockout_policy` | max 5 failed, 15 min lockout | account lockout |
| `session_policy` | idle 30 min, absolute 12 h | session lifetime |

## 8. Audit & monitoring

- **Audit log:** `GET /api/v1/audit` (filter by `action`, `actor`). Every state
  change is recorded in a hash-chained log.
- **Integrity check:** `GET /api/v1/audit/verify` → `{"intact":true,"brokenAtSeq":0}`.
  Run this periodically; a non-zero `brokenAtSeq` indicates tampering.
- **Export:** `GET /api/v1/audit/export` streams the full chain for archival.
- **Metrics:** scrape `GET /metrics` (Prometheus). See [Security Guide](./security-guide.md).

## 9. Certificates

Manage the CA and certificate lifecycle (`Certificate.Manage`) under
`/api/v1/certificates`: list issued certs, rotate the CA, revoke a serial, and
fetch the KRL. Details in [certificate-lifecycle.md](./certificate-lifecycle.md).

## 10. Routine operations

| Task | Command / endpoint |
|------|--------------------|
| Tail logs | `make logs` |
| Restart stack | `make down && make up` |
| Stop (keep data) | `make down` |
| Wipe everything | `make clean` (**destroys volumes**) |
| Health / readiness | `GET /health`, `GET /ready` |
| Verify audit chain | `GET /api/v1/audit/verify` |

For backup, restore, and recovery, see [Disaster Recovery](./disaster-recovery.md).
