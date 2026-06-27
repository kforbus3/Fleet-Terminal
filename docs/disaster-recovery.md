# Fleet Terminal — Disaster Recovery

This guide covers backup, restore, and recovery scenarios for Fleet Terminal. The
platform's durable state lives in **PostgreSQL** and the **session recordings
directory**; the CA private key (encrypted) lives in the database, protected by
`FLEET_CA_PASSPHRASE`.

## What must be protected

| Asset | Where | Notes |
|-------|-------|-------|
| Relational state | PostgreSQL (`pgdata` volume) | users, RBAC, hosts, certs, sessions, audit, settings |
| CA private key | `ca_keys.private_enc` in PostgreSQL | encrypted with `FLEET_CA_PASSPHRASE` |
| `FLEET_CA_PASSPHRASE` | secret store / `.env` | **without it the CA key is unrecoverable** |
| `FLEET_JWT_SECRET`, `FLEET_CSRF_SECRET` | secret store / `.env` | losing these invalidates live tokens (users re-login) |
| Session recordings | `recordings` volume (`FLEET_RECORDING_DIR`) | asciicast files referenced by `session_recordings.path` |
| Audit export | external archive | immutable copy of the hash chain |

> **Critical:** back up the secrets (`FLEET_CA_PASSPHRASE` especially) **out of
> band** from the database. A database backup is useless for CA recovery without
> the passphrase. Losing the passphrase means rotating to a brand-new CA and
> re-trusting it on every managed host.

## Backups

The stack is defined in `deploy/compose/docker-compose.yml` with named volumes
`pgdata`, `redisdata`, and `recordings`. Redis is a cache/job broker and does not
need backup.

### Database (logical dump)

```sh
# from the repo root; service name is "postgres", db/user "fleet"
docker compose -f deploy/compose/docker-compose.yml exec -T postgres \
  pg_dump -U fleet -d fleet --format=custom \
  > backups/fleet-$(date +%Y%m%d-%H%M%S).dump
```

Automate daily; retain per your compliance window. Test restores regularly.

### Session recordings

```sh
docker run --rm -v compose_recordings:/data -v "$PWD/backups:/backup" alpine \
  tar czf /backup/recordings-$(date +%Y%m%d).tar.gz -C /data .
```

(Volume name is `compose_recordings` under the default Compose project; confirm
with `docker volume ls`.)

### Audit chain export

Independently of the DB dump, archive the tamper-evident chain to immutable
storage:

```sh
curl -s "http://localhost:8080/api/v1/audit/export" \
  -H "Authorization: Bearer $TOKEN" \
  -o backups/audit-$(date +%Y%m%d).json
```

### Secrets

Store `FLEET_JWT_SECRET`, `FLEET_CSRF_SECRET`, and `FLEET_CA_PASSPHRASE` in your
secret manager (or `deploy/k8s/11-secret.yaml`). Keep a sealed offline copy of
`FLEET_CA_PASSPHRASE`.

## Restore (full)

1. Provision a clean stack but **do not** let the app create a fresh DB you'll
   overwrite. Bring up only the database:
   ```sh
   docker compose -f deploy/compose/docker-compose.yml up -d postgres
   ```
2. Restore the dump:
   ```sh
   docker compose -f deploy/compose/docker-compose.yml exec -T postgres \
     pg_restore -U fleet -d fleet --clean --if-exists < backups/fleet-YYYYMMDD-HHMMSS.dump
   ```
3. Restore the recordings volume (reverse of the tar backup above).
4. Provide the **same** `FLEET_CA_PASSPHRASE`, `FLEET_JWT_SECRET`, and
   `FLEET_CSRF_SECRET` as the original deployment.
5. Bring up the rest of the stack:
   ```sh
   make up-app
   ```
6. **Verify** (see below).

On startup the backend runs migrations idempotently (`FLEET_MIGRATE_ON_START`)
and `EnsureUserCA` finds the existing CA — it will **not** create a new one, so
managed hosts continue to trust it.

## Verification after restore

