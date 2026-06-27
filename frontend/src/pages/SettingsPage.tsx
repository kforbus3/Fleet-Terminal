import { useState } from "react";
import {
  Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, IconButton,
  Paper, Stack, Table, TableBody, TableCell, TableContainer, TableHead,
  TableRow, TextField, Tooltip, Typography, Alert,
} from "@mui/material";
import EditIcon from "@mui/icons-material/Edit";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { listSettings, setSetting } from "../api/admin";
import { downloadBackup } from "../api/system";

// System settings editor. Values are arbitrary JSON; the editor exposes them as
// raw JSON text and validates before PUTting the new value.
export function SettingsPage() {
  const qc = useQueryClient();
  const { data: settings = {}, isLoading } = useQuery({ queryKey: ["settings"], queryFn: listSettings });

  const [editKey, setEditKey] = useState<string | null>(null);
  const [draft, setDraft] = useState("");
  const [error, setError] = useState<string | null>(null);

  const openEdit = (key: string, value: unknown) => {
    setEditKey(key);
    setDraft(JSON.stringify(value, null, 2));
    setError(null);
  };

  const saveMut = useMutation({
    mutationFn: (parsed: unknown) => setSetting(editKey as string, parsed),
    onSuccess: () => { setEditKey(null); qc.invalidateQueries({ queryKey: ["settings"] }); },
  });

  const onSave = () => {
    let parsed: unknown;
    try {
      parsed = JSON.parse(draft);
    } catch {
      setError("Value must be valid JSON");
      return;
    }
    setError(null);
    saveMut.mutate(parsed);
  };

  const entries = Object.entries(settings);

  return (
    <Box>
      <Typography variant="h5" sx={{ mb: 2 }}>System Settings</Typography>

      <BackupCard />

      <TableContainer component={Paper} variant="outlined">
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>Key</TableCell>
              <TableCell>Value</TableCell>
              <TableCell align="right">Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {entries.map(([key, value]) => (
              <TableRow key={key} hover>
                <TableCell sx={{ fontFamily: "monospace" }}>{key}</TableCell>
                <TableCell sx={{ fontFamily: "monospace", whiteSpace: "pre-wrap" }}>
                  {JSON.stringify(value)}
                </TableCell>
                <TableCell align="right">
                  <Tooltip title="Edit">
                    <IconButton size="small" onClick={() => openEdit(key, value)}><EditIcon fontSize="small" /></IconButton>
                  </Tooltip>
                </TableCell>
              </TableRow>
            ))}
            {!isLoading && entries.length === 0 && (
              <TableRow><TableCell colSpan={3}>
                <Typography color="text.secondary">No settings configured.</Typography>
              </TableCell></TableRow>
            )}
          </TableBody>
        </Table>
      </TableContainer>

      <Dialog open={editKey !== null} onClose={() => setEditKey(null)} fullWidth maxWidth="sm">
        <DialogTitle>{editKey ? `Edit · ${editKey}` : "Edit"}</DialogTitle>
        <DialogContent>
          <Stack spacing={2} sx={{ mt: 1 }}>
            {error && <Alert severity="error">{error}</Alert>}
            <TextField label="Value (JSON)" value={draft} multiline minRows={4}
              onChange={(e) => setDraft(e.target.value)}
              sx={{ "& textarea": { fontFamily: "monospace" } }} />
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setEditKey(null)}>Cancel</Button>
          <Button variant="contained" disabled={saveMut.isPending} onClick={onSave}>Save</Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}

// BackupCard downloads a logical database backup. Restore is documented as an
// out-of-band operation in the disaster-recovery guide.
function BackupCard() {
  const backupMut = useMutation({ mutationFn: downloadBackup });
  return (
    <Paper variant="outlined" sx={{ p: 2, mb: 3 }}>
      <Typography variant="h6">Backup &amp; Restore</Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5, mb: 1.5 }}>
        Download a full logical database backup (pg_dump). Restore is performed offline:
        <code> psql "$FLEET_DATABASE_URL" &lt; fleet-backup.sql</code> — see the disaster-recovery guide.
      </Typography>
      {backupMut.isError && <Alert severity="error" sx={{ mb: 1 }}>Backup failed.</Alert>}
      <Button variant="contained" onClick={() => backupMut.mutate()} disabled={backupMut.isPending}>
        {backupMut.isPending ? "Preparing…" : "Download backup"}
      </Button>
    </Paper>
  );
}
