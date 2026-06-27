import { useEffect, useMemo, useState } from "react";
import {
  Box, Button, Chip, Dialog, DialogActions, DialogContent, DialogTitle,
  IconButton, Stack, TextField, Tooltip, Typography,
} from "@mui/material";
import {
  DataGrid, GridToolbarContainer, GridToolbarQuickFilter,
  type GridColDef, type GridRowSelectionModel,
} from "@mui/x-data-grid";
import AddIcon from "@mui/icons-material/Add";
import DeleteIcon from "@mui/icons-material/Delete";
import RefreshIcon from "@mui/icons-material/Refresh";
import CableIcon from "@mui/icons-material/Cable";
import TerminalIcon from "@mui/icons-material/Terminal";
import FolderIcon from "@mui/icons-material/Folder";
import { Alert, CircularProgress, List, ListItem, ListItemText } from "@mui/material";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  createHost, deleteHost, enrollHost, listHosts, nextWGAddress,
  type EnrollmentResult, type EnrollParams, type Host, type HostInput,
} from "../api/hosts";
import {
  Checkbox, FormControl, FormControlLabel, FormLabel, Radio, RadioGroup,
} from "@mui/material";

const STATUS_COLOR: Record<string, "success" | "error" | "warning" | "default"> = {
  online: "success",
  offline: "error",
  unknown: "warning",
};

const EMPTY_FORM: HostInput = {
  hostname: "", description: "", environment: "", owner: "",
  address: "", wgAddress: "", sshPort: 22, sshUser: "", tags: [],
};

function fmtDate(value?: string): string {
  if (!value) return "—";
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? "—" : d.toLocaleString();
}

// Toolbar combines quick search with the New Host action and a bulk-delete
// button that appears only while rows are selected.
interface ToolbarProps {
  selectedCount: number;
  onNew: () => void;
  onDelete: () => void;
  onRefresh: () => void;
}

// Teach the DataGrid slotProps about our custom toolbar's extra props.
declare module "@mui/x-data-grid" {
  interface ToolbarPropsOverrides extends ToolbarProps {}
}

function HostsToolbar({ selectedCount, onNew, onDelete, onRefresh }: ToolbarProps) {
  return (
    <GridToolbarContainer sx={{ p: 1, gap: 1 }}>
      <GridToolbarQuickFilter />
      <Box sx={{ flexGrow: 1 }} />
      {selectedCount > 0 && (
        <Button
          color="error"
          size="small"
          startIcon={<DeleteIcon />}
          onClick={onDelete}
        >
          Delete ({selectedCount})
        </Button>
      )}
      <Tooltip title="Refresh">
        <IconButton size="small" onClick={onRefresh}>
          <RefreshIcon fontSize="small" />
        </IconButton>
      </Tooltip>
      <Button size="small" variant="contained" startIcon={<AddIcon />} onClick={onNew}>
        New Host
      </Button>
    </GridToolbarContainer>
  );
}

// NewHostDialog collects the minimal create payload; tags are entered as a
// comma-separated list and normalized on submit.
interface NewHostDialogProps {
  open: boolean;
  onClose: () => void;
  onSubmit: (input: HostInput) => void;
  submitting: boolean;
}

function NewHostDialog({ open, onClose, onSubmit, submitting }: NewHostDialogProps) {
  const [form, setForm] = useState<HostInput>(EMPTY_FORM);
  const [tags, setTags] = useState("");

  // Show what auto-assignment would pick from the overlay pool.
  const { data: nextWG } = useQuery({
    queryKey: ["next-wg"],
    queryFn: nextWGAddress,
    enabled: open,
  });

  const set = (key: keyof HostInput) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm((f) => ({ ...f, [key]: e.target.value }));

  const handleSubmit = () => {
    onSubmit({
      ...form,
      sshPort: Number(form.sshPort) || 22,
      tags: tags.split(",").map((t) => t.trim()).filter(Boolean),
    });
  };

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>New Host</DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ mt: 1 }}>
          <TextField
            label="Hostname" required value={form.hostname}
            onChange={set("hostname")} autoFocus fullWidth
          />
          <TextField label="Description" value={form.description} onChange={set("description")} fullWidth />
          <Stack direction="row" spacing={2}>
            <TextField label="Environment" value={form.environment} onChange={set("environment")} fullWidth />
            <TextField label="Owner" value={form.owner} onChange={set("owner")} fullWidth />
          </Stack>
          <Stack direction="row" spacing={2}>
            <TextField
              label="Address" value={form.address} onChange={set("address")} fullWidth
              helperText="Management address used to reach the host during enrollment"
            />
            <TextField
              label="WireGuard Address" value={form.wgAddress} onChange={set("wgAddress")} fullWidth
              placeholder={nextWG?.nextWgAddress ? `auto: ${nextWG.nextWgAddress}` : "auto-assign"}
              helperText={
                nextWG?.exhausted
                  ? `Overlay pool ${nextWG.subnet} exhausted`
                  : nextWG?.nextWgAddress
                    ? `Leave blank to auto-assign ${nextWG.nextWgAddress} from ${nextWG.subnet}`
                    : "Leave blank to auto-assign from the overlay pool"
              }
            />
          </Stack>
          <Stack direction="row" spacing={2}>
            <TextField
              label="SSH Port" type="number" value={form.sshPort}
              onChange={set("sshPort")} sx={{ width: 140 }}
            />
            <TextField label="SSH User" value={form.sshUser} onChange={set("sshUser")} fullWidth />
          </Stack>
          <TextField
            label="Tags" helperText="Comma-separated" value={tags}
            onChange={(e) => setTags(e.target.value)} fullWidth
          />
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        <Button
          variant="contained" onClick={handleSubmit}
          disabled={submitting || form.hostname.trim() === ""}
        >
          Create
        </Button>
      </DialogActions>
    </Dialog>
  );
}