```sh
curl -s http://localhost:8080/ready                              # {"status":"ready"}
curl -s http://localhost:8080/api/v1/audit/verify -H "Authorization: Bearer $TOKEN"
# expect {"intact": true, "brokenAtSeq": 0}
curl -s http://localhost:8080/api/v1/certificates/ca -H "Authorization: Bearer $TOKEN"
# confirm activeUserCA matches what managed hosts trust
```

Then have a user open a terminal to confirm end-to-end SSH works.

## Recovery scenarios

### Lost all administrators / locked out

The bootstrap wizard self-closes once any user exists. To recover:

1. Confirm there is genuinely no usable admin (`SELECT username, is_super_admin,
   is_disabled, locked_until FROM users;`).
2. **Preferred — unlock/reset in place** (direct DB access): clear a lockout or
   reset a known admin, e.g.
   ```sql
   UPDATE users SET is_disabled=false, locked_until=NULL, failed_logins=0
   WHERE username='admin';
   ```
   Re-issue the password hash via the app's reset flow if you still have any
   account that holds `User.ResetPassword`.
3. **Reopen bootstrap (offline recovery):** with the app stopped, delete the
   remaining (orphaned) user rows so the user count reaches zero, set
   `FLEET_ALLOW_BOOTSTRAP=true`, restart, and re-run the wizard. This is a
   break-glass procedure — perform it deliberately and audit it. Re-disable
   bootstrap (`FLEET_ALLOW_BOOTSTRAP=false`) afterward.

### CA key compromise

1. Rotate the CA: `POST /api/v1/certificates/ca/rotate`.
2. Distribute the new `activeUserCA` public key to every managed host's
   `TrustedUserCAKeys`.
3. Revoke outstanding certificates if needed and publish the KRL
   (`GET /api/v1/certificates/krl`) to hosts' `RevokedKeys`.
See [certificate-lifecycle.md](./certificate-lifecycle.md).

### Lost `FLEET_CA_PASSPHRASE`

The stored CA private key cannot be decrypted. You must generate a **new** CA
(rotate) and re-trust its public key on all hosts. Existing certificates signed by
the old CA can no longer be renewed; sessions will need re-issuance under the new
CA.

### Lost `FLEET_JWT_SECRET` / `FLEET_CSRF_SECRET`

Live access tokens and CSRF cookies become invalid; users simply log in again. Set
fresh strong secrets and restart.

### Suspected audit tampering

`GET /api/v1/audit/verify` returns `intact: false` with the first broken `seq`.
Compare against your immutable audit exports to determine what was altered, treat
as a security incident, and preserve evidence (DB snapshot + exports).

## RPO / RTO planning

- **RPO** is bounded by your DB backup + audit-export frequency (e.g. daily dumps
  + continuous WAL archiving for near-zero RPO).
- **RTO** is dominated by DB restore time and re-distributing the CA public key
  only if the CA had to be regenerated.
- Rehearse restores quarterly and after any schema migration. The whole stack is
  reproducible via `make up` / `make up-app`, so DR drills are cheap.

## Backup & Restore (operational)

**Backup** — Admins can download a full logical backup from **Settings → Backup & Restore**
(or `GET /api/v1/system/backup`, which streams `pg_dump --clean --if-exists --no-owner`).
Automate it with a cron that calls the endpoint with an admin token, or run `pg_dump`
directly against `FLEET_DATABASE_URL`.

**Restore** — performed offline (intentionally not exposed in the web UI):

```bash
psql "$FLEET_DATABASE_URL" < fleet-backup.sql
```

**Recovery when locked out** — use the bundled `fleetctl` CLI (ships in the backend image):

```bash
fleetctl create-admin <username> <password>   # new Super Administrator
fleetctl reset-mfa <username>                  # clear a user's MFA
fleetctl enable-user <username>                # re-enable + unlock
fleetctl rotate-ca                             # rotate the user CA
```

Note: SSH **session recordings** live on the recordings volume (`FLEET_RECORDING_DIR`),
not in the database — back that volume up alongside the SQL dump.
