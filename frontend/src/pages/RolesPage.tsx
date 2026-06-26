import { useEffect, useState } from "react";
import {
  Box, Button, Checkbox, Dialog, DialogActions, DialogContent, DialogTitle,
  FormControlLabel, FormGroup, IconButton, Paper, Stack, Table, TableBody,
  TableCell, TableContainer, TableHead, TableRow, TextField, Tooltip, Typography,
  Chip,
} from "@mui/material";
import AddIcon from "@mui/icons-material/Add";
import DeleteIcon from "@mui/icons-material/Delete";
import SecurityIcon from "@mui/icons-material/Security";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  createRole, deleteRole, listPermissions, listRoles, setRolePermissions,
  type Role,
} from "../api/admin";

// Role administration with a permission-checkbox editor. The editor seeds its
// selection from the role's current permissions and persists via PUT.
export function RolesPage() {
  const qc = useQueryClient();
  const { data: roles = [] } = useQuery({ queryKey: ["roles"], queryFn: listRoles });
  const { data: permissions = [] } = useQuery({ queryKey: ["permissions"], queryFn: listPermissions });

  const [createOpen, setCreateOpen] = useState(false);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [editing, setEditing] = useState<Role | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());

  useEffect(() => {
    if (editing) setSelected(new Set(editing.permissions ?? []));
  }, [editing]);

  const invalidate = () => qc.invalidateQueries({ queryKey: ["roles"] });

  const createMut = useMutation({
    mutationFn: () => createRole(name, description),
    onSuccess: () => { setCreateOpen(false); setName(""); setDescription(""); invalidate(); },
  });
  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteRole(id),
    onSuccess: invalidate,
  });
  const permsMut = useMutation({
    mutationFn: (role: Role) => setRolePermissions(role.id, Array.from(selected)),
    onSuccess: () => { setEditing(null); invalidate(); },
  });

  const toggle = (key: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key); else next.add(key);
      return next;
    });
  };

  return (
    <Box>
      <Stack direction="row" alignItems="center" sx={{ mb: 2 }}>
        <Typography variant="h5" sx={{ flexGrow: 1 }}>Role Management</Typography>
        <Button startIcon={<AddIcon />} variant="contained" onClick={() => setCreateOpen(true)}>
          New Role
        </Button>
      </Stack>

      <TableContainer component={Paper} variant="outlined">
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>Name</TableCell>
              <TableCell>Description</TableCell>
              <TableCell>Permissions</TableCell>
              <TableCell align="right">Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {roles.map((role) => (
              <TableRow key={role.id} hover>
                <TableCell>{role.name}{role.isBuiltin && (
                  <Chip label="builtin" size="small" sx={{ ml: 1 }} />
                )}</TableCell>
                <TableCell>{role.description}</TableCell>
                <TableCell>{(role.permissions ?? []).length}</TableCell>
                <TableCell align="right">
                  <Tooltip title="Edit permissions">
                    <IconButton size="small" onClick={() => setEditing(role)}><SecurityIcon fontSize="small" /></IconButton>
                  </Tooltip>
                  <Tooltip title="Delete">
                    <span>
                      <IconButton size="small" disabled={role.isBuiltin}
                        onClick={() => deleteMut.mutate(role.id)}><DeleteIcon fontSize="small" /></IconButton>
                    </span>
                  </Tooltip>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>

      <Dialog open={createOpen} onClose={() => setCreateOpen(false)} fullWidth maxWidth="sm">
        <DialogTitle>New Role</DialogTitle>
        <DialogContent>
          <Stack spacing={2} sx={{ mt: 1 }}>
            <TextField label="Name" value={name} onChange={(e) => setName(e.target.value)} required />
            <TextField label="Description" value={description}
              onChange={(e) => setDescription(e.target.value)} />
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setCreateOpen(false)}>Cancel</Button>
          <Button variant="contained" disabled={!name || createMut.isPending}
            onClick={() => createMut.mutate()}>Create</Button>
        </DialogActions>
      </Dialog>

      <Dialog open={editing !== null} onClose={() => setEditing(null)} fullWidth maxWidth="md">
        <DialogTitle>{editing ? `Permissions · ${editing.name}` : "Permissions"}</DialogTitle>
        <DialogContent>
          <FormGroup sx={{ mt: 1, display: "grid", gridTemplateColumns: "1fr 1fr" }}>
            {permissions.map((perm) => (
              <FormControlLabel
                key={perm.key}
                control={<Checkbox checked={selected.has(perm.key)} onChange={() => toggle(perm.key)} />}
                label={
                  <span>
                    {perm.key}
                    {perm.description && (
                      <Typography component="span" variant="caption" color="text.secondary" sx={{ ml: 1 }}>
                        {perm.description}
                      </Typography>
                    )}
                  </span>
                }
              />
            ))}
          </FormGroup>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setEditing(null)}>Cancel</Button>
          <Button variant="contained" disabled={permsMut.isPending}
            onClick={() => editing && permsMut.mutate(editing)}>Save</Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