// Host inventory grid: searchable, sortable, paginated, with multi-select bulk
// delete and a create dialog. Theme (light/dark) is inherited from the provider.
export function HostsPage() {
  const qc = useQueryClient();
  const { data, isLoading, refetch } = useQuery({
    queryKey: ["hosts"],
    queryFn: listHosts,
  });

  const [selection, setSelection] = useState<GridRowSelectionModel>([]);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [enrollResult, setEnrollResult] = useState<EnrollmentResult | null>(null);
  const [enrollError, setEnrollError] = useState<string | null>(null);
  const [enrollOpen, setEnrollOpen] = useState(false);
  const [enrollTarget, setEnrollTarget] = useState<Host | null>(null);

  const createMut = useMutation({
    mutationFn: createHost,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["hosts"] });
      setDialogOpen(false);
    },
  });

  const enrollMut = useMutation({
    mutationFn: ({ id, params }: { id: string; params: EnrollParams }) => enrollHost(id, params),
    onMutate: () => {
      setEnrollTarget(null);
      setEnrollOpen(true);
      setEnrollResult(null);
      setEnrollError(null);
    },
    onSuccess: (res) => {
      setEnrollResult(res);
      void qc.invalidateQueries({ queryKey: ["hosts"] });
    },
    onError: (err: unknown) => {
      const e = err as { response?: { data?: { error?: string } } };
      setEnrollError(e.response?.data?.error ?? "Enrollment failed.");
    },
  });

  const deleteMut = useMutation({
    mutationFn: async (ids: string[]) => {
      await Promise.all(ids.map((id) => deleteHost(id)));
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["hosts"] });
      setSelection([]);
    },
  });

  const columns = useMemo<GridColDef<Host>[]>(() => [
    { field: "hostname", headerName: "Hostname", minWidth: 160, flex: 1 },
    { field: "description", headerName: "Description", minWidth: 180, flex: 1 },
    { field: "environment", headerName: "Environment", minWidth: 130 },
    { field: "owner", headerName: "Owner", minWidth: 130 },
    { field: "address", headerName: "Address", minWidth: 140 },
    { field: "wgAddress", headerName: "WG Address", minWidth: 140 },
    {
      field: "sshVersion", headerName: "SSH Version", minWidth: 130,
      valueGetter: (_v, row) => row.inventory?.sshVersion ?? "",
    },
    {
      field: "status", headerName: "Status", minWidth: 120, sortable: true,
      valueGetter: (_v, row) => row.status?.status ?? "unknown",
      renderCell: (params) => {
        const s = String(params.value ?? "unknown");
        return <Chip size="small" label={s} color={STATUS_COLOR[s] ?? "default"} />;
      },
    },
    {
      field: "latency", headerName: "Latency", minWidth: 110, type: "number",
      valueGetter: (_v, row) => row.status?.latencyMs ?? null,
      valueFormatter: (value) => (value == null ? "—" : `${value} ms`),
    },
    {
      field: "tags", headerName: "Tags", minWidth: 180, flex: 1, sortable: false,
      valueGetter: (_v, row) => (row.tags ?? []).join(", "),
      renderCell: (params) => (
        <Stack direction="row" spacing={0.5} sx={{ flexWrap: "wrap", py: 0.5 }}>
          {(params.row.tags ?? []).map((t) => (
            <Chip key={t} size="small" label={t} variant="outlined" />
          ))}
        </Stack>
      ),
    },
    {
      field: "groups", headerName: "Groups", minWidth: 160, sortable: false,
      valueGetter: (_v, row) => (row.groups ?? []).join(", "),
      renderCell: (params) => (
        <Stack direction="row" spacing={0.5} sx={{ flexWrap: "wrap", py: 0.5 }}>
          {(params.row.groups ?? []).map((g) => (
            <Chip key={g} size="small" label={g} />
          ))}
        </Stack>
      ),
    },
    {
      field: "lastSeen", headerName: "Last Seen", minWidth: 180,
      valueGetter: (_v, row) => row.status?.checkedAt ?? "",
      valueFormatter: (value) => fmtDate(value ? String(value) : undefined),
    },
    {
      field: "actions", headerName: "Actions", width: 190, sortable: false, filterable: false,
      renderCell: (params) => (
        <Stack direction="row" spacing={0.5}>
          <Tooltip title="Open terminal in a new tab">
            <IconButton
              size="small" color="primary"
              onClick={() => window.open(`/terminals/${params.row.id}`, "_blank", "noopener")}
            >
              <TerminalIcon fontSize="small" />
            </IconButton>
          </Tooltip>
          <Tooltip title="Browse files (SFTP) in a new tab">
            <IconButton
              size="small"
              onClick={() => window.open(`/files/${params.row.id}`, "_blank", "noopener")}
            >
              <FolderIcon fontSize="small" />
            </IconButton>
          </Tooltip>
          <Tooltip title={params.row.enrolled ? "Re-enroll (provision WireGuard)" : "Enroll (provision WireGuard)"}>
            <span>
              <IconButton
                size="small"
                color={params.row.enrolled ? "success" : "primary"}
                disabled={enrollMut.isPending}
                onClick={() => setEnrollTarget(params.row)}
              >
                <CableIcon fontSize="small" />
              </IconButton>
            </span>
          </Tooltip>
          <Tooltip title="Delete host">
            <IconButton
              size="small" color="error"
              onClick={() => deleteMut.mutate([params.row.id])}
            >
              <DeleteIcon fontSize="small" />
            </IconButton>
          </Tooltip>
        </Stack>
      ),
    },
  ], [deleteMut, enrollMut]);

  const rows = data?.hosts ?? [];

  return (
    <Box>
      <Typography variant="h5" gutterBottom>
        Host Inventory
      </Typography>
      <Box sx={{ width: "100%", height: "calc(100vh - 180px)" }}>
        <DataGrid<Host>
          rows={rows}
          columns={columns}
          loading={isLoading || deleteMut.isPending}
          getRowId={(row) => row.id}
          checkboxSelection
          disableRowSelectionOnClick
          rowSelectionModel={selection}
          onRowSelectionModelChange={setSelection}
          pageSizeOptions={[10, 25, 50, 100]}
          initialState={{
            pagination: { paginationModel: { pageSize: 25, page: 0 } },
            // Keep the default view simple: the essentials for finding a host and
            // connecting. The rest are one click away in the column menu.
            columns: {
              columnVisibilityModel: {
                owner: false, wgAddress: false, sshVersion: false,
                latency: false, groups: false,
              },
            },
          }}
          getRowHeight={() => "auto"}
          slots={{ toolbar: HostsToolbar }}
          slotProps={{
            toolbar: {
              selectedCount: selection.length,
              onNew: () => setDialogOpen(true),
              onDelete: () => deleteMut.mutate(selection.map(String)),
              onRefresh: () => void refetch(),
            },
          }}
          sx={{ "& .MuiDataGrid-cell": { alignItems: "flex-start", py: 0.5 } }}
        />
      </Box>
      <NewHostDialog
        open={dialogOpen}
        onClose={() => setDialogOpen(false)}
        onSubmit={(input) => createMut.mutate(input)}
        submitting={createMut.isPending}
      />
      <EnrollCredsDialog
        host={enrollTarget}
        onClose={() => setEnrollTarget(null)}
        onSubmit={(params) => enrollTarget && enrollMut.mutate({ id: enrollTarget.id, params })}
      />
      <EnrollDialog
        open={enrollOpen}
        pending={enrollMut.isPending}
        result={enrollResult}
        error={enrollError}
        onClose={() => setEnrollOpen(false)}
      />
    </Box>
  );
}

