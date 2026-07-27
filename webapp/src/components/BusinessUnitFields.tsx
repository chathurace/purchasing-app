import { useMemo } from "react";
import { Box, Stack, TextField, Typography } from "@wso2/oxygen-ui";
import { useUserLookup } from "../hooks/useUserLookup";
import { DirectoryUserPicker } from "./DirectoryUserPicker";
import type { BusinessUnitInput } from "../types/api";

interface Props {
  value: BusinessUnitInput;
  onChange: (next: BusinessUnitInput) => void;
}

// BusinessUnitFields edits everything except the active/inactive status, which is
// controlled separately via an Activate/Deactivate action (mirrors VendorFields).
// A business unit is a name, an optional description, and a flat list of
// approvers — any of whom the requester may pick as the budget approver.
export function BusinessUnitFields({ value, onChange }: Props) {
  const set = (patch: Partial<BusinessUnitInput>) => onChange({ ...value, ...patch });
  const { data: users } = useUserLookup();

  const selectedUsers = useMemo(
    () => (users ?? []).filter((u) => value.approver_ids.includes(u.id)),
    [users, value.approver_ids],
  );

  return (
    <Stack spacing={2}>
      <TextField
        label="Name"
        required
        size="small"
        fullWidth
        value={value.name}
        onChange={(e) => set({ name: e.target.value })}
        placeholder="e.g. Engineering"
      />

      <TextField
        label="Description"
        size="small"
        fullWidth
        multiline
        minRows={2}
        value={value.description}
        onChange={(e) => set({ description: e.target.value })}
        placeholder="What this business unit covers"
      />

      <Box>
        <Typography variant="body2" sx={{ fontWeight: 600, mb: 0.5 }}>
          Approvers *
        </Typography>
        <Typography variant="caption" color="text.secondary" sx={{ display: "block", mb: 1 }}>
          The budget approvers for this business unit. When a requester picks this unit, they choose
          the budget approver from this list.
        </Typography>
        <DirectoryUserPicker
          selectedIds={value.approver_ids}
          selectedUsers={selectedUsers}
          onAdd={(u) => set({ approver_ids: [...value.approver_ids, u.id] })}
          onRemove={(id) => set({ approver_ids: value.approver_ids.filter((x) => x !== id) })}
        />
      </Box>
    </Stack>
  );
}
