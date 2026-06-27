import { api } from "./client";

export interface SftpEntry {
  name: string;
  size: number;
  isDir: boolean;
  mode: string;
  modTime: string;
}

export interface SftpListing {
  path: string;
  entries: SftpEntry[];
}

export async function listDir(hostId: string, path: string): Promise<SftpListing> {
  const { data } = await api.get<SftpListing>(`/api/v1/hosts/${hostId}/sftp/list`, {
    params: { path },
  });
  return data;
}

// downloadFile streams the remote file and triggers a browser save.
export async function downloadFile(hostId: string, path: string): Promise<void> {
  const res = await api.get(`/api/v1/hosts/${hostId}/sftp/download`, {
    params: { path },
    responseType: "blob",
  });
  const url = URL.createObjectURL(res.data as Blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = path.split("/").pop() ?? "download";
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

export async function uploadFile(hostId: string, dir: string, file: File): Promise<void> {
  await api.post(`/api/v1/hosts/${hostId}/sftp/upload`, file, {
    params: { path: dir, name: file.name },
    headers: { "Content-Type": "application/octet-stream" },
  });
}
