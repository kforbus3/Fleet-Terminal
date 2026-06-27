import { api, getAccessToken } from "./client";

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

export type ProgressFn = (loaded: number, total: number) => void;

export async function listDir(hostId: string, path: string): Promise<SftpListing> {
  const { data } = await api.get<SftpListing>(`/api/v1/hosts/${hostId}/sftp/list`, {
    params: { path },
  });
  return data;
}

// uploadFile streams a file to the host with upload progress. The browser reads
// the file from disk as it sends, so arbitrarily large files are supported.
export async function uploadFile(
  hostId: string,
  dir: string,
  file: File,
  onProgress?: ProgressFn,
): Promise<void> {
  await api.post(`/api/v1/hosts/${hostId}/sftp/upload`, file, {
    params: { path: dir, name: file.name },
    headers: { "Content-Type": "application/octet-stream" },
    onUploadProgress: (e) => onProgress?.(e.loaded, e.total ?? file.size),
  });
}

interface SavePickerWindow {
  showSaveFilePicker?: (opts: { suggestedName?: string }) => Promise<FileSystemFileHandle>;
}

// downloadFile streams a file from the host with progress. When the File System
// Access API is available (Chromium), it streams straight to disk so multi-GB
// files never sit in browser memory; otherwise it falls back to a Blob.
export async function downloadFile(
  hostId: string,
  path: string,
  onProgress?: ProgressFn,
): Promise<void> {
  const token = getAccessToken();
  const url = `/api/v1/hosts/${hostId}/sftp/download?path=${encodeURIComponent(path)}`;
  const res = await fetch(url, {
    headers: token ? { Authorization: `Bearer ${token}` } : {},
    credentials: "include",
  });
  if (!res.ok || !res.body) {
    throw new Error(`download failed: ${res.status}`);
  }
  const filename = path.split("/").pop() ?? "download";
  const total = Number(res.headers.get("Content-Length") ?? 0);
  const reader = res.body.getReader();
  let received = 0;

  const picker = (window as unknown as SavePickerWindow).showSaveFilePicker;
  if (picker) {
    let handle: FileSystemFileHandle;
    try {
      handle = await picker({ suggestedName: filename });
    } catch {
      await reader.cancel();
      return; // user cancelled the save dialog
    }
    const writable = await handle.createWritable();
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      await writable.write(value);
      received += value.byteLength;
      onProgress?.(received, total);
    }
    await writable.close();
    return;
  }

  // Fallback: accumulate to a Blob (memory-bound; fine for moderate files).
  const chunks: BlobPart[] = [];
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    chunks.push(value);
    received += value.byteLength;
    onProgress?.(received, total);
  }
  const blobUrl = URL.createObjectURL(new Blob(chunks));
  const a = document.createElement("a");
  a.href = blobUrl;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(blobUrl);
}
