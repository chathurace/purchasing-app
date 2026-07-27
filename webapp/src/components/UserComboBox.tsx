import { useMemo, useRef, useState } from "react";
import { Box, IconButton, InputAdornment, MenuItem, Paper, TextField, Typography } from "@wso2/oxygen-ui";
import { X } from "@wso2/oxygen-ui-icons-react";
import type { UserSummary } from "../types/api";

interface Props {
  // users is the full candidate list; matches are filtered from it as you type.
  users: UserSummary[];
  // value is the selected user id, or "" for no selection (controlled).
  value: number | "";
  onChange: (id: number | "") => void;
  placeholder?: string;
  loading?: boolean;
  ariaLabel?: string;
  className?: string;
  // label renders a field label above/inside the box (MUI outlined label),
  // naming the filter even after a value is picked.
  label?: string;
}

function label(u: UserSummary): string {
  return u.name ? `${u.name} (${u.email})` : u.email || `#${u.id}`;
}

// UserComboBox is a single-select, type-to-search field: you type a name/email,
// pick a suggestion, and it reports the chosen user id. Clearing the text (or the
// × button) reports "" (no filter). Remount (via a changing `key`) resets it —
// which is how the page's "Clear filters" button wipes it.
export function UserComboBox({
  users,
  value,
  onChange,
  placeholder,
  loading,
  ariaLabel,
  className,
  label: fieldLabel,
}: Props) {
  const selected = value ? users.find((u) => u.id === value) : undefined;
  // query holds the text in the box. When a user is selected its label is shown;
  // otherwise it's whatever the user has typed so far.
  const [query, setQuery] = useState(selected ? label(selected) : "");
  const [open, setOpen] = useState(false);
  const blurTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  const matches = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (q === "") return [];
    return users
      .filter((u) => u.email.toLowerCase().includes(q) || u.name.toLowerCase().includes(q))
      .slice(0, 8);
  }, [users, query]);

  const clear = () => {
    setQuery("");
    setOpen(false);
    onChange("");
  };

  return (
    <Box className={className} sx={{ position: "relative", width: 224 }}>
      <TextField
        size="small"
        fullWidth
        label={fieldLabel}
        // Keep the label lifted: the placeholder would otherwise sit under an
        // un-shrunk label and overlap it while the box is empty and unfocused.
        InputLabelProps={fieldLabel ? { shrink: true } : undefined}
        value={query}
        placeholder={loading ? "Loading…" : placeholder}
        inputProps={{ "aria-label": ariaLabel }}
        onChange={(e) => {
          setQuery(e.target.value);
          setOpen(true);
          // Typing invalidates any prior selection until a new one is picked.
          if (value) onChange("");
        }}
        onFocus={() => query.trim() !== "" && setOpen(true)}
        onBlur={() => {
          // Delay so a suggestion click (mousedown → click) registers first.
          blurTimer.current = setTimeout(() => setOpen(false), 120);
        }}
        InputProps={{
          endAdornment:
            query !== "" ? (
              <InputAdornment position="end">
                <IconButton size="small" onClick={clear} aria-label="Clear" edge="end">
                  <X size={16} />
                </IconButton>
              </InputAdornment>
            ) : undefined,
        }}
      />

      {open && query.trim() !== "" && (
        <Paper
          variant="outlined"
          sx={{
            position: "absolute",
            zIndex: 10,
            mt: 0.5,
            width: 288,
            maxHeight: 224,
            overflow: "auto",
          }}
        >
          {matches.length === 0 ? (
            <Box sx={{ px: 1.5, py: 1 }}>
              <Typography variant="body2" color="text.secondary">
                No matching users.
              </Typography>
            </Box>
          ) : (
            matches.map((u) => (
              <MenuItem
                key={u.id}
                // onMouseDown fires before the input's blur, so the pick lands.
                onMouseDown={(e) => {
                  e.preventDefault();
                  if (blurTimer.current) clearTimeout(blurTimer.current);
                  setQuery(label(u));
                  setOpen(false);
                  onChange(u.id);
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
