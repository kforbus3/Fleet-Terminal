# Fleet Terminal — Security Guide

Fleet Terminal is a Privileged Access Management platform; its security posture is
central to its purpose. This guide documents the controls, their configuration,
and operational recommendations.

## Threat model (summary)

- **No standing SSH keys.** Compromise of a single managed host must not yield
  reusable credentials for the rest of the fleet.
- **Tamper-evident accountability.** An attacker (including an insider with DB
  access) must not be able to silently alter the record of what happened.
- **Backend-authoritative authorization.** A malicious or buggy client must not be
  able to bypass access control.
- **Network isolation.** Managed hosts must not be directly reachable.

## 1. Ephemeral, in-RAM SSH identities

- On login, the **Issuer** generates a fresh `ssh-ed25519` keypair in an
  in-process **Identity Vault** and signs a short-lived user certificate
  (default TTL 7 days, `FLEET_USER_CERT_TTL`), bound to the browser session with
  principals `fleet` + username.
- **Private keys never touch disk or the database.** Only certificate *metadata*
  is persisted in `ssh_certificates` (serial, principals, public key, validity,
  `key_id`). On logout the key is zeroized and its certificates revoked.
- A background loop renews certificates ~24h before expiry
  (`FLEET_CERT_RENEW_BEFORE`) so active sessions don't break.

## 2. Certificate authority hardening

- The CA private key is stored **encrypted at rest** in `ca_keys.private_enc`,
  encrypted with `FLEET_CA_PASSPHRASE` (≥16 bytes, required in production). It
  never leaves the backend process.
- Every issued certificate has a unique, **never-reused serial** from
  `ssh_cert_serial_seq`, enabling precise revocation.
- **Revocation** is recorded in `cert_revocations` and published as a Key
  Revocation List via `GET /api/v1/certificates/krl`.
- **CA rotation** is a single API call (`POST /api/v1/certificates/ca/rotate`);
  the previous CA is retired (`ca_keys.retired_at`) but kept for verification of
  already-issued certs. See [certificate-lifecycle.md](./certificate-lifecycle.md).

## 3. Hash-chained, tamper-evident audit

- Every state change appends a row to `audit_events` where
  `hash = H(prev_hash || canonical(event))`, forming an append-only chain ordered
  by `seq`.
- `actor_name` is denormalized so accountability survives user deletion.
- **Verify integrity** with `GET /api/v1/audit/verify` →
  `{"intact": true, "brokenAtSeq": 0}`. A non-zero `brokenAtSeq` pinpoints the
  first altered/missing row. Run this on a schedule and alert on failure.
- **Export** the full chain with `GET /api/v1/audit/export` (streamed JSON) for
  off-box archival; archive to write-once/immutable storage for strongest
  guarantees.

## 4. Authentication & sessions

- **Passwords:** Argon2id hashing, stored separately from the user row in
  `user_credentials` (least privilege on reads). A configurable `password_policy`
  enforces length/complexity and a no-reuse `pw_history`. `must_change_pw` forces
  rotation; resets can require a change at next login.
- **Lockout:** `lockout_policy` (default 5 failed attempts → 15-minute lockout)
  via `failed_logins` / `locked_until`. Admins can unlock with
  `POST /users/{id}/unlock`.
- **Tokens:** short-lived **access JWTs** (HMAC, `FLEET_JWT_SECRET`, default 15m)
  carried as `Authorization: Bearer`. Rotating **refresh tokens** (default 30d)
  are stored only as hashes (`sessions.refresh_hash`).
- **Cookies:** refresh (`fleet_refresh`) and session-id (`fleet_sid`) cookies are
  HttpOnly, `Secure` (when `FLEET_COOKIE_SECURE=true`), and SameSite=Strict,
  scoped to `/api/v1/auth`. The CSRF cookie (`fleet_csrf`) is JS-readable by
  design (double-submit).
