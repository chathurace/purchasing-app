import { useMemo, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  MenuItem,
  Paper,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { useUsers } from "../hooks/useUsers";
import { useIsAdmin } from "../hooks/useIsAdmin";
import { useMe } from "../hooks/useMe";
import { addUserRole, createUser, removeUserRole, setUserActive, updateUser } from "../api/users";
import { ApiError } from "../api/client";
import { useDirectory } from "../hooks/useDirectory";
import { EmailAutocomplete } from "../components/EmailAutocomplete";
import { ASSIGNABLE_ROLES, ROLE_LABELS, type AdminUser, type Role } from "../types/api";

// A user is "invited" (pending, never signed in), "active" (signed in and
// enabled), or "deactivated" (login blocked) — matching the Status column badges.
type UserStatusFilter = "all" | "invited" | "active" | "deactivated";

function matchesStatus(u: AdminUser, f: UserStatusFilter): boolean {
  switch (f) {
    case "invited":
      return u.is_active && u.pending;
    case "active":
      return u.is_active && !u.pending;
    case "deactivated":
      return !u.is_active;
    default:
      return true;
  }
}

export function UserManagementPage() {
  const isAdmin = useIsAdmin();
  const { data: me } = useMe();
  const { data: users, isLoading, error } = useUsers(isAdmin);
  const qc = useQueryClient();

  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const { data: directory, isLoading: dirLoading, refresh: refreshDir } = useDirectory(isAdmin);
  const [formError, setFormError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  // Filters (client-side — the whole list is already loaded for admins).
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<UserStatusFilter>("all");

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return (users ?? [])
      .filter((u) => matchesStatus(u, statusFilter))
      .filter((u) =>
        q === ""
          ? true
          : u.name.toLowerCase().includes(q) || u.email.toLowerCase().includes(q),
      );
  }, [users, search, statusFilter]);

  const invalidate = () => qc.invalidateQueries({ queryKey: ["users"] });
  const onActionError = (e: unknown) =>
    setActionError(e instanceof ApiError ? e.message : "Action failed");

  const create = useMutation({
    mutationFn: () => createUser({ email: email.trim(), name: name.trim() }),
    onSuccess: () => {
      setEmail("");
      setName("");
      setFormError(null);
      invalidate();
    },
    onError: (e) => setFormError(e instanceof ApiError ? e.message : "Failed to add user"),
  });

  const addRole = useMutation({
    mutationFn: ({ id, role }: { id: number; role: Role }) => addUserRole(id, role),
    onSuccess: invalidate,
    onError: onActionError,
  });
  const removeRole = useMutation({
    mutationFn: ({ id, role }: { id: number; role: Role }) => removeUserRole(id, role),
    onSuccess: invalidate,
    onError: onActionError,
  });
  const toggleActive = useMutation({
    mutationFn: ({ id, active }: { id: number; active: boolean }) => setUserActive(id, active),
    onSuccess: invalidate,
    onError: onActionError,
  });
  const update = useMutation({
    mutationFn: ({ id, email, name }: { id: number; email: string; name: string }) =>
      updateUser(id, { email, name }),
    onSuccess: invalidate,
  });

  if (!isAdmin) {
    return (
      <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
        <Card variant="outlined" sx={{ borderStyle: "dashed" }}>
          <CardContent sx={{ p: 4, textAlign: "center" }}>
            <Typography color="text.secondary">
              You need the admin role to manage users.
            </Typography>
          </CardContent>
        </Card>
      </Box>
    );
  }

  return (
    <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
      <Typography variant="h5" sx={{ fontWeight: 600 }}>
        Users
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5, mb: 3 }}>
        Add people by email and manage their roles. Anyone can sign in and starts as staff; adding
        them here lets you assign roles before their first login. Deactivating a user blocks them
        from signing in without losing their history.
      </Typography>

      {/* Add user */}
      <Card
        variant="outlined"
        component="form"
        onSubmit={(e: React.FormEvent) => {
          e.preventDefault();
          create.mutate();
        }}
        sx={{ mb: 3 }}
      >
        <CardContent>
          <Stack direction="row" spacing={1.5} sx={{ flexWrap: "wrap", alignItems: "flex-end" }}>
            <Box>
              <Typography variant="caption" sx={{ fontWeight: 500, display: "block", mb: 0.5 }}>
                Email
              </Typography>
              <Box sx={{ width: 256 }}>
                <EmailAutocomplete
                  value={email}
                  onChange={(picked, u) => {
                    setEmail(picked);
                    if (u && u.name) setName(u.name);
                  }}
                  directory={directory ?? []}
                  refresh={refreshDir}
                  loading={dirLoading}
                  placeholder="person@company.com"
                  ariaLabel="User email"
                />
              </Box>
            </Box>
            <Box>
              <Typography variant="caption" sx={{ fontWeight: 500, display: "block", mb: 0.5 }}>
                Name (optional)
              </Typography>
              <TextField
                size="small"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="Jane Doe"
                sx={{ width: 192 }}
              />
            </Box>
            <Button
              type="submit"
              variant="contained"
              disabled={create.isPending || email.trim() === ""}
            >
              {create.isPending ? "Adding…" : "Add user"}
            </Button>
          </Stack>
          {formError && (
            <Typography variant="body2" color="error.main" sx={{ mt: 1 }}>
              {formError}
            </Typography>
          )}
        </CardContent>
      </Card>

      {actionError && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {actionError}
        </Alert>
      )}

      <Stack direction="row" spacing={1.5} sx={{ mb: 2, flexWrap: "wrap", alignItems: "center" }}>
        <TextField
          size="small"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search by name or email…"
          sx={{ width: 288 }}
        />
        <TextField
          select
          size="small"
          value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value as UserStatusFilter)}
        >
          <MenuItem value="all">All statuses</MenuItem>
          <MenuItem value="invited">Invited</MenuItem>
          <MenuItem value="active">Active</MenuItem>
          <MenuItem value="deactivated">Deactivated</MenuItem>
        </TextField>
      </Stack>

      {isLoading && <CircularProgress size={24} />}
      {error && <Alert severity="error">Failed to load users.</Alert>}

      {users && (
        <TableContainer component={Paper} variant="outlined">
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>User</TableCell>
                <TableCell>Status</TableCell>
                <TableCell>Roles</TableCell>
                <TableCell align="right">Actions</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {filtered.length === 0 && (
                <TableRow>
                  <TableCell colSpan={4} align="center" sx={{ py: 4, color: "text.secondary" }}>
                    No users match.
                  </TableCell>
                </TableRow>
              )}
              {filtered.map((u) => (
                <UserRow
                  key={u.id}
                  user={u}
                  isSelf={me?.id === u.id}
                  onAddRole={(role) => addRole.mutate({ id: u.id, role })}
                  onRemoveRole={(role) => removeRole.mutate({ id: u.id, role })}
                  onToggleActive={() => toggleActive.mutate({ id: u.id, active: !u.is_active })}
                  onSave={(email, name) => update.mutateAsync({ id: u.id, email, name })}
                />
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}
    </Box>
  );
}

function UserRow({
  user,
  isSelf,
  onAddRole,
  onRemoveRole,
  onToggleActive,
  onSave,
}: {
  user: AdminUser;
  isSelf: boolean;
  onAddRole: (role: Role) => void;
  onRemoveRole: (role: Role) => void;
  onToggleActive: () => void;
  onSave: (email: string, name: string) => Promise<unknown>;
}) {
  const addable = ASSIGNABLE_ROLES.filter((r) => !user.roles.includes(r));
  const [editing, setEditing] = useState(false);
  const [email, setEmail] = useState(user.email);
  const [name, setName] = useState(user.name);
  const [saving, setSaving] = useState(false);
  const [editError, setEditError] = useState<string | null>(null);

  const startEdit = () => {
    setEmail(user.email);
    setName(user.name);
    setEditError(null);
    setEditing(true);
  };
  const save = async () => {
    setSaving(true);
    setEditError(null);
    try {
      await onSave(email.trim(), name.trim());
      setEditing(false);
    } catch (e) {
      setEditError(e instanceof ApiError ? e.message : "Failed to save");
    } finally {
      setSaving(false);
    }
  };

  return (
    <TableRow sx={{ bgcolor: user.is_active ? "transparent" : "action.hover" }}>
      <TableCell sx={{ verticalAlign: "top" }}>
        {editing ? (
          <Stack spacing={1}>
            <TextField
              size="small"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="person@company.com"
              sx={{ width: 256 }}
            />
            <TextField
              size="small"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Name (optional)"
              sx={{ width: 256 }}
            />
            {editError && (
              <Typography variant="caption" color="error.main">
                {editError}
              </Typography>
            )}
          </Stack>
        ) : (
          <>
            <Typography variant="body2" sx={{ fontWeight: 500 }}>
              {user.email || "(no email)"}
            </Typography>
            {user.name && (
              <Typography variant="body2" color="text.secondary">
                {user.name}
              </Typography>
            )}
          </>
        )}
      </TableCell>
      <TableCell sx={{ verticalAlign: "top" }}>
        {!user.is_active ? (
          <Chip size="small" variant="outlined" color="error" label="Deactivated" />
        ) : user.pending ? (
          <Chip size="small" variant="outlined" color="warning" label="Invited" />
        ) : (
          <Chip size="small" variant="outlined" color="success" label="Active" />
        )}
      </TableCell>
      <TableCell sx={{ verticalAlign: "top" }}>
        <Stack direction="row" spacing={0.75} sx={{ flexWrap: "wrap", alignItems: "center", gap: 0.75 }}>
          {user.roles.length === 0 && (
            <Typography variant="body2" color="text.secondary">
              —
            </Typography>
          )}
          {user.roles.map((r) => (
            <Chip
              key={r}
              size="small"
              label={ROLE_LABELS[r] ?? r}
              onDelete={r !== "staff" ? () => onRemoveRole(r) : undefined}
            />
          ))}
          {addable.length > 0 && (
            <TextField
              select
              size="small"
              value=""
              onChange={(e) => {
                if (e.target.value) onAddRole(e.target.value as Role);
              }}
              SelectProps={{ displayEmpty: true }}
              sx={{ minWidth: 120 }}
            >
              <MenuItem value="">+ Add role</MenuItem>
              {addable.map((r) => (
                <MenuItem key={r} value={r}>
                  {ROLE_LABELS[r] ?? r}
                </MenuItem>
              ))}
            </TextField>
          )}
        </Stack>
      </TableCell>
      <TableCell align="right" sx={{ verticalAlign: "top" }}>
        <Stack direction="row" spacing={1} sx={{ justifyContent: "flex-end" }}>
          {editing ? (
            <>
              <Button
                size="small"
                variant="contained"
                onClick={save}
                disabled={saving || email.trim() === ""}
              >
                {saving ? "Saving…" : "Save"}
              </Button>
              <Button
                size="small"
                variant="outlined"
                color="inherit"
                onClick={() => setEditing(false)}
                disabled={saving}
              >
                Cancel
              </Button>
            </>
          ) : (
            <>
              {user.pending && (
                <Button
                  size="small"
                  variant="outlined"
                  color="inherit"
                  onClick={startEdit}
                  title="Edit this invited user's email or name (available until their first sign-in)"
                >
                  Edit
                </Button>
              )}
              <Button
                size="small"
                variant="outlined"
                color="inherit"
                onClick={onToggleActive}
                disabled={isSelf}
                title={isSelf ? "You cannot deactivate your own account" : undefined}
              >
                {user.is_active ? "Deactivate" : "Reactivate"}
              </Button>
            </>
          )}
        </Stack>
      </TableCell>
    </TableRow>
  );
}
