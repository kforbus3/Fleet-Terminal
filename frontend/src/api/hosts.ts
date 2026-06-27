import { api } from "./client";

// Host inventory types mirror backend/internal/models/models.go (camelCase JSON).
export interface HostInventory {
  osName: string;
  osVersion: string;
  kernelVersion: string;
  architecture: string;
  sshVersion: string;
  cpuCount: number;
  memoryMb: number;
  collectedAt?: string;
}

export interface HostStatus {
  status: string; // online|offline|unknown
  sshOk: boolean;
  wgOk: boolean;
  latencyMs?: number;
  uptimeSeconds?: number;
  lastSuccessAt?: string;
  lastFailureAt?: string;
  lastError?: string;
  checkedAt?: string;
}

export interface Host {
  id: string;
  hostname: string;
  description: string;
  environment: string;
  owner: string;
  address?: string;
  wgAddress?: string;
  sshPort: number;
  sshUser: string;
  tags: string[];
  enrolled: boolean;
  createdAt: string;
  updatedAt: string;
  groups?: string[];
  inventory?: HostInventory;
  status?: HostStatus;
}

export interface HostListResponse {
  hosts: Host[];
  count: number;
}

// HostInput is the create/update payload accepted by the backend.
export interface HostInput {
  hostname: string;
  description: string;
  environment: string;
  owner: string;
  address: string;
  wgAddress: string;
  sshPort: number;
  sshUser: string;
  tags: string[];
}

export async function listHosts(): Promise<HostListResponse> {
  const { data } = await api.get<HostListResponse>("/api/v1/hosts");
  return data;
}

export async function getHost(id: string): Promise<Host> {
  const { data } = await api.get<Host>(`/api/v1/hosts/${id}`);
  return data;
}

export async function createHost(input: HostInput): Promise<Host> {
  const { data } = await api.post<Host>("/api/v1/hosts", input);
  return data;
}

export async function updateHost(id: string, input: HostInput): Promise<Host> {
  const { data } = await api.put<Host>(`/api/v1/hosts/${id}`, input);
  return data;
}

export async function deleteHost(id: string): Promise<void> {
  await api.delete(`/api/v1/hosts/${id}`);
}

export interface NextWG {
  nextWgAddress: string;
  subnet: string;
  exhausted?: boolean;
}

// nextWGAddress returns what auto-assignment would pick from the overlay pool.
export async function nextWGAddress(): Promise<NextWG> {
  const { data } = await api.get<NextWG>("/api/v1/hosts/wg/next");
  return data;
}

export type HostStatusStats = Record<string, number>;

export async function getHostStatusStats(): Promise<HostStatusStats> {
  const { data } = await api.get<HostStatusStats>("/api/v1/hosts/stats/status");
  return data;
}

export interface EnrollmentStep {
  name: string;
  status: string;
  detail?: string;
  timestamp: string;
}

export interface EnrollmentResult {
  job: { id: string; status: string; steps: EnrollmentStep[]; error?: string };
  wgAddress: string;
  hostPublicKey: string;
}

export interface EnrollParams {
  // "password" bootstraps a host with no prior setup (installs CA trust +
  // WireGuard over an SSH password); "trusted" uses the session certificate on
  // a host that already trusts the Fleet CA.
  method: "password" | "trusted";
  bootstrapUser?: string;
  password?: string;
  // Route the bootstrap SSH connection through the jump host (for hosts the
  // backend can't reach directly but the jump host can).
  viaJump?: boolean;
}

// Enroll installs CA trust + WireGuard on the host (when bootstrapping), sets up
// the tunnel + jump-host peer, and validates per-user certificate login.
export async function enrollHost(id: string, params: EnrollParams): Promise<EnrollmentResult> {
  const { data } = await api.post<EnrollmentResult>(`/api/v1/hosts/${id}/enroll`, params);
  return data;
}
