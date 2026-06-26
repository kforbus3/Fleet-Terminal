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
