import { api } from "./client";

// Read-only access to the tamper-evident audit chain (/audit, /audit/verify).

export interface AuditEvent {
  seq: number;
  id: string;
  actorId?: string;
  actorName?: string;
  action: string;
  targetKind?: string;
  targetId?: string;
  ip?: string;
  detail?: Record<string, unknown>;
  prevHash: string;
  hash: string;
  createdAt: string;
}

export interface AuditFilter {
  action?: string;
  actor?: string;
  limit?: number;
  offset?: number;
}

export interface VerifyResult {
  intact: boolean;
  brokenAtSeq: number;
}

export async function listAudit(filter: AuditFilter = {}): Promise<AuditEvent[]> {
  const { data } = await api.get<{ events: AuditEvent[] }>("/api/v1/audit", {
    params: filter,
  });
  return data.events;
}

export async function verifyAudit(): Promise<VerifyResult> {
  const { data } = await api.get<VerifyResult>("/api/v1/audit/verify");
  return data;
}
