import { useState } from "react";
import {
  Box, Button, Checkbox, Chip, Dialog, DialogActions, DialogContent, DialogTitle,
  FormControlLabel, FormGroup, IconButton, Paper, Stack, Table, TableBody, TableCell,
  TableContainer, TableHead, TableRow, TextField, Tooltip, Typography,
} from "@mui/material";
import AddIcon from "@mui/icons-material/Add";
import DeleteIcon from "@mui/icons-material/Delete";
import PeopleIcon from "@mui/icons-material/People";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  addUserToGroup, createGroup, deleteGroup, listGroups, listUsers, removeUserFromGroup,
  type Group,
} from "../api/admin";

// Group administration: shared host-authorization buckets. A user in a group can
// reach any host that is also in that group (see a host's Manage access dialog).
export function GroupsPage() {
  const qc = useQueryClient();
  const { data: groups = [], isLoading } = useQuery({ queryKey: ["groups"], queryFn: listGroups });
  const { data: users = [] } = useQuery({ queryKey: ["users"], queryFn: listUsers });

  const [createOpen, setCreateOpen] = useState(false);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [membersGroup, setMembersGroup] = useState<Group | null>(null);

  const invalidate = () => qc.invalidateQueries({ queryKey: ["groups"] });

  const createMut = useMutation({
    mutationFn: () => createGroup(name, description),
    onSuccess: () => { setCreateOpen(false); setName(""); setDescription(""); invalidate(); },
  });
  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteGroup(id),
    onSuccess: invalidate,
  });
  const memberMut = useMutation({
    mutationFn: ({ userId, groupId, add }: { userId: string; groupId: string; add: boolean }) =>
      add ? addUserToGroup(userId, groupId) : removeUserFromGroup(userId, groupId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["users"] }),
  });

  const memberCount = (g: Group) => users.filter((u) => (u.groups ?? []).includes(g.name)).length;

  return (
    <Box>
      <Stack direction="row" alignItems="center" sx={{ mb: 2 }}>
        <Typography variant="h5" sx={{ flexGrow: 1 }}>Group Management</Typography>
        <Button startIcon={<AddIcon />} variant="contained" onClick={() => setCreateOpen(true)}>
          New Group
        </Button>
      </Stack>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        Put users in a group, then add that group to hosts (host → Manage access).
        Every member of the group can then reach every host in it.
      </Typography>

      <TableContainer component={Paper} variant="outlined">
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>Name</TableCell>
              <TableCell>Description</TableCell>
              <TableCell>Members</TableCell>
              <TableCell align="right">Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {groups.map((g) => (
              <TableRow key={g.id} hover>
                <TableCell>{g.name}</TableCell>
                <TableCell>{g.description}</TableCell>
                <TableCell>
                  <Chip size="small" label={memberCount(g)} />
                </TableCell>
                <TableCell align="right">
                  <Tooltip title="Manage members">
                    <IconButton size="small" onClick={() => setMembersGroup(g)}><PeopleIcon fontSize="small" /></IconButton>
                  </Tooltip>
                  <Tooltip title="Delete">
                    <IconButton size="small" onClick={() => deleteMut.mutate(g.id)}><DeleteIcon fontSize="small" /></IconButton>
                  </Tooltip>
                </TableCell>
              </TableRow>
            ))}
            {!isLoading && groups.length === 0 && (
              <TableRow><TableCell colSpan={4}>
                <Typography color="text.secondary">No groups yet.</Typography>
              </TableCell></TableRow>
            )}
          </TableBody>
        </Table>
      </TableContainer>

      <Dialog open={createOpen} onClose={() => setCreateOpen(false)} fullWidth maxWidth="sm">
        <DialogTitle>New Group</DialogTitle>
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

      <Dialog open={Boolean(membersGroup)} onClose={() => setMembersGroup(null)} fullWidth maxWidth="xs">
        <DialogTitle>Members — {membersGroup?.name}</DialogTitle>
        <DialogContent dividers>
          {(() => {
            // Membership comes from each user's live group list, so checkboxes
            // update after toggling.
            const gname = membersGroup?.name;
            return (
              <FormGroup>
                {users.map((u) => (
                  <FormControlLabel
                    key={u.id}
                    control={
                      <Checkbox
                        checked={(u.groups ?? []).includes(gname ?? "")}
                        disabled={memberMut.isPending}
                        onChange={(e) =>
                          membersGroup && memberMut.mutate({ userId: u.id, groupId: membersGroup.id, add: e.target.checked })
                        }
                      />
                    }
                    label={u.username}
                  />
                ))}
                {users.length === 0 && (
                  <Typography variant="body2" color="text.secondary">No users yet.</Typography>
                )}
              </FormGroup>
            );
          })()}
        </DialogContent>
        <DialogActions><Button onClick={() => setMembersGroup(null)}>Close</Button></DialogActions>
      </Dialog>
    </Box>
  );
}
