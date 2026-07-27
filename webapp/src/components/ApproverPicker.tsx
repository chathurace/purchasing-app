import { useMemo, useState } from "react";
import { Box, Chip, MenuItem, Paper, Stack, TextField, Typography } from "@wso2/oxygen-ui";
import type { UserSummary } from "../types/api";

interface Props {
  // candidates is the full set of selectable users (already excluding self).
  candidates: UserSummary[];
  // selectedIds are the users already chosen/added; they are hidden from the
  // add list to avoid duplicates.
  selectedIds: number[];
  onAdd: (user: UserSummary) => void;
  // When provided, the chosen users render as removable chips below the input.
  selectedUsers?: UserSummary[];
  onRemove?: (id: number) => void;
  disabled?: boolean;
  loading?: boolean;
}

function label(u: UserSummary): string {
  return u.name ? `${u.name} (${u.email})` : u.email || `#${u.id}`;
}

// ApproverPicker is a search-and-add control for choosing approvers. It is used
// both on the new-request form (local selection) and on the detail page (each
// add is an immediate API call).
export function ApproverPicker({
  candidates,
  selectedIds,
  onAdd,
  selectedUsers,
  onRemove,
  disabled,
  loading,
}: Props) {
  const [query, setQuery] = useState("");

  const available = useMemo(() => {
    const chosen = new Set(selectedIds);
    const q = query.trim().toLowerCase();
    return candidates
      .filter((u) => !chosen.has(u.id))
      .filter((u) => q === "" || u.email.toLowerCase().includes(q) || u.name.toLowerCase().includes(q))
      .slice(0, 8);
  }, [candidates, selectedIds, query]);

  return (
    <Box>
      {onRemove && selectedUsers && selectedUsers.length > 0 && (
        <Stack direction="row" flexWrap="wrap" gap={1} sx={{ mb: 1 }}>
          {selectedUsers.map((u) => (
            <Chip
              key={u.id}
              size="small"
              variant="outlined"
              label={label(u)}
              disabled={disabled}
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
        disabled={disabled}
        onChange={(e) => setQuery(e.target.value)}
        placeholder={loading ? "Loading users…" : "Search people by name or email…"}
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
                key={u.id}
                disabled={disabled}
                onClick={() => {
                  onAdd(u);
                  setQuery("");
                }}
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
    </Box>
  );
}
