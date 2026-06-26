import { api } from "./client";

// Mirrors backend models.User (json tags). Only fields surfaced by the API are
// declared; optional ones map to backend `omitempty`.
export interface User {
  id: string;
  username: string;
  email?: string;
  displayName: string;
  isSuperAdmin: boolean;
  isDisabled: boolean;
  emailVerified: boolean;
  mustChangePassword: boolean;
  lockedUntil?: string;
  lastLoginAt?: string;
  createdAt: string;
  updatedAt: string;
  roles?: string[];
  groups?: string[];
}

export interface LoginResponse {
  accessToken: string;
  accessExpiresAt: string;
  csrfToken: string;
  user: User;
  mustChangePassword: boolean;
}

export interface MeResponse {
  user: User;
  permissions: string[];
  isSuperAdmin: boolean;
}

export interface BootstrapStatus {
  bootstrapAvailable: boolean;
}

export interface BootstrapInitParams {
  username: string;
  email: string;
  displayName: string;
  password: string;
}

export interface BootstrapInitResponse {
  status: string;
  user: User;
}

export async function bootstrapStatus(): Promise<BootstrapStatus> {
  const { data } = await api.get<BootstrapStatus>("/api/v1/bootstrap/status");
  return data;
}

export async function bootstrapInit(
  params: BootstrapInitParams,
): Promise<BootstrapInitResponse> {
  const { data } = await api.post<BootstrapInitResponse>(
    "/api/v1/bootstrap/init",
    params,
  );
  return data;
}

export async function login(
  username: string,
  password: string,
): Promise<LoginResponse> {
  const { data } = await api.post<LoginResponse>("/api/v1/auth/login", {
    username,
    password,
  });
  return data;
}

export async function me(): Promise<MeResponse> {
  const { data } = await api.get<MeResponse>("/api/v1/auth/me");
  return data;
}

export async function logout(): Promise<void> {
  await api.post("/api/v1/auth/logout");
}

export async function changePassword(
  currentPassword: string,
  newPassword: string,
): Promise<void> {
  await api.post("/api/v1/auth/change-password", {
    currentPassword,
    newPassword,
  });
}
