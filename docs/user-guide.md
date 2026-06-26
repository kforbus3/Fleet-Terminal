# Fleet Terminal — User Guide

Fleet Terminal lets you open SSH sessions to managed hosts straight from your
browser — no local SSH keys, no VPN client, no copying credentials. This guide
covers everyday use.

## Signing in

1. Open the Fleet Terminal URL your administrator gave you.
2. Enter your **username** and **password**.
3. If your account was created with a temporary password (or a reset was issued),
   you'll be prompted to set a new one before continuing.

Behind the scenes, signing in mints a short-lived, ephemeral SSH identity bound
to your browser session. You never see or manage an SSH key — it lives only in
the server's memory and is destroyed when you log out. Your session refreshes
automatically; you'll be asked to sign in again after the configured idle or
absolute timeout (defaults: 30 min idle, 12 h absolute).

To change your password later, use **Change Password** in the app
(`POST /api/v1/auth/change-password`): supply your current and new password. The
new password must meet the policy (default: ≥12 chars with upper, lower, digit,
and symbol).

## Finding hosts

The **Hosts** page lists the hosts you're authorized to reach. You can:

- Search and filter by hostname, environment, owner, or tags.
- See live status (online / offline / unknown).

You only see hosts you can access — access is granted by being placed in a
**group** that the host also belongs to, or via a temporary just-in-time grant
(below).

## Opening a terminal

1. From the Hosts page, click **Connect** on a host.
2. A browser terminal opens (xterm.js). The connection is a WebSocket to
   `/api/v1/terminal/{hostId}` authenticated with your session token; the backend
   dials the host through the jump host and WireGuard using your ephemeral
   certificate.
3. Type as you would in any SSH terminal. Resize the window and the remote PTY
   resizes with it.

Every keystroke and byte of output is recorded for audit and replay. Sessions
end when you type `exit`, close the tab, or your session expires.

You need the **Host.Connect** permission and authorization for that specific
host. If you lack access you'll see a "not authorized for host" error — request
access (below).

## Requesting just-in-time access

If you need access to a host or group you can't currently reach:

1. Open **Approvals → New Request**.
2. Choose the target (a **host** or a **group**), enter a **reason** and an
   optional **ticket reference**, and the **duration** you need.
3. Submit. An approver reviews and either approves (optionally shortening the
   duration) or denies the request.
4. Once approved, a time-boxed grant is created automatically and expires on its
   own — no cleanup needed. Connect to the host as usual while it's active.

Track your requests under **Approvals → My Requests** and your active grants
under **My Grants**.

## Approving requests (approvers)

If you hold **Approval.Decide**, the **Approvals** page shows all pending
requests. Open one to see the requester, target, reason, ticket, and requested
duration, then **Approve** (optionally setting a shorter granted duration with a
note) or **Deny**. Approving immediately provisions the requester's temporary
access.

## Reviewing recorded sessions

If you have **Session.Replay**, the **Sessions** page lists recorded SSH
sessions (filter by user or host). Open a session to replay it as an asciicast —
a faithful playback of exactly what happened, including timing.

## Tips

- **No keys to manage.** You never handle SSH keys or `authorized_keys`. The
  platform issues a fresh short-lived certificate per session.
- **Everything is audited.** Connections, file transfers, and administrative
  actions are recorded in a tamper-evident log.
- **Least privilege.** Prefer just-in-time access for occasional needs rather
  than standing membership in broad groups.
