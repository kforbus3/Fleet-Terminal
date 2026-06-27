# Fleet Terminal — User Guide

Fleet Terminal lets you open SSH sessions and transfer files to your servers
**right from your browser** — no SSH client, no VPN, no keys to manage. This
guide walks through everyday use, step by step.

---

## 1. Sign in

1. Open the Fleet Terminal URL your administrator gave you.
2. Enter your **username** and **password**, then **Sign in**.
3. First time, or after a reset, you may be asked to **set a new password**
   (minimum 12 characters with upper, lower, a digit, and a symbol).

You never see or handle an SSH key. Behind the scenes, signing in creates a
short-lived identity that lives only in the server's memory and is destroyed when
you log out or your session times out (typically 30 minutes idle).

### If two-factor authentication (2FA) is required

- **You already set up an authenticator:** after your password, enter the 6-digit
  code from your app.
- **You haven't set one up yet and it's required:** you'll be shown a setup screen.
  Add the displayed **secret key** to an authenticator app (Google Authenticator,
  1Password, Authy, …), then type the current 6-digit code to finish signing in.
  After this, 2FA is asked for at every sign-in.

You can also set up or manage 2FA anytime under **Security** (including passkeys,
if your administrator has them enabled).

---

## 2. Connect to a server (Terminal)

The quickest path is the **Terminals** page:

1. Click **Terminals** in the sidebar.
2. Find your server (search by name, environment, or tag). Online servers appear
   first, with a status dot and latency.
3. Click **Terminal**. A full SSH terminal opens **in a new browser tab**.
4. Use it like any terminal — run commands, `sudo`, `vim`, `htop`, `tmux`.
   Resizing the window resizes the remote terminal too.

You can also start a terminal from the **Hosts** page using the terminal icon on
a host row. To close a session, type `exit` or close the tab.

> Each connection uses a unique, automatically-issued certificate just for you and
> that server. You'll only see servers you're allowed to reach.

---

## 3. Transfer files (SFTP)

1. On the **Terminals** or **Hosts** page, click **Files** (folder icon) for a
   server. A file browser opens in a new tab.
2. **Download:** click a file. **Upload:** use the upload button or drag-and-drop
   files (or a whole folder) into the window.
3. A progress bar shows transfer status; you can cancel an in-progress transfer.

Every transfer is brokered by the server and recorded for audit.

---

## 4. Request access you don't have (Just-in-Time)

If you don't see a server you need, you can request temporary access:

1. Go to **Approvals → New Request**.
2. Pick the **host** or **group** you need, enter a **reason** and optional
   **ticket reference**, and choose **how long** you need it.
3. Submit. An approver reviews it.
4. When approved, access is granted automatically for the time window and
   **expires on its own** — nothing to clean up. Connect as usual while it's active.

Track your requests under **Approvals → My Requests** and active grants under
**My Grants**.

### If you approve requests

With approver permission, the **Approvals** queue shows pending requests. Open one
to see who's asking, for what, and why, then **Approve** (optionally for a shorter
time, with a note) or **Deny**.

---

## 5. Review a recorded session

If you have replay permission, **Session Replay** lists recorded SSH sessions
(filter by user or host). Open one to **replay** it as a faithful playback —
exactly what happened, with original timing. You can also **export** a session as
a self-contained file to watch offline.

---

## 6. Tips & good habits

- **Show/hide the sidebar** with the menu (☰) button in the top bar — handy on
  wide tables like Hosts.
- **No keys to manage.** You never touch SSH keys or `authorized_keys`; the
  platform issues a fresh short-lived certificate per session, per server.
- **Everything is audited.** Connections, file transfers, and actions are recorded
  in a tamper-evident log.
- **Least privilege.** Prefer Just-in-Time access for occasional needs over
  standing membership in broad groups.
- **Protect your 2FA.** If you lose your authenticator, ask an administrator to
  reset your factors.

---

## 7. Troubleshooting

| Symptom | What to do |
|---|---|
| "Not authorized for host" | You don't have access — request it via **Approvals**. |
| Server isn't in my list | Same — it's not shared with you yet; request access. |
| Signed out unexpectedly | Idle/absolute timeout; just sign in again. |
| "Rate limit exceeded" | Too many rapid attempts from your network; wait a moment. |
| Terminal won't open | Check the server's status on the Terminals page; if offline, contact an admin. |
| Lost my 2FA device | Ask an administrator to reset your MFA. |
