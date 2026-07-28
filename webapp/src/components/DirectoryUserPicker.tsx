import { useMemo, useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { Box, Chip, MenuItem, Paper, Stack, TextField, Typography } from "@wso2/oxygen-ui";
import { ensureDirectoryUser } from "../api/users";
import { useDirectory, useRefreshOnNoMatch } from "../hooks/useDirectory";
import { searchPeople } from "../lib/directorySearch";
import { ApiError } from "../api/client";
import type { DirectoryUser, UserSummary } from "../types/api";

interface Props {
  // selectedIds are already-chosen app users; their emails are excluded from the
  // suggestion list to avoid duplicates.
  selectedIds: number[];
  onAdd: (user: UserSummary) => void;
  // When both are provided, chosen users render as removable chips.
  selectedUsers?: UserSummary[];
  onRemove?: (id: number) => void;
  disabled?: boolean;
}

function label(u: UserSummary): string {
  return u.name ? `${u.name} (${u.email})` : u.email || `#${u.id}`;
}

// DirectoryUserPicker is a search-and-add control for id-based fields (business
// unit approvers, team members) that sources candidates from the org directory
// (SCIM). Picking someone resolves them to an app user via /users/ensure
// (get-or-create by email) so a person who has never logged in can still be
// chosen — then reports the resulting UserSummary (with an id) to onAdd.
export function DirectoryUserPicker({
  selectedIds,
  onAdd,
  selectedUsers,
  onRemove,
  disabled,
}: Props) {
  const { data: directory, isLoading, refresh } = useDirectory();
  const [query, setQuery] = useState("");
  const [error, setError] = useState<string | null>(null);
  // resolved holds users ensured this session, so chips and de-duplication work
  // immediately — before the parent's own user list refetches.
  const [resolved, setResolved] = useState<Record<number, UserSummary>>({});

  const chips = useMemo(() => {
    const byId = new Map<number, UserSummary>();
    for (const u of selectedUsers ?? []) byId.set(u.id, u);
    for (const u of Object.values(resolved)) byId.set(u.id, u);
    return selectedIds.map((id) => byId.get(id) ?? { id, email: "", name: `#${id}` });
  }, [selectedIds, selectedUsers, resolved]);

  const selectedEmails = useMemo(
    () => new Set(chips.map((u) => u.email.toLowerCase()).filter(Boolean)),
    [chips],
  );

  const ensure = useMutation({
    mutationFn: (u: DirectoryUser) => ensureDirectoryUser({ email: u.email, name: u.name }),
    onSuccess: (user) => {
      setResolved((r) => ({ ...r, [user.id]: user }));
      if (!selectedIds.includes(user.id)) onAdd(user);
      setQuery("");
      setError(null);
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to add user"),
  });

  const { available, foundCount } = useMemo(() => {
    const { matches, foundCount } = searchPeople(directory, query, { exclude: selectedEmails });
    return { available: matches, foundCount };
  }, [directory, selectedEmails, query]);

  useRefreshOnNoMatch(query, foundCount, refresh);

  const busy = disabled || ensure.isPending;

  return (
    <Box>
      {onRemove && selectedUsers && chips.length > 0 && (
        <Stack direction="row" flexWrap="wrap" gap={1} sx={{ mb: 1 }}>
          {chips.map((u) => (
            <Chip
              key={u.id}
              size="small"
              variant="outlined"
              label={label(u)}
              disabled={busy}
              onDelete={() => onRemove(u.id)}
              aria-label={`Remove ${label(u)}`}
            />
          ))}
        </Stack>
      )}

      <TextField
        size="small"
        fullWidth
        value={query}
        disabled={busy}
        onChange={(e) => setQuery(e.target.value)}
        placeholder={isLoading ? "Loading directory…" : "Search people by name or email…"}
      />

      {query.trim() !== "" && (
        <Paper variant="outlined" sx={{ mt: 0.5, maxHeight: 224, overflow: "auto" }}>
          {available.length === 0 ? (
            <Box sx={{ px: 1.5, py: 1 }}>
              <Typography variant="body2" color="text.secondary">
                No matching users.
              </Typography>
            </Box>
          ) : (
            available.map((u) => (
              <MenuItem
                key={u.email}
                disabled={busy}
                onClick={() => ensure.mutate(u)}
                sx={{ display: "flex", justifyContent: "space-between", gap: 1 }}
              >
                <Typography variant="body2" color="text.primary">
                  {u.name || u.email}
                </Typography>
                {u.name && (
                  <Typography variant="caption" color="text.secondary">
                    {u.email}
                  </Typography>
                )}
              </MenuItem>
            ))
          )}
        </Paper>
      )}
      {error && (
        <Typography variant="caption" color="error.main" sx={{ display: "block", mt: 0.5 }}>
          {error}
        </Typography>
      )}
    </Box>
  );
}
