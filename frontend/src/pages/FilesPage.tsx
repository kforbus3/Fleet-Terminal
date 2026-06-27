import { useRef, useState } from "react";
import { useParams } from "react-router-dom";
import {
  Alert, Box, Breadcrumbs, Button, IconButton, Link, List, ListItem,
  ListItemButton, ListItemIcon, ListItemText, Paper, Stack, Typography,
} from "@mui/material";
import FolderIcon from "@mui/icons-material/Folder";
import InsertDriveFileIcon from "@mui/icons-material/InsertDriveFile";
import DownloadIcon from "@mui/icons-material/Download";
import UploadFileIcon from "@mui/icons-material/UploadFile";
import ArrowUpwardIcon from "@mui/icons-material/ArrowUpward";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { downloadFile, listDir, uploadFile } from "../api/sftp";

// SFTP file browser. All transfers are brokered by the backend through the SSH
// gateway and audited; the browser never speaks SFTP directly.
export function FilesPage() {
  const { hostId } = useParams<{ hostId: string }>();
  const qc = useQueryClient();
  const [path, setPath] = useState(".");
  const [dragOver, setDragOver] = useState(false);
  const fileInput = useRef<HTMLInputElement | null>(null);

  const { data, isLoading, error } = useQuery({
    queryKey: ["sftp", hostId, path],
    queryFn: () => listDir(hostId!, path),
    enabled: !!hostId,
  });

  const resolved = data?.path ?? path;

  const uploadMut = useMutation({
    mutationFn: (file: File) => uploadFile(hostId!, resolved, file),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["sftp", hostId] }),
  });

  const goUp = () => setPath(resolved.replace(/\/[^/]+\/?$/, "") || "/");
  const enter = (name: string) => setPath(resolved.replace(/\/$/, "") + "/" + name);

  const onFiles = (files: FileList | null) => {
    if (!files) return;
    Array.from(files).forEach((f) => uploadMut.mutate(f));
  };

  return (
    <Box
      onDragOver={(e) => { e.preventDefault(); setDragOver(true); }}
      onDragLeave={() => setDragOver(false)}
      onDrop={(e) => { e.preventDefault(); setDragOver(false); onFiles(e.dataTransfer.files); }}
    >
      <Stack direction="row" alignItems="center" justifyContent="space-between" sx={{ mb: 2 }}>
        <Typography variant="h5">Files</Typography>
        <Stack direction="row" spacing={1}>
          <Button startIcon={<ArrowUpwardIcon />} onClick={goUp} size="small">Up</Button>
          <Button
            startIcon={<UploadFileIcon />} variant="contained" size="small"
            onClick={() => fileInput.current?.click()}
            disabled={uploadMut.isPending}
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
      {uploadMut.isError && <Alert severity="error">Upload failed.</Alert>}

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
                  <IconButton edge="end" onClick={() => downloadFile(hostId!, resolved.replace(/\/$/, "") + "/" + e.name)}>
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
