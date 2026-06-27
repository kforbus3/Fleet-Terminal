import { useState } from "react";
import {
  Alert, Box, Button, Chip, Dialog, DialogActions, DialogContent, DialogTitle,
  Divider, FormControlLabel, IconButton, List, ListItem, ListItemText, Menu,
  MenuItem, Snackbar, Stack, Switch, Table, TableBody, TableCell,
  TableContainer, TableHead, TableRow, TextField, Typography, Paper, Tooltip,
} from "@mui/material";
import AddIcon from "@mui/icons-material/Add";
import DeleteIcon from "@mui/icons-material/Delete";
import EditIcon from "@mui/icons-material/Edit";
import MoreVertIcon from "@mui/icons-material/MoreVert";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  createUser, deleteUser, listUsers, resetUserMFA, resetUserPassword,
  setUserDisabled, terminateUserSessions, unlockUser, updateUser,
  userLoginHistory, type AuthEvent, type CreateUserInput, type User,
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
  const [menuEl, setMenuEl] = useState<null | HTMLElement>(null);
  const [menuUser, setMenuUser] = useState<User | null>(null);
  const [snack, setSnack] = useState<string | null>(null);
  const [history, setHistory] = useState<{ user: User; events: AuthEvent[] } | null>(null);

  const invalidate = () => qc.invalidateQueries({ queryKey: ["users"] });

  const openMenu = (el: HTMLElement, u: User) => { setMenuEl(el); setMenuUser(u); };
  const closeMenu = () => { setMenuEl(null); setMenuUser(null); };

  // Run an admin action against the menu's user, show feedback, refresh.
  const runAction = async (label: string, fn: (id: string) => Promise<void>) => {
    if (!menuUser) return;
    const u = menuUser;
    closeMenu();
    try {
      await fn(u.id);
      setSnack(`${label}: ${u.username}`);
      invalidate();
    } catch {
      setSnack(`Failed: ${label}`);
    }
  };

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
                  <Tooltip title="More actions">
                    <IconButton size="small" onClick={(e) => openMenu(e.currentTarget, u)}>
                      <MoreVertIcon fontSize="small" />
                    </IconButton>
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

      <Menu anchorEl={menuEl} open={Boolean(menuEl)} onClose={closeMenu}>
        <MenuItem onClick={() => runAction(menuUser?.isDisabled ? "Enabled" : "Disabled",
          (id) => setUserDisabled(id, !menuUser?.isDisabled))}>
          {menuUser?.isDisabled ? "Enable account" : "Disable account"}
        </MenuItem>
        <MenuItem onClick={() => runAction("Unlocked", unlockUser)}>Unlock account</MenuItem>
        <Divider />
        <MenuItem onClick={() => {
          const pw = window.prompt("New password for " + menuUser?.username);
          if (pw) void runAction("Password reset", (id) => resetUserPassword(id, pw, true));
          else closeMenu();
        }}>Reset password…</MenuItem>
        <MenuItem onClick={() => runAction("MFA reset", resetUserMFA)}>Reset MFA</MenuItem>
        <MenuItem onClick={() => runAction("Sessions terminated", terminateUserSessions)}>
          Terminate sessions
        </MenuItem>
        <Divider />
        <MenuItem onClick={async () => {
          if (!menuUser) return;
          const u = menuUser; closeMenu();
          try { setHistory({ user: u, events: await userLoginHistory(u.id) }); }
          catch { setSnack("Failed to load history"); }
        }}>Login history…</MenuItem>
      </Menu>

      <Dialog open={Boolean(history)} onClose={() => setHistory(null)} fullWidth maxWidth="sm">
        <DialogTitle>Login history — {history?.user.username}</DialogTitle>
        <DialogContent dividers>
          <List dense>
            {(history?.events ?? []).map((e) => (
              <ListItem key={e.id} disableGutters>
                <ListItemText
                  primary={`${e.event}${e.ip ? "  ·  " + e.ip : ""}`}
                  secondary={new Date(e.createdAt).toLocaleString()}
                />
              </ListItem>
            ))}
            {history && history.events.length === 0 && (
              <ListItem><ListItemText primary="No events" /></ListItem>
            )}
          </List>
        </DialogContent>
        <DialogActions><Button onClick={() => setHistory(null)}>Close</Button></DialogActions>
      </Dialog>

      <Snackbar
        open={Boolean(snack)} autoHideDuration={3000} onClose={() => setSnack(null)}
        anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
      >
        <Alert severity="info" onClose={() => setSnack(null)}>{snack}</Alert>
      </Snackbar>
    </Box>
  );
}
