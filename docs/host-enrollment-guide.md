# Fleet Terminal — Host Enrollment Guide

Enrollment makes a managed host reachable through Fleet Terminal. The goal is for
the host to **trust the Fleet user CA** so that any short-lived user certificate
the platform issues is accepted — without distributing per-user keys or managing
`authorized_keys`.

## Concepts

- **User CA.** Fleet runs an SSH certificate authority (`ssh-ed25519`). Its public
  key is configured as a `TrustedUserCAKeys` on every managed host. Its private
  key is encrypted at rest (`FLEET_CA_PASSPHRASE`) and never leaves the backend.
- **Reachability.** Managed hosts are not exposed directly. The backend reaches
  them through the **jump host** (`FLEET_JUMP_HOST`) over a **WireGuard** tunnel
  network. Each host record carries both a routable `address` and a `wg_address`.
- **Host record.** A row in the `hosts` table holds hostname, environment, owner,
  addresses, SSH port/user, tags, and an `enrolled` flag. Collected facts land in
  `host_inventory`; known host keys in `host_fingerprints`; live status in
  `host_status`. Enrollment runs are tracked in `enrollment_jobs`.

## Prerequisites

- The host runs OpenSSH and is reachable from the jump host over the WireGuard
  network.
- You hold the **Host.Enroll** permission.
- The Fleet user CA exists (created automatically on first backend start;
  retrievable via `GET /api/v1/certificates/ca` → `activeUserCA`).

## Step 1 — Add the host to inventory

`POST /api/v1/hosts` (requires `Host.Enroll`):

```json
{
  "hostname": "web-01",
  "description": "frontend node",
  "environment": "production",
  "owner": "platform-team",
  "address": "10.0.1.5",
  "wgAddress": "10.9.0.5",
  "sshPort": 22,
  "sshUser": "fleet",
  "tags": ["web", "edge"]
}
```

The response includes the new host `id`. This writes a `host.create` audit event.

## Step 2 — Trust the Fleet user CA on the host

Fetch the active user CA public key:

```sh
curl -s http://localhost:8080/api/v1/certificates/ca \
  -H "Authorization: Bearer $TOKEN" | jq -r .activeUserCA
```

On the managed host, install it and trust it in `sshd_config`:

```sh
# /etc/ssh/fleet_user_ca.pub  <- contents of activeUserCA
sudo tee /etc/ssh/fleet_user_ca.pub >/dev/null <<'EOF'
ssh-ed25519 AAAA... fleet-user-ca
EOF

# /etc/ssh/sshd_config
TrustedUserCAKeys /etc/ssh/fleet_user_ca.pub

sudo systemctl reload sshd
```

Create the login account the platform uses (matching the host's `ssh_user`,
default `fleet`) and ensure the certificate principals (`fleet` and the
operator's username) map to allowed logins. With CA trust in place you do **not**
add per-user public keys to `authorized_keys`.

## Step 3 — Authorize users via groups

Access is granted by shared group membership:

1. Create or pick a group: `POST /api/v1/groups`.
2. Add the host: `POST /api/v1/hosts/{hostId}/groups/{groupId}` (requires `Host.Edit`).
3. Add users: `POST /api/v1/users/{userId}/groups/{groupId}` (requires `Group.Edit`).

Users without standing access can still request time-boxed access through the
approval workflow (see the [User Guide](./user-guide.md)).

## Step 4 — Verify connectivity

Have an authorized user open a terminal from the **Hosts** page (or, for a smoke
test, confirm the host shows **online** in `GET /api/v1/hosts/stats/status`).
A successful connection means:

- WireGuard tunnel up (`host_status.wg_ok`),
- SSH reachable through the jump host (`host_status.ssh_ok`),
- the host accepted a Fleet-issued user certificate.

## Host key fingerprints

On first contact the host's SSH host key fingerprint is recorded in
`host_fingerprints` (`SHA256:…`) and pinned for subsequent connections, guarding
against man-in-the-middle changes. If a host is rebuilt and its key legitimately
changes, an administrator must clear/refresh the stored fingerprint.

## Rollback & troubleshooting

- Enrollment jobs record an ordered **step log** and status in `enrollment_jobs`
  (`pending → running → succeeded | failed | rolled_back`). A failed run is rolled
  back so the inventory isn't left half-configured.
- **Can't connect / "not authorized for host":** verify the user shares a group
  with the host (or has an active grant) and holds `Host.Connect`.
- **SSH refuses the certificate:** confirm `TrustedUserCAKeys` points at the
  current CA public key. If the CA was **rotated**, re-distribute the new public
  key (see [certificate-lifecycle.md](./certificate-lifecycle.md)).
- **Host shows offline:** check the WireGuard tunnel and that the jump host can
  reach `wg_address:ssh_port`.

## Decommissioning a host

`DELETE /api/v1/hosts/{id}` (requires `Host.Delete`) removes the host and, via
`ON DELETE CASCADE`, its group links, inventory, fingerprints, and status. The
deletion is audited. Optionally remove the `TrustedUserCAKeys` line on the host
if it is leaving the fleet entirely.
