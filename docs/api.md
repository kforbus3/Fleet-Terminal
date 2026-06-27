# Fleet Terminal — REST API Reference

All application endpoints are served under the versioned prefix **`/api/v1`**.
Operational endpoints (`/health`, `/ready`, `/version`, `/metrics`) are served at
the root.

## Conventions

- **Format:** request and response bodies are JSON (`Content-Type: application/json`).
- **Authentication:** authenticated endpoints require a bearer access token:
  `Authorization: Bearer <accessToken>`. The token is obtained from
  `POST /api/v1/auth/login` and refreshed via `POST /api/v1/auth/refresh`.
- **Authorization:** authorization is enforced server-side. Each route below
  lists its **Required permission**. The holder of `Admin.All` (Super
  Administrator) passes every permission check. Missing permission → `403`.
- **CSRF:** cookie-authenticated, state-changing calls (`refresh`, `logout`)
  require the double-submit header `X-CSRF-Token: <csrfToken>` matching the
  `fleet_csrf` cookie.
- **Errors:** failures return `{"error": "<message>"}` with an appropriate HTTP
  status (`400`, `401`, `403`, `404`, `409`, `500`).
- **Pagination:** list endpoints accept `?limit=&offset=` query parameters.

> Note on mounting: `bootstrap`, `auth`, `hosts`, and `certificates` are wired in
> `registerRoutes`. The `admin`, `auditapi`, `sessionsapi`, `approvals`, and
> `terminal` modules each expose `func Mount(r chi.Router, d *app.Deps)` and
> attach at the same `/api/v1` mount seam (`mountModules`). Paths and permissions
> below are taken directly from each module's `Mount`.

---

## Operational (unauthenticated)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Liveness. `{"status":"ok"}` |
| GET | `/ready` | Readiness; pings the DB. `200 {"status":"ready"}` or `503 {"status":"db_unavailable"}` |
| GET | `/version` | `{"version":"<build version>"}` |
| GET | `/metrics` | Prometheus metrics (text exposition format) |
| GET | `/api/v1/ping` | `{"pong":"ok"}` |

---

## Bootstrap

First-run wizard. Intentionally unauthenticated — self-gated on the absence of
any user account and on `FLEET_ALLOW_BOOTSTRAP`.

| Method | Path | Required permission |
|--------|------|---------------------|
| GET | `/api/v1/bootstrap/status` | none |
| POST | `/api/v1/bootstrap/init` | none (only works while zero users exist) |

**`GET /bootstrap/status`** → `{"bootstrapAvailable": true}`

**`POST /bootstrap/init`**
```json
{ "username": "admin", "email": "admin@example.com",
  "displayName": "Site Admin", "password": "correct horse battery staple 9!" }
```
→ `201 Created`
```json
{ "status": "bootstrapped",
  "user": { "id": "…", "username": "admin", "isSuperAdmin": true } }
```
Returns `409` once any user exists, `400` on weak password.

---

## Auth

| Method | Path | Required permission |
|--------|------|---------------------|
| POST | `/api/v1/auth/login` | none |
| POST | `/api/v1/auth/refresh` | none (uses refresh cookie) |
| POST | `/api/v1/auth/mfa/verify` | none (uses challenge) |
| POST | `/api/v1/auth/mfa/setup/begin` | none (uses setup token) |
| POST | `/api/v1/auth/mfa/setup/confirm` | none (uses setup token) |
| POST | `/api/v1/auth/logout` | authenticated |
| GET | `/api/v1/auth/me` | authenticated |
| POST | `/api/v1/auth/change-password` | authenticated |
| GET/POST | `/api/v1/auth/mfa[/totp/...]` | authenticated |

**`POST /auth/login`**
```json
{ "username": "admin", "password": "…" }
```
→ `200 OK` (also sets `fleet_refresh`, `fleet_sid`, `fleet_csrf` cookies)
```json
{ "accessToken": "eyJ…", "accessExpiresAt": "2026-06-26T12:15:00Z",
  "csrfToken": "…", "user": { "id": "…", "username": "admin", "roles": ["Super Administrator"] },
  "mustChangePassword": false }
```

