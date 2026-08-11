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
  IconButton,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { ChevronUp, ChevronDown } from "@wso2/oxygen-ui-icons-react";
import { ApiError } from "../api/client";
import {
  createConfigOption,
  deleteConfigOption,
  updateConfigOption,
  type ConfigList,
  type ConfigOption,
} from "../api/config";
import { useConfigLookup, useConfigOptionsAdmin } from "../hooks/useConfigOptions";
import { useCanManageBusinessUnits } from "../hooks/useCanManageBusinessUnits";
import { useIsAdmin } from "../hooks/useIsAdmin";
import { useConfirmAction } from "../components/ConfirmDialog";
import { StorageSettings } from "../components/StorageSettings";
import { TeamsSection } from "../components/TeamsSection";

// SettingsPage lets admin / procurement_admin manage the values that populate the
// requisition-form dropdowns. Access mirrors the Business units page
// (middleware.HasBusinessUnitAdmin enforces the same rule server-side).
export function SettingsPage() {
  const isAdmin = useIsAdmin();
  const canManage = useCanManageBusinessUnits();
  const { data: config } = useConfigLookup(canManage);
  const { data: options, isLoading, error } = useConfigOptionsAdmin();
  const qc = useQueryClient();
  const [actionError, setActionError] = useState<string | null>(null);

  const invalidate = () => qc.invalidateQueries({ queryKey: ["config_options"] });
  const onError = (e: unknown) =>
    setActionError(e instanceof ApiError ? e.message : "Action failed");

  const byKey = useMemo(() => {
    const m: Record<string, ConfigOption[]> = {};
    for (const o of options ?? []) (m[o.list_key] ??= []).push(o);
    for (const k of Object.keys(m)) m[k].sort((a, b) => a.sort_order - b.sort_order);
    return m;
  }, [options]);

  const create = useMutation({
    mutationFn: createConfigOption,
    onSuccess: () => {
      setActionError(null);
      invalidate();
    },
    onError,
  });
  const update = useMutation({
    mutationFn: ({ id, ...rest }: { id: number; value: string; sort_order: number; is_active: boolean }) =>
      updateConfigOption(id, rest),
    onSuccess: () => {
      setActionError(null);
      invalidate();
    },
    onError,
  });
  const remove = useMutation({
    mutationFn: (id: number) => deleteConfigOption(id),
    onSuccess: () => {
      setActionError(null);
      invalidate();
    },
    onError,
  });

  if (!canManage) {
    return (
      <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
        <Card variant="outlined" sx={{ borderStyle: "dashed" }}>
          <CardContent sx={{ p: 4, textAlign: "center" }}>
            <Typography color="text.secondary">
              You need the admin or procurement_admin role to manage settings.
            </Typography>
          </CardContent>
        </Card>
      </Box>
    );
  }

  const lists: ConfigList[] = config?.keys ?? [];

  return (
    <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
      <Typography variant="h5" sx={{ fontWeight: 600 }}>
        Settings
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5, mb: 3 }}>
        Manage the values that appear in the new-request form dropdowns. Deactivating a value keeps
        it on existing requests but hides it from new ones; deleting removes it entirely.
      </Typography>

      {isAdmin && (
        <Box sx={{ mb: 4 }}>
          <StorageSettings />
        </Box>
      )}

      <Box sx={{ mb: 4 }}>
        <TeamsSection />
      </Box>

      <Typography variant="h6" sx={{ fontWeight: 600 }}>
        Dropdown values
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5, mb: 2 }}>
        Values that appear in the new-request form dropdowns.
      </Typography>

      {actionError && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {actionError}
        </Alert>
      )}

      {isLoading && <CircularProgress size={24} />}
      {error && <Alert severity="error">Failed to load settings.</Alert>}

      <Stack spacing={2}>
        {lists.map((list) => (
          <ListSection
            key={list.key}
            list={list}
            options={byKey[list.key] ?? []}
            adding={create.isPending}
            onAdd={(value) => {
              const max = (byKey[list.key] ?? []).reduce((m, o) => Math.max(m, o.sort_order), -1);
              create.mutate({ list_key: list.key, value, sort_order: max + 1, is_active: true });
            }}
            onRename={(o, value) =>
              update.mutate({ id: o.id, value, sort_order: o.sort_order, is_active: o.is_active })
            }
            onToggle={(o) =>
              update.mutate({ id: o.id, value: o.value, sort_order: o.sort_order, is_active: !o.is_active })
            }
            onMove={(o, dir) => {
              const arr = byKey[list.key] ?? [];
              const i = arr.findIndex((x) => x.id === o.id);
              const j = dir === "up" ? i - 1 : i + 1;
              if (j < 0 || j >= arr.length) return;
              const other = arr[j];
              // Swap the two rows' sort_order.
              update.mutate({ id: o.id, value: o.value, sort_order: other.sort_order, is_active: o.is_active });
              update.mutate({ id: other.id, value: other.value, sort_order: o.sort_order, is_active: other.is_active });
            }}
            onDelete={(o) => remove.mutate(o.id)}
          />
        ))}
      </Stack>
    </Box>
  );
}

