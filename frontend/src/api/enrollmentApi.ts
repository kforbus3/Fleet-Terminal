import { api } from "./client";

export interface EnrollmentStep {
  name: string;
  status: string;
  detail?: string;
  timestamp: string;
}

export interface EnrollmentJob {
  id: string;
  hostId?: string;
  target: string;
  status: string;
  steps: EnrollmentStep[];
  error?: string;
  createdAt: string;
  startedAt?: string;
  finishedAt?: string;
}

export async function listEnrollmentJobs(limit = 100): Promise<EnrollmentJob[]> {
  const { data } = await api.get<{ jobs: EnrollmentJob[] }>(`/api/v1/enrollment/jobs?limit=${limit}`);
  return data.jobs ?? [];
}
