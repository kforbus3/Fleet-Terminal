import { api } from "./client";

export interface SSHCertificate {
  id: string;
  serial: number;
  kind: string;
  userId?: string;
  sessionId?: string;
  hostId?: string;
  keyId: string;
  principals: string[];
  publicKey: string;
  issuedAt: string;
  expiresAt: string;
  revokedAt?: string;
  revokeReason?: string;
}

export interface CACert {
  id: string;
  kind: string;
  algo: string;
  publicKey: string;
  fingerprint: string;
  active: boolean;
  createdAt: string;
  retiredAt?: string;
}

export async function listCertificates(limit = 200): Promise<SSHCertificate[]> {
  const { data } = await api.get<{ certificates: SSHCertificate[] }>(`/api/v1/certificates?limit=${limit}`);
  return data.certificates ?? [];
}

export async function listCAs(): Promise<{ cas: CACert[]; activeUserCA: string }> {
  const { data } = await api.get<{ cas: CACert[]; activeUserCA: string }>("/api/v1/certificates/ca");
  return data;
}

export async function rotateCA(): Promise<void> {
  await api.post("/api/v1/certificates/ca/rotate");
}

export async function revokeCertificate(serial: number, reason: string): Promise<void> {
  await api.post(`/api/v1/certificates/${serial}/revoke`, { reason });
}
