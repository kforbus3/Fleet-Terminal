# Fleet Terminal

**Browser-based Privileged Access Management (PAM) for Linux fleets.**

Fleet Terminal gives operators secure, audited SSH access to hundreds or thousands of
Linux hosts **from the browser** — with no SSH client, VPN, WireGuard, keys, or
certificates on the user side. The browser talks only to the backend over HTTPS/WebSocket;
the **backend is the sole SSH client** and brokers every connection through a jump host and
WireGuard overlay to managed hosts.

```
Browser ──HTTPS/WS──> React SPA ──REST/WS──> Go Backend ──SSH──> Jump Host ──WireGuard──> Managed Hosts
                                                  │
                                   ┌──────────────┴───────────────┐
                                   │ SSH CA · ephemeral identities │
                                   │ RBAC · JIT approvals · audit  │
                                   └───────────────────────────────┘
```

## Why it's different

- **Ephemeral identities.** Every login mints a brand-new Ed25519 keypair **in backend RAM**
  and signs a short-lived (7-day) OpenSSH user certificate. Private keys never touch disk,
  the database, cookies, or the browser, and are zeroized on logout/idle. Certificates
  auto-renew ~24h before expiry and are revoked on session end.
- **Internal SSH Certificate Authority.** The CA private key is generated in-process,
  encrypted at rest (AES-256-GCM), and never leaves the backend. Supports rotation and
  revocation (KRL).
- **Backend-only SSH.** The browser exchanges terminal bytes over a WebSocket; it never
  speaks SSH, holds keys, or gets VPN connectivity.
- **Defense in depth.** Argon2id passwords, JWT access + rotating refresh tokens, CSRF
  double-submit, account lockout, fine-grained RBAC enforced server-side, group-based host
  authorization, just-in-time approvals, and a **hash-chained tamper-evident audit log**.

## Features

| Area | Capability |
|------|-----------|
| Access | Browser SSH terminal (xterm.js), multi-tab, full PTY, session recording & replay |
| Identity | Self-contained auth, Argon2id, optional MFA seams (TOTP/WebAuthn), first-run bootstrap wizard |
| AuthZ | RBAC (built-in + custom roles), host groups, just-in-time temporary access with auto-expiry |
| Hosts | Inventory dashboard, live SSH-based health monitoring, automated enrollment (planned automation) |
| CA | Ephemeral user certs, CA rotation, revocation, lifecycle API |
| Audit | Hash-chained audit with integrity verification + export; full auth event log |
| Ops | Prometheus metrics, structured logs, health/ready endpoints, Docker/K8s/Helm/systemd artifacts |

## Quick start

Requires Docker + Docker Compose. No local Go/Node/Postgres toolchain needed.

```bash
make up        # builds & starts Postgres, Redis, backend, frontend, and the SSH test fabric
```

Then open the frontend (http://localhost:5173). On first run you'll be guided through the
**bootstrap wizard** to create the initial Super Administrator; the wizard then permanently
self-disables.

Useful targets:

```bash
make up-app    # app stack only (no SSH test fabric)
make test      # backend + frontend tests
make logs      # tail logs
make down      # stop;  make clean  # stop + remove volumes
```

## Repository layout

```
backend/    Go API server + SSH gateway (chi, pgx, x/crypto/ssh, gorilla/websocket)
frontend/   React + TypeScript + Vite + MUI + xterm.js + React Query + Zustand
deploy/     docker-compose (app + test fabric), k8s manifests, Helm chart, systemd units
docs/       architecture, API, schema, admin/user/developer/security/DR guides
scripts/    orchestration + dev helpers
```

## Architecture & docs

- [docs/architecture.md](docs/architecture.md) — components, data flows, security model
- [docs/api.md](docs/api.md) — REST API reference
- [docs/database.md](docs/database.md) — schema reference
- [docs/security-guide.md](docs/security-guide.md) · [docs/certificate-lifecycle.md](docs/certificate-lifecycle.md)
- [docs/admin-guide.md](docs/admin-guide.md) · [docs/user-guide.md](docs/user-guide.md) · [docs/host-enrollment-guide.md](docs/host-enrollment-guide.md)

## Status

This repository delivers a complete, coherent foundation with working vertical paths through
every major subsystem (auth, RBAC, bootstrap, hosts, SSH CA, ephemeral identities, gateway,
terminal, recording, audit, JIT approvals, admin). Some advanced automation (host enrollment
push, monitoring scheduler, SFTP UI, MFA enrollment) is scaffolded for incremental deepening.
See the milestone history in `git log`.

## License

Internal / proprietary (adjust as appropriate for your organization).