function ListSection({
  list,
  options,
  adding,
  onAdd,
  onRename,
  onToggle,
  onMove,
  onDelete,
}: {
  list: ConfigList;
  options: ConfigOption[];
  adding: boolean;
  onAdd: (value: string) => void;
  onRename: (o: ConfigOption, value: string) => void;
  onToggle: (o: ConfigOption) => void;
  onMove: (o: ConfigOption, dir: "up" | "down") => void;
  onDelete: (o: ConfigOption) => void;
}) {
  const [draft, setDraft] = useState("");
  const [editId, setEditId] = useState<number | null>(null);
  const [editValue, setEditValue] = useState("");
  // Deleting an option is irreversible; deactivating pulls it out of the pickers.
  const [confirmNode, confirmAction] = useConfirmAction();

  const submitAdd = () => {
    const v = draft.trim();
    if (!v) return;
    onAdd(v);
    setDraft("");
  };

  return (
    <Card variant="outlined">
      <Box
        sx={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          px: 2,
          py: 1.25,
          borderBottom: 1,
          borderColor: "divider",
        }}
      >
        <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>
          {list.label}
        </Typography>
        <Typography variant="caption" color="text.secondary">
          {options.length} value{options.length === 1 ? "" : "s"}
        </Typography>
      </Box>

      <Box>
        {options.length === 0 && (
          <Box sx={{ px: 2, py: 1.5 }}>
            <Typography variant="body2" color="text.secondary">
              No values yet.
            </Typography>
          </Box>
        )}
        {options.map((o, i) => (
          <Box
            key={o.id}
            sx={{
              display: "flex",
              alignItems: "center",
              gap: 1,
              px: 2,
              py: 1,
              borderTop: i === 0 ? 0 : 1,
              borderColor: "divider",
              bgcolor: o.is_active ? "transparent" : "action.hover",
            }}
          >
            <Stack sx={{ alignItems: "center" }}>
              <IconButton
                size="small"
                onClick={() => onMove(o, "up")}
                disabled={i === 0}
                title="Move up"
                sx={{ p: 0.25 }}
              >
                <ChevronUp size={14} />
              </IconButton>
              <IconButton
                size="small"
                onClick={() => onMove(o, "down")}
                disabled={i === options.length - 1}
                title="Move down"
                sx={{ p: 0.25 }}
              >
                <ChevronDown size={14} />
              </IconButton>
            </Stack>

            {editId === o.id ? (
              <TextField
                size="small"
                autoFocus
                value={editValue}
                onChange={(e) => setEditValue(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" && editValue.trim()) {
                    onRename(o, editValue.trim());
                    setEditId(null);
                  } else if (e.key === "Escape") setEditId(null);
                }}
                sx={{ flex: 1 }}
              />
            ) : (
              <Typography
                variant="body2"
                sx={{
                  flex: 1,
                  color: o.is_active ? "text.primary" : "text.disabled",
                  textDecoration: o.is_active ? "none" : "line-through",
                }}
              >
                {o.value}
              </Typography>
            )}

            {!o.is_active && editId !== o.id && (
              <Chip size="small" label="Hidden" />
            )}

            <Stack direction="row" spacing={0.5} sx={{ alignItems: "center" }}>
              {editId === o.id ? (
                <>
                  <Button
                    size="small"
                    variant="outlined"
                    color="inherit"
                    onClick={() => {
                      if (editValue.trim()) onRename(o, editValue.trim());
                      setEditId(null);
                    }}
                  >
                    Save
                  </Button>
                  <Button
                    size="small"
                    variant="outlined"
                    color="inherit"
                    onClick={() => setEditId(null)}
                  >
                    Cancel
                  </Button>
                </>
              ) : (
                <>
                  <Button
                    size="small"
                    variant="outlined"
                    color="inherit"
                    onClick={() => {
                      setEditId(o.id);
                      setEditValue(o.value);
                    }}
                  >
                    Rename
                  </Button>
                  <Button
                    size="small"
                    variant="outlined"
                    color="inherit"
                    onClick={() => {
                      // Reactivating is additive — only guard the removing direction.
                      if (!o.is_active) {
                        onToggle(o);
                        return;
                      }
                      confirmAction({
                        title: "Deactivate option",
                        message: (
                          <>
                            Deactivate <strong>{o.value}</strong>? It stops being offered in{" "}
                            {list.label.toLowerCase()} pickers. Requests that already use it keep the
                            value.
                          </>
                        ),
                        confirmLabel: "Deactivate",
                        onConfirm: () => onToggle(o),
                      });
                    }}
                  >
                    {o.is_active ? "Deactivate" : "Reactivate"}
                  </Button>
                  <Button
                    size="small"
                    variant="outlined"
                    color="error"
                    onClick={() =>
                      confirmAction({
                        title: "Delete option",
                        message: (
                          <>
                            Delete <strong>{o.value}</strong> from {list.label.toLowerCase()}? This
                            cannot be undone — deactivate instead to keep it on record.
                          </>
                        ),
                        confirmLabel: "Delete",
                        onConfirm: () => onDelete(o),
                      })
                    }
                  >
                    Delete
                  </Button>
                </>
              )}
            </Stack>
          </Box>
        ))}
      </Box>

      <Box
        sx={{
          display: "flex",
          alignItems: "center",
          gap: 1,
          px: 2,
          py: 1.25,
          borderTop: 1,
          borderColor: "divider",
        }}
      >
        <TextField
          size="small"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") submitAdd();
          }}
          placeholder={`Add a ${list.label.toLowerCase()} value`}
          sx={{ flex: 1 }}
        />
        <Button
          variant="contained"
          onClick={submitAdd}
          disabled={adding || draft.trim() === ""}
        >
          Add
        </Button>
      </Box>
      {confirmNode}
    </Card>
  );
}