If the account has a confirmed factor, login returns `{ "mfaRequired": true,
"challenge": "…" }` — exchange it at `POST /auth/mfa/verify` `{ challenge, code }`.
If MFA is **required but not enrolled**, login returns `{ "mfaEnrollmentRequired":
true, "setupToken": "…" }`; enroll via `POST /auth/mfa/setup/begin` `{ setupToken }`
(returns a TOTP secret) then `POST /auth/mfa/setup/confirm` `{ setupToken, code }`,
which completes login. No session is issued until a factor is confirmed.

**`POST /auth/refresh`** → rotates tokens using the `fleet_refresh` + `fleet_sid`
cookies; returns a fresh `accessToken`, `accessExpiresAt`, and `csrfToken`.

**`GET /auth/me`** →
```json
{ "user": { "id": "…", "username": "admin", "roles": [...], "groups": [...] },
  "permissions": ["Admin.All"], "isSuperAdmin": true }
```

**`POST /auth/change-password`**
```json
{ "currentPassword": "…", "newPassword": "…" }
```
→ `{"status":"password_changed"}`

---

## Hosts

Inventory CRUD. The list endpoint shows all hosts to holders of `Host.Enroll` /
`Admin.All`; otherwise it is restricted to hosts the principal can access.

| Method | Path | Required permission |
|--------|------|---------------------|
| GET | `/api/v1/hosts` | `Host.View` |
| GET | `/api/v1/hosts/{id}` | `Host.View` |
| GET | `/api/v1/hosts/stats/status` | `Host.View` |
| POST | `/api/v1/hosts` | `Host.Enroll` |
| PUT | `/api/v1/hosts/{id}` | `Host.Edit` |
| DELETE | `/api/v1/hosts/{id}` | `Host.Delete` |
| POST | `/api/v1/hosts/{id}/groups/{groupId}` | `Host.Edit` |
| DELETE | `/api/v1/hosts/{id}/groups/{groupId}` | `Host.Edit` |
| GET | `/api/v1/hosts/{id}/access` | `Host.Edit` |
| POST | `/api/v1/hosts/{id}/users/{userId}` | `Host.Edit` |
| DELETE | `/api/v1/hosts/{id}/users/{userId}` | `Host.Edit` |
| POST | `/api/v1/hosts/{id}/enroll` | `Host.Enroll` |
| GET | `/api/v1/hosts/{id}/enroll/script` | `Host.Enroll` |
| POST | `/api/v1/hosts/{id}/enroll/finish` | `Host.Enroll` |
| GET (WS) | `/api/v1/hosts/{id}/enroll/agent` | `Host.Enroll` (token) |

**Enrollment methods** — `POST /hosts/{id}/enroll` body selects `method`:
`"password"` (`+ bootstrapUser, password`), `"key"` (`+ privateKey, keyPassphrase`),
`"trusted"`, or omit for trusted. `agent` uses the WebSocket (`fleet-enroll-agent`
bridge); the no-install flow uses `GET …/enroll/script` (pipe through your own
ssh) then `POST …/enroll/finish` `{ "hostPublicKey": "…" }`. See the
[Host Enrollment Guide](./host-enrollment-guide.md).

**`GET /hosts/{id}/access`** → `{ "groups": ["ops"], "users": [ … ] }`

**`POST /hosts`**
```json
{ "hostname": "web-01", "description": "frontend node", "environment": "production",
  "owner": "platform", "address": "10.0.1.5", "wgAddress": "10.9.0.5",
  "sshPort": 22, "sshUser": "fleet", "tags": ["web","edge"] }
```
→ `201` with the created host object.

**`GET /hosts`** → `{ "hosts": [ … ], "count": 12 }`

**`GET /hosts/stats/status`** → counts by status, e.g. `{"online":9,"offline":2,"unknown":1}`

---

## Users, Roles, Groups, Settings (admin module)

### Users

