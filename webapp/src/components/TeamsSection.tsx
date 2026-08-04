import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { ApiError } from "../api/client";
import { listTeams, updateTeamEmail, addTeamMember, removeTeamMember } from "../api/teams";
import { ROLE_LABELS } from "../types/api";
import type { Team } from "../types/api";
import { DirectoryUserPicker } from "./DirectoryUserPicker";

// TeamsSection manages the Legal / Security / Compliance / Procurement teams from the Settings
// page: each team's shared email and its members (adding/removing a member
// grants/revokes the team's member role). The member role itself is fixed —
// shown, not editable — because the approval-card actor logic is keyed on it.
export function TeamsSection() {
  const qc = useQueryClient();
  const { data: teams, isLoading, error } = useQuery({ queryKey: ["teams"], queryFn: listTeams });
  const [actionError, setActionError] = useState<string | null>(null);

  const invalidate = () => qc.invalidateQueries({ queryKey: ["teams"] });
  const onError = (e: unknown) =>
    setActionError(e instanceof ApiError ? e.message : "Action failed");

  const setEmail = useMutation({
    mutationFn: ({ key, email }: { key: string; email: string }) => updateTeamEmail(key, email),
    onSuccess: () => {
      setActionError(null);
      invalidate();
    },
    onError,
  });
  const addMember = useMutation({
    mutationFn: ({ key, userId }: { key: string; userId: number }) => addTeamMember(key, userId),
    onSuccess: () => {
      setActionError(null);
      invalidate();
    },
    onError,
  });
  const removeMember = useMutation({
    mutationFn: ({ key, userId }: { key: string; userId: number }) => removeTeamMember(key, userId),
    onSuccess: () => {
      setActionError(null);
      invalidate();
    },
    onError,
  });

  return (
    <Box component="section">
      <Typography variant="h6" sx={{ mb: 0.5 }}>
        Teams
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        Members of a team hold its role — adding or removing a member grants or revokes that role.
        The team email is CC'd on the team's approval notifications.
      </Typography>

      {actionError && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {actionError}
        </Alert>
      )}
      {isLoading && (
        <Typography variant="body2" color="text.secondary">
          Loading…
        </Typography>
      )}
      {error && (
        <Typography variant="body2" color="error.main">
          Failed to load teams.
        </Typography>
      )}

      <Box
        sx={{
          display: "grid",
          gap: 2,
          gridTemplateColumns: { xs: "1fr", md: "repeat(2, 1fr)", xl: "repeat(3, 1fr)" },
        }}
      >
        {(teams ?? []).map((team) => (
          <TeamCard
            key={team.key}
            team={team}
            busy={setEmail.isPending || addMember.isPending || removeMember.isPending}
            onSaveEmail={(email) => setEmail.mutate({ key: team.key, email })}
            onAddMember={(userId) => addMember.mutate({ key: team.key, userId })}
            onRemoveMember={(userId) => removeMember.mutate({ key: team.key, userId })}
          />
        ))}
      </Box>
    </Box>
  );
}

function TeamCard({
  team,
  busy,
  onSaveEmail,
  onAddMember,
  onRemoveMember,
}: {
  team: Team;
  busy: boolean;
  onSaveEmail: (email: string) => void;
  onAddMember: (userId: number) => void;
  onRemoveMember: (userId: number) => void;
}) {
  const [email, setEmail] = useState(team.team_email);
  const emailDirty = email.trim() !== team.team_email;

  // Tolerate an older API response that predates admin_members.
  const adminMembers = team.admin_members ?? [];
  const adminIds = useMemo(() => new Set(adminMembers.map((m) => m.id)), [adminMembers]);
  // Base members are shown as removable rows; anyone who is also an admin is lifted
  // out into the badged, non-removable admin group (admins are managed on Users).
  const baseMembers = useMemo(
    () => team.members.filter((m) => !adminIds.has(m.id)),
    [team.members, adminIds],
  );
  const memberIds = useMemo(
    () => new Set([...team.members, ...adminMembers].map((m) => m.id)),
    [team.members, adminMembers],
  );
  const totalMembers = baseMembers.length + adminMembers.length;

  return (
    <Card variant="outlined">
      <CardContent>
        <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 1 }}>
          <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
            {team.name}
          </Typography>
          <Chip size="small" variant="outlined" label={ROLE_LABELS[team.member_role]} />
        </Box>

        {/* Team email */}
        <Typography
          variant="caption"
          component="label"
          sx={{
            display: "block",
            mt: 2,
            fontWeight: 600,
            textTransform: "uppercase",
            letterSpacing: "0.05em",
            color: "text.secondary",
          }}
        >
          Team email
        </Typography>
        <Box sx={{ mt: 0.5, display: "flex", gap: 1 }}>
          <TextField
            type="email"
            size="small"
            fullWidth
            placeholder="team@example.com"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
          <Button
            variant="contained"
            disabled={busy || !emailDirty}
            onClick={() => onSaveEmail(email.trim())}
          >
            Save
          </Button>
        </Box>

        {/* Members */}
        <Typography
          variant="caption"
          sx={{
            display: "block",
            mt: 2,
            fontWeight: 600,
            textTransform: "uppercase",
            letterSpacing: "0.05em",
            color: "text.secondary",
          }}
        >
          Members ({totalMembers})
        </Typography>
        {totalMembers === 0 ? (
          <Typography variant="body2" color="text.disabled" sx={{ mt: 0.5 }}>
            No members yet.
          </Typography>
        ) : (
          <Stack spacing={0.5} sx={{ mt: 0.5 }}>
            {baseMembers.map((m) => (
              <Box
                key={m.id}
                sx={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  gap: 1,
                  border: 1,
                  borderColor: "divider",
                  borderRadius: 1,
                  bgcolor: "background.default",
                  px: 1.25,
                  py: 0.75,
                }}
              >
                <Typography
                  variant="body2"
                  color="text.secondary"
                  title={m.email}
                  sx={{ minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}
                >
                  {m.email || m.name || `#${m.id}`}
                </Typography>
                <Button
                  variant="text"
                  color="error"
                  size="small"
                  disabled={busy}
                  onClick={() => onRemoveMember(m.id)}
                  sx={{ flexShrink: 0 }}
                >
                  Remove
                </Button>
              </Box>
            ))}
            {adminMembers.map((m) => (
              <Box
                key={m.id}
                sx={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  gap: 1,
                  border: 1,
                  borderColor: "divider",
                  borderRadius: 1,
                  bgcolor: "background.default",
                  px: 1.25,
                  py: 0.75,
                }}
              >
                <Typography
                  variant="body2"
                  color="text.secondary"
                  title={m.email}
                  sx={{ minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}
                >
                  {m.email || m.name || `#${m.id}`}
                </Typography>
                <Chip
                  size="small"
                  variant="outlined"
                  color="warning"
                  label="admin"
                  title="Admin — manage this role on the Users page"
                  sx={{ flexShrink: 0 }}
                />
              </Box>
            ))}
          </Stack>
        )}
        {adminMembers.length > 0 && (
          <Typography variant="caption" color="text.disabled" sx={{ display: "block", mt: 0.5 }}>
            Admins are managed on the Users page.
          </Typography>
        )}

        {/* Add member — search the org directory; a picked person who isn't yet an
            app user is provisioned (invited) and granted the team's role. */}
        <Box sx={{ mt: 1 }}>
          <DirectoryUserPicker
            selectedIds={[...memberIds]}
            onAdd={(u) => onAddMember(u.id)}
            disabled={busy}
          />
        </Box>
      </CardContent>
    </Card>
  );
}