// EnrollCredsDialog collects how to reach the host for the initial bootstrap:
// an SSH password (installs CA trust + WireGuard on a brand-new host), or the
// session certificate when the host already trusts the Fleet CA.
function EnrollCredsDialog({
  host, onClose, onSubmit,
}: {
  host: Host | null;
  onClose: () => void;
  onSubmit: (params: EnrollParams) => void;
}) {
  const [method, setMethod] = useState<"password" | "trusted">("password");
  const [bootstrapUser, setBootstrapUser] = useState("root");
  const [password, setPassword] = useState("");
  const [sudoPassword, setSudoPassword] = useState("");
  const [wgEndpoint, setWgEndpoint] = useState("");
  const [viaJump, setViaJump] = useState(false);

  // Pre-fill the jump host's WireGuard endpoint with the configured default.
  const { data: nextWG } = useQuery({ queryKey: ["next-wg"], queryFn: nextWGAddress, enabled: Boolean(host) });
  useEffect(() => {
    if (nextWG?.jumpEndpoint && wgEndpoint === "") setWgEndpoint(nextWG.jumpEndpoint);
  }, [nextWG?.jumpEndpoint, wgEndpoint]);

  return (
    <Dialog open={Boolean(host)} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>Enroll {host?.hostname}</DialogTitle>
      <DialogContent>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 1, mb: 2 }}>
          Enrollment installs the SSH CA trust and WireGuard on the host, joins it to the
          VPN overlay, and verifies per-user certificate login through the jump host.
        </Typography>
        <FormControl sx={{ mb: 2 }}>
          <FormLabel>How should we reach this host first?</FormLabel>
          <RadioGroup value={method} onChange={(e) => setMethod(e.target.value as "password" | "trusted")}>
            <FormControlLabel
              value="password" control={<Radio />}
              label="SSH password — install everything (new/existing host with no setup)"
            />
            <FormControlLabel
              value="trusted" control={<Radio />}
              label="Host already trusts the Fleet CA (re-provision only)"
            />
          </RadioGroup>
        </FormControl>
        {method === "password" && (
          <Stack spacing={2}>
            <TextField
              label="SSH user" value={bootstrapUser}
              onChange={(e) => setBootstrapUser(e.target.value)}
              helperText="A user with sudo, or root"
            />
            <TextField
              label="SSH password" type="password" value={password}
              onChange={(e) => setPassword(e.target.value)} autoFocus
              helperText="Used once to bootstrap trust; never stored"
            />
            <TextField
              label="Sudo password (optional)" type="password" value={sudoPassword}
              onChange={(e) => setSudoPassword(e.target.value)}
              helperText="Only if this user's sudo requires a password different from the SSH password"
            />
          </Stack>
        )}
        <TextField
          fullWidth sx={{ mt: 2 }}
          label="Jump host WireGuard endpoint" value={wgEndpoint}
          onChange={(e) => setWgEndpoint(e.target.value)}
          helperText="Public address:port the HOST uses to reach the VPN server (e.g. vpn.example.com:51820). Must be resolvable from the host — not an internal Docker name."
        />
        <FormControlLabel
          sx={{ mt: 1 }}
          control={<Checkbox checked={viaJump} onChange={(e) => setViaJump(e.target.checked)} />}
          label="Reach this host through the jump host (backend can't reach it directly)"
        />
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        <Button
          variant="contained"
          disabled={method === "password" && password === ""}
          onClick={() => onSubmit({ method, bootstrapUser, password, sudoPassword, wgEndpoint, viaJump })}
        >
          Enroll
        </Button>
      </DialogActions>
    </Dialog>
  );
}