| Method | Path | Required permission |
|--------|------|---------------------|
| GET | `/api/v1/users` | `User.Edit` |
| POST | `/api/v1/users` | `User.Create` |
| GET | `/api/v1/users/{id}` | `User.Edit` |
| PUT | `/api/v1/users/{id}` | `User.Edit` |
| DELETE | `/api/v1/users/{id}` | `User.Delete` |
| POST | `/api/v1/users/{id}/disable` | `User.Edit` |
| POST | `/api/v1/users/{id}/unlock` | `User.Edit` |
| POST | `/api/v1/users/{id}/require-mfa` | `User.Edit` |
| GET | `/api/v1/users/{id}/hosts` | `User.Edit` |
| POST | `/api/v1/users/{id}/reset-password` | `User.ResetPassword` |
| POST | `/api/v1/users/{id}/roles/{roleId}` | `Role.Edit` |
| DELETE | `/api/v1/users/{id}/roles/{roleId}` | `Role.Edit` |
| POST | `/api/v1/users/{id}/groups/{groupId}` | `Group.Edit` |
| DELETE | `/api/v1/users/{id}/groups/{groupId}` | `Group.Edit` |

**`POST /users`**
```json
{ "username": "alice", "email": "alice@example.com", "displayName": "Alice",
  "password": "…", "isSuperAdmin": false, "mustChangePassword": true }
```

**`POST /users/{id}/disable`** → `{ "disabled": true }`

**`POST /users/{id}/reset-password`** → `{ "newPassword": "…", "mustChangePassword": true }`

### Roles & permissions

| Method | Path | Required permission |
|--------|------|---------------------|
| GET | `/api/v1/roles` | `Role.Edit` |
| POST | `/api/v1/roles` | `Role.Create` |
| DELETE | `/api/v1/roles/{id}` | `Role.Delete` |
| PUT | `/api/v1/roles/{id}/permissions` | `Role.Edit` |
| GET | `/api/v1/permissions` | `Role.Edit` |

**`POST /roles`** → `{ "name": "Deployer", "description": "CI/CD operators" }`

**`PUT /roles/{id}/permissions`** → `{ "permissions": ["Host.View","Host.Connect","Session.Start"] }`

### Groups

| Method | Path | Required permission |
|--------|------|---------------------|
| GET | `/api/v1/groups` | `Group.Edit` |
| POST | `/api/v1/groups` | `Group.Create` |
| DELETE | `/api/v1/groups/{id}` | `Group.Delete` |

**`POST /groups`** → `{ "name": "web-team", "description": "Owns the web tier" }`

### System settings

| Method | Path | Required permission |
|--------|------|---------------------|
| GET | `/api/v1/settings` | `System.Configure` |
| GET | `/api/v1/settings/{key}` | `System.Configure` |
| PUT | `/api/v1/settings/{key}` | `System.Configure` |

Known setting keys (seeded): `password_policy`, `lockout_policy`, `session_policy`.

---

## Audit

Hash-chained, tamper-evident audit log.

| Method | Path | Required permission |
|--------|------|---------------------|
| GET | `/api/v1/audit` | `Audit.View` |
| GET | `/api/v1/audit/verify` | `Audit.View` |
| GET | `/api/v1/audit/export` | `Audit.Export` |

**`GET /audit?action=host.create&actor=<uuid>&limit=50&offset=0`** →
```json
{ "events": [
    { "seq": 42, "id": "…", "actorName": "admin", "action": "host.create",
      "targetKind": "host", "targetId": "…", "prevHash": "…", "hash": "…",
      "createdAt": "2026-06-26T11:00:00Z" }
  ], "count": 1 }
```

**`GET /audit/verify`** → `{ "intact": true, "brokenAtSeq": 0 }` (a non-zero
`brokenAtSeq` identifies the first tampered/broken row).

**`GET /audit/export`** → streams the entire chain as a JSON array with
`Content-Disposition: attachment; filename="audit-export.json"`.

---

## Sessions (replay)

Read-only access to recorded SSH sessions and their `asciicast-v2` recordings.

