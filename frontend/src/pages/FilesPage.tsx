import { useEffect, useRef, useState } from "react";
import { useParams } from "react-router-dom";
import {
  Alert, Box, Breadcrumbs, Button, Chip, IconButton, LinearProgress, Link, List,
  ListItem, ListItemButton, ListItemIcon, ListItemText, Paper, Stack, Typography,
} from "@mui/material";
import FolderIcon from "@mui/icons-material/Folder";
import InsertDriveFileIcon from "@mui/icons-material/InsertDriveFile";
import DownloadIcon from "@mui/icons-material/Download";
import UploadFileIcon from "@mui/icons-material/UploadFile";
import ArrowUpwardIcon from "@mui/icons-material/ArrowUpward";
import CloseIcon from "@mui/icons-material/Close";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { downloadFile, listDir, uploadFile } from "../api/sftp";
import { getHost } from "../api/hosts";

interface Transfer {
  id: string;
  name: string;
  dir: "up" | "down";
  loaded: number;
  total: number;
  status: "active" | "done" | "error";
}

// SFTP file browser. All transfers are brokered by the backend through the SSH
// gateway and audited; the browser never speaks SFTP directly. Uploads stream
// from disk and downloads stream to disk (File System Access API) so large files
// never sit in memory; both show progress.
export function FilesPage() {
  const { hostId } = useParams<{ hostId: string }>();
  const qc = useQueryClient();
  const [path, setPath] = useState(".");
  const [dragOver, setDragOver] = useState(false);
  const [transfers, setTransfers] = useState<Transfer[]>([]);
  const fileInput = useRef<HTMLInputElement | null>(null);

  const { data: host } = useQuery({
    queryKey: ["host", hostId],
    queryFn: () => getHost(hostId!),
    enabled: !!hostId,
  });
  useEffect(() => {
    if (host?.hostname) document.title = `Files · ${host.hostname} — Fleet Terminal`;
  }, [host?.hostname]);

  const { data, isLoading, error } = useQuery({
    queryKey: ["sftp", hostId, path],
    queryFn: () => listDir(hostId!, path),
    enabled: !!hostId,
  });

  const resolved = data?.path ?? path;

  const addTransfer = (name: string, dir: "up" | "down"): string => {
    const id = `${Date.now()}-${Math.round(Math.random() * 1e6)}`;
    setTransfers((t) => [{ id, name, dir, loaded: 0, total: 0, status: "active" }, ...t]);
    return id;
  };
  const update = (id: string, loaded: number, total: number) =>
    setTransfers((t) => t.map((x) => (x.id === id ? { ...x, loaded, total } : x)));
  const finish = (id: string, status: "done" | "error") =>
    setTransfers((t) => t.map((x) => (x.id === id ? { ...x, status } : x)));

  const startUpload = async (file: File) => {
    const id = addTransfer(file.name, "up");
    try {
      await uploadFile(hostId!, resolved, file, (l, tot) => update(id, l, tot));
      finish(id, "done");
      void qc.invalidateQueries({ queryKey: ["sftp", hostId] });
    } catch {
      finish(id, "error");
    }
  };

  const startDownload = async (remote: string, name: string) => {
    const id = addTransfer(name, "down");
    try {
      await downloadFile(hostId!, remote, (l, tot) => update(id, l, tot));
      finish(id, "done");
    } catch {
      finish(id, "error");
    }
  };

  const goUp = () => setPath(resolved.replace(/\/[^/]+\/?$/, "") || "/");
  const enter = (name: string) => setPath(resolved.replace(/\/$/, "") + "/" + name);

  const onFiles = (files: FileList | null) => {
    if (!files) return;
    Array.from(files).forEach((f) => void startUpload(f));
  };

  return (
    <Box
      sx={{ p: 3, minHeight: "100vh" }}
      onDragOver={(e) => { e.preventDefault(); setDragOver(true); }}
      onDragLeave={() => setDragOver(false)}
      onDrop={(e) => { e.preventDefault(); setDragOver(false); onFiles(e.dataTransfer.files); }}
    >
      <Stack direction="row" alignItems="center" justifyContent="space-between" sx={{ mb: 2 }}>
        <Typography variant="h5">Files{host?.hostname ? ` · ${host.hostname}` : ""}</Typography>
        <Stack direction="row" spacing={1}>
          <Button startIcon={<ArrowUpwardIcon />} onClick={goUp} size="small">Up</Button>
          <Button
            startIcon={<UploadFileIcon />} variant="contained" size="small"
            onClick={() => fileInput.current?.click()}
          >
            Upload
          </Button>
          <input
            ref={fileInput} type="file" multiple hidden
            onChange={(e) => onFiles(e.target.files)}
          />
        </Stack>
      </Stack>

      <Breadcrumbs sx={{ mb: 1 }}>
        <Link component="button" onClick={() => setPath("/")}>/</Link>
        <Typography color="text.primary">{resolved}</Typography>
      </Breadcrumbs>

      {error && <Alert severity="error">{(error as Error).message}</Alert>}

      {transfers.length > 0 && (
        <Paper variant="outlined" sx={{ p: 1.5, mb: 2 }}>
          <Stack direction="row" alignItems="center" sx={{ mb: 1 }}>
            <Typography variant="subtitle2" sx={{ flexGrow: 1 }}>Transfers</Typography>
            <IconButton size="small" onClick={() => setTransfers((t) => t.filter((x) => x.status === "active"))}>
              <CloseIcon fontSize="small" />
            </IconButton>
          </Stack>
          <Stack spacing={1}>
            {transfers.map((t) => {
              const pct = t.total > 0 ? Math.round((t.loaded / t.total) * 100) : 0;
              return (
                <Box key={t.id}>
                  <Stack direction="row" spacing={1} alignItems="center">
                    <Chip size="small" label={t.dir === "up" ? "↑" : "↓"} />
                    <Typography variant="body2" sx={{ flexGrow: 1 }} noWrap>{t.name}</Typography>
                    <Typography variant="caption" color="text.secondary">
                      {t.status === "active"
                        ? `${formatBytes(t.loaded)}${t.total ? " / " + formatBytes(t.total) : ""}${t.total ? ` (${pct}%)` : ""}`
                        : t.status === "done" ? "done" : "failed"}
                    </Typography>
                  </Stack>
                  <LinearProgress
                    variant={t.status === "active" && t.total > 0 ? "determinate" : t.status === "active" ? "indeterminate" : "determinate"}
                    value={t.status === "done" ? 100 : pct}
                    color={t.status === "error" ? "error" : t.status === "done" ? "success" : "primary"}
                    sx={{ mt: 0.5, height: 6, borderRadius: 3 }}
                  />
                </Box>
              );
            })}
          </Stack>
        </Paper>
      )}

      <Paper
        variant="outlined"
        sx={{ borderStyle: dragOver ? "dashed" : "solid", borderColor: dragOver ? "primary.main" : undefined }}
      >
        <List dense>
          {isLoading && <ListItem><ListItemText primary="Loading…" /></ListItem>}
          {data?.entries.map((e) => (
            <ListItem
              key={e.name}
              secondaryAction={
                !e.isDir && (
                  <IconButton edge="end" onClick={() => void startDownload(resolved.replace(/\/$/, "") + "/" + e.name, e.name)}>
                    <DownloadIcon />
                  </IconButton>
                )
              }
              disablePadding
            >
              <ListItemButton onClick={() => e.isDir && enter(e.name)} disabled={!e.isDir}>
                <ListItemIcon sx={{ minWidth: 36 }}>
                  {e.isDir ? <FolderIcon color="primary" /> : <InsertDriveFileIcon />}
                </ListItemIcon>
                <ListItemText
                  primary={e.name}
                  secondary={`${e.mode}  ${e.isDir ? "" : formatBytes(e.size)}`}
                />
              </ListItemButton>
            </ListItem>
          ))}
          {data && data.entries.length === 0 && (
            <ListItem><ListItemText primary="(empty directory)" /></ListItem>
          )}
        </List>
      </Paper>
      <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: "block" }}>
        Drag files here to upload. All transfers are audited.
      </Typography>
    </Box>
  );
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let v = n / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
  return `${v.toFixed(1)} ${units[i]}`;
}
