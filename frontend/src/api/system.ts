import { api } from "./client";

export interface SchedulerStatus {
  name: string;
  runs: number;
  lastRunAt?: string;
  lastError?: string;
  ok: boolean;
}

export interface EnrollmentStep {
  name: string;
  status: string;
  detail?: string;
  timestamp: string;
}

export interface EnrollmentJob {
  id: string;
  target: string;
  status: string;
  error?: string;
  steps: EnrollmentStep[];
  createdAt: string;
  finishedAt?: string;
}

export interface JobsResponse {
  schedulers: SchedulerStatus[];
  enrollmentJobs: EnrollmentJob[];
}

export async function getJobs(): Promise<JobsResponse> {
  const { data } = await api.get<JobsResponse>("/api/v1/system/jobs");
  return data;
}
