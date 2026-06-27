# Operations Guide

Day-to-day operator flows for Fleet Terminal. Assumes the stack is up via `make up`.

## First run

1. `make up` — starts Postgres, Redis, backend, frontend, and the local SSH test fabric
   (jump host + Ubuntu/Rocky managed hosts with userspace WireGuard).
2. Open the frontend (http://localhost:5173). The **bootstrap wizard** appears on first run;
   create the initial Super Administrator. The wizard then permanently self-disables.
3. `make trust` — seeds the test-fabric nodes with the backend's SSH CA public key so they
   trust issued certificates. In production this trust is established during enrollment over a
   bootstrap credential; in the local fabric we seed it directly. Re-run after any fresh `make up`.

## Adding & enrolling a host

1. **Hosts → New Host**. Provide:
   - **Hostname** (required)
   - **Address** — the management address used to reach the host during enrollment
     (test fabric: `172.30.0.21` for `host-ubuntu`, `172.30.0.22` for `host-rocky`)
   - **WireGuard Address** — leave blank to **auto-assign** the next free overlay address, or
     type a specific one (validated to be in the overlay subnet and not already used)
   - **SSH user** — `fleet` for the test fabric
2. Click the **Enroll** (cable) icon on the host row. Enrollment, over SSH:
   - reads the jump host's WireGuard key,
   - cert-authenticates to the managed host and collects facts,
   - generates the host's WireGuard keypair **on the host** (private key never leaves it),
   - brings up `wg0` with the overlay address and writes `/etc/wireguard/wg0.conf`,
   - registers the host as a peer on the jump host (the VPN server),
   - verifies the handshake.
   A dialog streams each step and shows the assigned overlay address.

> WireGuard address pool and endpoints are configured via `FLEET_WG_SUBNET`,
> `FLEET_WG_JUMP_IP`, `FLEET_WG_JUMP_ENDPOINT`, and `FLEET_WG_PORT`.

## Connecting a terminal

Click the **terminal** icon on a host row (or navigate to `/terminals/<hostId>`). The browser
opens a WebSocket to the backend, which is the only SSH client; it dials the host through the
jump host using your session's ephemeral certificate. The session is recorded (replay under
**Session Replay**) and audited.

## Transferring files (SFTP)

Click the **folder** icon on a host row (or `/files/<hostId>`). Browse directories, download
files, and upload (button or drag-and-drop). Every transfer is brokered by the backend and
recorded in the audit log.

## Live monitoring

The monitor runs authenticated SSH health checks (no ICMP) against enrolled hosts every 30s,
updating status (online/offline/unknown), latency, uptime, and WireGuard handshake freshness.
The dashboard subscribes to a WebSocket and updates in real time.

## Two-factor authentication (TOTP)

1. **Security → Set up authenticator**. Add the shown secret / `otpauth` URL to an authenticator
   app (the secret is displayed only once), then enter the current 6-digit code to confirm.
2. Subsequent sign-ins prompt for the code after the password step.
3. Remove a factor from the same page. (Admins can reset a locked-out user's factors.)

## Just-in-time access

Users without permanent group access request temporary access under **Approvals → My requests**
(reason, duration, optional ticket). Approvers act under **Approvals → Queue**. Grants expire
automatically; a background reaper revokes elapsed grants every minute.

## Audit integrity

**Audit → Verify integrity** recomputes the hash chain; any tampering with a historical row
makes verification fail and reports the first broken sequence number.