| Method | Path | Required permission |
|--------|------|---------------------|
| GET | `/api/v1/sessions` | `Session.Replay` |
| GET | `/api/v1/sessions/{id}` | `Session.Replay` |
| GET | `/api/v1/sessions/{id}/recording` | `Session.Replay` |

**`GET /sessions?user=<uuid>&host=<uuid>&limit=&offset=`** →
`{ "sessions": [ … ], "count": n }`

**`GET /sessions/{id}/recording`** →
```json
{ "recording": { "format": "asciicast-v2", "durationMs": 84200, "sha256": "…" },
  "cast": "{\"version\":2,...}\n[0.1,\"o\",\"...\"]\n…" }
```

---

## Approvals (just-in-time access)

| Method | Path | Required permission |
|--------|------|---------------------|
| POST | `/api/v1/approvals` | `Approval.Request` |
| GET | `/api/v1/approvals` | `Approval.Request` (deciders see all; requesters see their own) |
| GET | `/api/v1/approvals/mine` | `Approval.Request` |
| GET | `/api/v1/approvals/grants/mine` | `Approval.Request` |
| POST | `/api/v1/approvals/{id}/decide` | `Approval.Decide` |

**`POST /approvals`**
```json
{ "targetKind": "host", "hostId": "…", "reason": "incident #4821",
  "ticketRef": "INC-4821", "requestedSecs": 3600 }
```
(`targetKind` is `host` or `group`; supply `hostId` or `groupId` accordingly.)

**`POST /approvals/{id}/decide`**
```json
{ "decision": "approve", "note": "approved for 30m", "grantedSecs": 1800 }
```
An approval mints a `temporary_permissions` grant that expires automatically.

**`GET /approvals/grants/mine`** → `{ "grants": [ … ], "count": n }`

---

## Certificates (CA lifecycle)

| Method | Path | Required permission |
|--------|------|---------------------|
| GET | `/api/v1/certificates` | `Certificate.Manage` |
| GET | `/api/v1/certificates/ca` | `Certificate.Manage` |
| POST | `/api/v1/certificates/ca/rotate` | `Certificate.Manage` |
| GET | `/api/v1/certificates/krl` | `Certificate.Manage` |
| POST | `/api/v1/certificates/{serial}/revoke` | `Certificate.Manage` |

**`GET /certificates/ca`** →
```json
{ "cas": [ { "kind": "user", "fingerprint": "SHA256:…", "active": true } ],
  "activeUserCA": "ssh-ed25519 AAAA… fleet-user-ca" }
```

**`POST /certificates/ca/rotate`** → `{ "status": "rotated", "activeCa": "<id>" }`

**`POST /certificates/{serial}/revoke`** → `{ "reason": "compromised" }` →
`{ "status": "revoked" }`

**`GET /certificates/krl`** → `{ "revokedSerials": [12, 87, 145] }`

See [certificate-lifecycle.md](./certificate-lifecycle.md) for the full lifecycle.

---

## Terminal (WebSocket)

| Method | Path | Required permission |
|--------|------|---------------------|
| GET (Upgrade) | `/api/v1/terminal/{hostId}?token=<accessToken>` | `Host.Connect` + host authorization |

The browser authenticates by passing the short-lived access token as the `token`
query parameter (a WebSocket cannot carry an `Authorization` header). The backend
additionally enforces host authorization (group membership or an active temporary
grant; super admins bypass).

**Client → server** control frames (text):
```json
{ "type": "resize", "cols": 120, "rows": 32 }
{ "type": "data", "data": "ls -la\n" }
```
Binary frames carry raw terminal input. **Server → client** binary frames carry
terminal output; a `{"type":"error","data":"…"}` text frame reports failures.

---

## Health & metrics summary

- `GET /health` — process liveness.
- `GET /ready` — DB-backed readiness (used by orchestrators).
- `GET /version` — build version string.
- `GET /metrics` — Prometheus counters/histograms (`fleet_http_requests_total`,
  `fleet_http_request_duration_seconds`, plus session/gateway metrics).
