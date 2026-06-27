import { api } from "./client";

// Read-only access to recorded SSH sessions and their asciicast recordings
// (/sessions, /sessions/{id}/recording).

export interface SSHSession {
  id: string;
  sessionId?: string;
  userId?: string;
  hostId?: string;
  username: string;
  hostname: string;
  certSerial?: number;
  status: string;
  startedAt: string;
  endedAt?: string;
  exitCode?: number;
  bytesIn: number;
  bytesOut: number;
  clientIp?: string;
  hasRecording: boolean;
}

export interface Recording {
  id: string;
  sshSessionId: string;
  format: string;
  sizeBytes: number;
  durationMs: number;
  sha256: string;
  createdAt: string;
}

export interface RecordingPayload {
  recording: Recording;
  cast: string;
}

export async function listSessions(): Promise<SSHSession[]> {
  const { data } = await api.get<{ sessions: SSHSession[] }>("/api/v1/sessions");
  return data.sessions;
}

export async function getRecording(id: string): Promise<RecordingPayload> {
  const { data } = await api.get<RecordingPayload>(`/api/v1/sessions/${id}/recording`);
  return data;
}

function triggerDownload(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

// downloadRecordingCast exports the raw asciicast (.cast) file (for asciinema CLI).
export async function downloadRecordingCast(id: string): Promise<void> {
  const res = await api.get(`/api/v1/sessions/${id}/recording/download`, { responseType: "blob" });
  triggerDownload(res.data as Blob, `session-${id}.cast`);
}

// downloadRecording exports a SELF-CONTAINED HTML file that plays the recording
// in any browser — double-click to watch. The cast data is embedded inline; the
// asciinema player loads from a CDN (needs internet to render).
export async function downloadRecording(id: string, label?: string): Promise<void> {
  const { cast } = await getRecording(id);
  const title = `Fleet Terminal session — ${label ?? id}`;
  const html = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>${escapeHtml(title)}</title>
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/asciinema-player@3.8.0/dist/bundle/asciinema-player.css">
<style>html,body{margin:0;background:#0d1117;color:#c9d1d9;font-family:system-ui,sans-serif}
header{padding:10px 16px;font-size:14px;border-bottom:1px solid #30363d}#player{padding:8px}</style>
</head><body>
<header>${escapeHtml(title)}</header>
<div id="player"></div>
<script src="https://cdn.jsdelivr.net/npm/asciinema-player@3.8.0/dist/bundle/asciinema-player.min.js"></script>
<script>
const cast = ${JSON.stringify(cast)};
const blob = new Blob([cast], {type: "application/x-asciicast"});
AsciinemaPlayer.create(URL.createObjectURL(blob), document.getElementById("player"), {fit: "width", terminalFontSize: "small"});
</script>
</body></html>`;
  triggerDownload(new Blob([html], { type: "text/html" }), `session-${id}.html`);
}

function escapeHtml(s: string): string {
  return s.replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c] ?? c));
}

export async function deleteRecording(id: string): Promise<void> {
  await api.delete(`/api/v1/sessions/${id}/recording`);
}

export interface RecordingStats {
  count: number;
  bytes: number;
}

export async function recordingStats(): Promise<RecordingStats> {
  const { data } = await api.get<RecordingStats>("/api/v1/recordings/stats");
  return data;
}

export async function pruneRecordings(olderThanDays: number): Promise<{ deleted: number; bytesReclaimed: number }> {
  const { data } = await api.post<{ deleted: number; bytesReclaimed: number }>(
    `/api/v1/recordings/prune?olderThanDays=${olderThanDays}`,
  );
  return data;
}
