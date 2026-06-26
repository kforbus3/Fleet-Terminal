import { useState } from "react";
import {
  Box, Button, Chip, Dialog, DialogActions, DialogContent, DialogTitle,
  FormControlLabel, IconButton, Stack, Switch, Table, TableBody, TableCell,
  TableContainer, TableHead, TableRow, TextField, Typography, Paper, Tooltip,
} from "@mui/material";
import AddIcon from "@mui/icons-material/Add";
import DeleteIcon from "@mui/icons-material/Delete";
import EditIcon from "@mui/icons-material/Edit";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  createUser, deleteUser, listUsers, updateUser,
  type CreateUserInput, type User,
} from "../api/admin";

const EMPTY_CREATE: CreateUserInput = {
  username: "", email: "", displayName: "", password: "",
  isSuperAdmin: false, mustChangePassword: true,
};

// User administration: listing, creation, edit, and deletion against the admin
// API. Mutations invalidate the cached user list on success.
export function UsersPage() {
  const qc = useQueryClient();
  const { data: users = [], isLoading } = useQuery({ queryKey: ["users"], queryFn: listUsers });

  const [createOpen, setCreateOpen] = useState(false);
  const [form, setForm] = useState<CreateUserInput>(EMPTY_CREATE);
  const [editing, setEditing] = useState<User | null>(null);

  const invalidate = () => qc.invalidateQueries({ queryKey: ["users"] });

  const createMut = useMutation({
    mutationFn: () => createUser(form),
    onSuccess: () => { setCreateOpen(false); setForm(EMPTY_CREATE); invalidate(); },
  });
  const updateMut = useMutation({
    mutationFn: (u: User) =>
      updateUser(u.id, { email: u.email ?? "", displayName: u.displayName, isDisabled: u.isDisabled }),
    onSuccess: () => { setEditing(null); invalidate(); },
  });
  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteUser(id),
    onSuccess: invalidate,
  });

  return (
    <Box>
      <Stack direction="row" alignItems="center" sx={{ mb: 2 }}>
        <Typography variant="h5" sx={{ flexGrow: 1 }}>User Management</Typography>
        <Button startIcon={<AddIcon />} variant="contained" onClick={() => setCreateOpen(true)}>
          New User
        </Button>
      </Stack>

      <TableContainer component={Paper} variant="outlined">
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>Username</TableCell>
              <TableCell>Display Name</TableCell>
              <TableCell>Email</TableCell>
              <TableCell>Roles</TableCell>
              <TableCell>Status</TableCell>
              <TableCell align="right">Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {users.map((u) => (
              <TableRow key={u.id} hover>
                <TableCell>{u.username}{u.isSuperAdmin && (
                  <Chip label="super" size="small" color="secondary" sx={{ ml: 1 }} />
                )}</TableCell>
                <TableCell>{u.displayName}</TableCell>
                <TableCell>{u.email}</TableCell>
                <TableCell>{(u.roles ?? []).join(", ")}</TableCell>
                <TableCell>
                  <Chip
                    label={u.isDisabled ? "disabled" : "active"}
                    color={u.isDisabled ? "default" : "success"}
                    size="small"
                  />
                </TableCell>
                <TableCell align="right">
                  <Tooltip title="Edit">
                    <IconButton size="small" onClick={() => setEditing(u)}><EditIcon fontSize="small" /></IconButton>
                  </Tooltip>
                  <Tooltip title="Delete">
                    <IconButton size="small" onClick={() => deleteMut.mutate(u.id)}><DeleteIcon fontSize="small" /></IconButton>
                  </Tooltip>
                </TableCell>
              </TableRow>
            ))}
            {!isLoading && users.length === 0 && (
              <TableRow><TableCell colSpan={6}>
                <Typography color="text.secondary">No users yet.</Typography>
              </TableCell></TableRow>
            )}
          </TableBody>
        </Table>
      </TableContainer>

      <Dialog open={createOpen} onClose={() => setCreateOpen(false)} fullWidth maxWidth="sm">
        <DialogTitle>New User</DialogTitle>
        <DialogContent>
          <Stack spacing={2} sx={{ mt: 1 }}>
            <TextField label="Username" value={form.username}
              onChange={(e) => setForm({ ...form, username: e.target.value })} required />
            <TextField label="Display Name" value={form.displayName}
              onChange={(e) => setForm({ ...form, displayName: e.target.value })} />
            <TextField label="Email" type="email" value={form.email}
              onChange={(e) => setForm({ ...form, email: e.target.value })} />
            <TextField label="Password" type="password" value={form.password}
              onChange={(e) => setForm({ ...form, password: e.target.value })} required />
            <FormControlLabel control={
              <Switch checked={form.isSuperAdmin}
                onChange={(e) => setForm({ ...form, isSuperAdmin: e.target.checked })} />
            } label="Super admin" />
            <FormControlLabel control={
              <Switch checked={form.mustChangePassword}
                onChange={(e) => setForm({ ...form, mustChangePassword: e.target.checked })} />
            } label="Must change password" />
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setCreateOpen(false)}>Cancel</Button>
          <Button variant="contained" disabled={!form.username || !form.password || createMut.isPending}
            onClick={() => createMut.mutate()}>Create</Button>
        </DialogActions>
      </Dialog>

      <Dialog open={editing !== null} onClose={() => setEditing(null)} fullWidth maxWidth="sm">
        <DialogTitle>Edit User</DialogTitle>
        <DialogContent>
          {editing && (
            <Stack spacing={2} sx={{ mt: 1 }}>
              <TextField label="Username" value={editing.username} disabled />
              <TextField label="Display Name" value={editing.displayName}
                onChange={(e) => setEditing({ ...editing, displayName: e.target.value })} />
              <TextField label="Email" type="email" value={editing.email ?? ""}
                onChange={(e) => setEditing({ ...editing, email: e.target.value })} />
              <FormControlLabel control={
                <Switch checked={editing.isDisabled}
                  onChange={(e) => setEditing({ ...editing, isDisabled: e.target.checked })} />
              } label="Disabled" />
            </Stack>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setEditing(null)}>Cancel</Button>
          <Button variant="contained" disabled={updateMut.isPending}
            onClick={() => editing && updateMut.mutate(editing)}>Save</Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