- **MFA:** `mfa_methods` supports TOTP and WebAuthn enrollment.

## 5. Authorization (RBAC)

- Authorization is enforced **server-side only**. Frontend permission checks are
  advisory. Every protected route is wrapped with `RequireAuth` +
  `RequirePermission("<Perm>")`.
- `Admin.All` (held by Super Administrators) is a wildcard that satisfies any
  check. Use it sparingly.
- **Host access** is a separate gate from the RBAC permission: even with
  `Host.Connect`, a user must share a group with the host or hold an active
  temporary grant (super admins bypass). This is enforced at terminal connect
  time (`UserCanAccessHost`).
- Prefer **least privilege**: narrow custom roles plus just-in-time approvals
  over broad standing access.

## 6. CSRF & transport

- State-changing, **cookie-authenticated** requests (`refresh`, `logout`) require
  the double-submit header `X-CSRF-Token` to match the `fleet_csrf` cookie.
  Bearer-only API calls don't rely on cookies and are exempt.
- CORS is restricted to `FLEET_PUBLIC_URL` (plus localhost dev origins) with
  credentials. Terminate TLS at the edge and set `FLEET_COOKIE_SECURE=true`.

## 7. WebSocket terminal

- Browsers can't set `Authorization` on a WebSocket, so the **short-lived access
  token** is passed as the `?token=` query parameter and validated with
  `AuthenticateToken`. Because it is short-lived, exposure window is minimal —
  still, terminate TLS so the URL isn't observable on the wire.
- The endpoint re-checks `Host.Connect` and host authorization before opening the
  PTY.

## 8. SQL & input handling

- **All** database access uses parameterized SQL via the store layer — no string
  concatenation of user input.
- UUIDs and serials are parsed/validated before use; invalid input returns `400`.

## 9. Network isolation

- Managed hosts are reachable only through the **jump host** and a **WireGuard**
  tunnel mesh. The only inbound surface a managed host needs is the WireGuard
  endpoint; SSH is not exposed publicly.
- Host SSH host keys are pinned (`host_fingerprints`, `SHA256:…`) to detect MITM.

## 10. Secrets & configuration

- In `production`, the backend **refuses to start** without strong
  `FLEET_JWT_SECRET` (≥32B), `FLEET_CSRF_SECRET` (≥16B), and
  `FLEET_CA_PASSPHRASE` (≥16B). In `development` it falls back to **insecure
  deterministic defaults** — never run production that way.
- Generate secrets with `openssl rand -hex 32`. Inject via your secret manager
  (`deploy/k8s/11-secret.yaml` for Kubernetes), not committed files.

## 11. Monitoring & response

- Scrape `GET /metrics` (Prometheus): HTTP request counts/latency plus session and
  gateway metrics. Alert on auth-failure spikes and audit-verify failures.
- `auth_events` provides a fast, queryable security event stream
  (`login_failure`, `lockout`, `mfa_*`, `pw_change`) independent of the audit
  chain.
- On suspected compromise: revoke affected certificates (or rotate the CA),
  disable/lock the user, verify the audit chain, and review `auth_events` and
  `ssh_sessions`. See [Disaster Recovery](./disaster-recovery.md).

## Security checklist (production)

- [ ] Strong `FLEET_JWT_SECRET`, `FLEET_CSRF_SECRET`, `FLEET_CA_PASSPHRASE` set.
- [ ] `FLEET_ENV=production`, `FLEET_COOKIE_SECURE=true`, TLS at the edge.
- [ ] `FLEET_ALLOW_BOOTSTRAP=false` after initial setup.
- [ ] Scheduled `audit/verify` with alerting; audit exports archived immutably.
- [ ] Least-privilege roles; `Admin.All` limited to break-glass accounts.
- [ ] CA public key distributed; rotation and revocation procedures rehearsed.
- [ ] Managed hosts reachable only via jump host + WireGuard.
- [ ] Database and recordings backed up and restore-tested.
