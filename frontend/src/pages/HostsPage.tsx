import { useMemo, useState } from "react";
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
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  createHost, deleteHost, listHosts, type Host, type HostInput,
} from "../api/hosts";

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
            <TextField label="Address" value={form.address} onChange={set("address")} fullWidth />
            <TextField label="WireGuard Address" value={form.wgAddress} onChange={set("wgAddress")} fullWidth />
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

  const createMut = useMutation({
    mutationFn: createHost,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["hosts"] });
      setDialogOpen(false);
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
      field: "actions", headerName: "Actions", width: 90, sortable: false, filterable: false,
      renderCell: (params) => (
        <Tooltip title="Delete host">
          <IconButton
            size="small" color="error"
            onClick={() => deleteMut.mutate([params.row.id])}
          >
            <DeleteIcon fontSize="small" />
          </IconButton>
        </Tooltip>
      ),
    },
  ], [deleteMut]);

  const rows = data?.hosts ?? [];

  return (
    <Box>
      <Typography variant="h5" gutterBottom>
        Host Inventory
      </Typography>
      <Box sx={{ width: "100%" }}>
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
    </Box>
  );
}