// EnrollDialog streams the enrollment job's step results (provisioning the
// WireGuard peer on the jump host, the interface on the host, and verification).
interface EnrollDialogProps {
  open: boolean;
  pending: boolean;
  result: EnrollmentResult | null;
  error: string | null;
  onClose: () => void;
}

function EnrollDialog({ open, pending, result, error, onClose }: EnrollDialogProps) {
  const stepColor = (s: string) =>
    s === "ok" ? "success.main" : s === "failed" ? "error.main" : s === "warning" ? "warning.main" : "text.secondary";
  const stepIcon = (s: string) =>
    s === "ok" ? "✓" : s === "failed" ? "✗" : s === "warning" ? "⚠" : "•";
  const hasWarning = result?.job.steps.some((s) => s.status === "warning");
  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>Host enrollment</DialogTitle>
      <DialogContent dividers>
        {pending && (
          <Stack direction="row" spacing={2} alignItems="center" sx={{ py: 2 }}>
            <CircularProgress size={22} />
            <Typography>Provisioning WireGuard and trust over SSH…</Typography>
          </Stack>
        )}
        {error && <Alert severity="error">{error}</Alert>}
        {result && (
          <>
            <Alert severity={hasWarning ? "warning" : "success"} sx={{ mb: 2 }}>
              Enrolled. Overlay address <b>{result.wgAddress}</b>; interface up on the host.
              {hasWarning && " Connectivity warning — see steps below."}
            </Alert>
            <List dense>
              {result.job.steps.map((st, i) => (
                <ListItem key={i} disableGutters>
                  <ListItemText
                    primary={
                      <Typography sx={{ color: stepColor(st.status) }}>
                        {stepIcon(st.status)} {st.name}
                      </Typography>
                    }
                    secondary={st.detail}
                  />
                </ListItem>
              ))}
            </List>
          </>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={pending}>Close</Button>
      </DialogActions>
    </Dialog>
  );
}
