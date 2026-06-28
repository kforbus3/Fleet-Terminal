import axios from "axios";

// Single axios instance. Credentials are sent so the HttpOnly refresh cookie
// flows automatically; the access token is attached by an interceptor once auth
// is wired (M2). The base URL is same-origin in dev (Vite proxy) and prod (nginx).
export const api = axios.create({
  baseURL: import.meta.env.VITE_API_BASE ?? "",
  withCredentials: true,
});

let accessToken: string | null = null;

export function setAccessToken(token: string | null) {
  accessToken = token;
}

// getAccessToken exposes the in-memory token for streamed fetch() calls (large
// SFTP transfers) that bypass the axios instance.
export function getAccessToken(): string | null {
  return accessToken;
}

api.interceptors.request.use((cfg) => {
  if (accessToken) {
    cfg.headers.Authorization = `Bearer ${accessToken}`;
  }
  return cfg;
});

export interface VersionInfo {
  version: string;
  environment?: string;
  appName?: string;
}

export async function getVersion(): Promise<VersionInfo> {
  const { data } = await api.get<VersionInfo>("/version");
  return data;
}
