import { useMemo, useRef, useState } from "react";
import { Box, MenuItem, Paper, TextField, Typography } from "@wso2/oxygen-ui";
import { useRefreshOnNoMatch } from "../hooks/useDirectory";
import { searchPeople } from "../lib/directorySearch";
import type { DirectoryUser } from "../types/api";

interface Props {
  // value is the current email text (fully controlled).
  value: string;
  // onChange fires on every keystroke (picked undefined) and on selecting a
  // suggestion (picked set, so the caller can also fill an associated name field).
  onChange: (email: string, picked?: DirectoryUser) => void;
  // directory is the full candidate list; matches are filtered from it as you type.
  directory: DirectoryUser[];
  // refresh, when provided, forces a fresh directory fetch if the typed prefix
  // matches nothing loaded (a just-added user may not be cached yet).
  refresh?: () => void;
  loading?: boolean;
  disabled?: boolean;
  placeholder?: string;
  id?: string;
  ariaLabel?: string;
  className?: string;
  // inputClassName lets the host form style the input to match its own inputs
  // (the ProQ requisition form uses its own CSS, not Tailwind).
  inputClassName?: string;
}

const noop = () => {};

// EmailAutocomplete is a free-text email field with type-to-search suggestions
// from the org directory. Unlike UserComboBox it does not force a selection — a
// typed email that isn't in the directory is still accepted (people who haven't
// logged in yet). Selecting a suggestion reports the matched directory user so
// the caller can also populate a name field.
export function EmailAutocomplete({
  value,
  onChange,
  directory,
  refresh,
  loading,
  disabled,
  placeholder,
  id,
  ariaLabel,
  className,
  inputClassName,
}: Props) {
  const [open, setOpen] = useState(false);
  const blurTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  const { matches, foundCount } = useMemo(() => {
    const found = searchPeople(directory, value);
    // Hide the dropdown once the text is an exact match (nothing left to pick),
    // but still count it as found so no needless refresh fires.
    if (
      found.foundCount === 1 &&
      found.matches[0]?.email.toLowerCase() === value.trim().toLowerCase()
    ) {
      return { matches: [] as DirectoryUser[], foundCount: 1 };
    }
    return found;
  }, [directory, value]);

  useRefreshOnNoMatch(value, foundCount, refresh ?? noop);

  return (
    <Box className={className} sx={{ position: "relative" }}>
      <TextField
        id={id}
        type="email"
        size="small"
        fullWidth
        value={value}
        disabled={disabled}
        placeholder={loading ? "Loading…" : placeholder}
        autoComplete="off"
        inputProps={{ "aria-label": ariaLabel, className: inputClassName }}
        onChange={(e) => {
          onChange(e.target.value);
          setOpen(true);
        }}
        onFocus={() => value.trim() !== "" && setOpen(true)}
        onBlur={() => {
          // Delay so a suggestion click (mousedown → click) registers first.
          blurTimer.current = setTimeout(() => setOpen(false), 120);
        }}
      />
      {open && matches.length > 0 && (
        <Paper
          variant="outlined"
          sx={{
            position: "absolute",
            zIndex: 20,
            mt: 0.5,
            width: "100%",
            minWidth: 256,
            maxHeight: 224,
            overflow: "auto",
          }}
        >
          {matches.map((u) => (
            <MenuItem
              key={u.email}
              // onMouseDown fires before the input's blur, so the pick lands.
              onMouseDown={(e) => {
                e.preventDefault();
                if (blurTimer.current) clearTimeout(blurTimer.current);
                onChange(u.email, u);
                setOpen(false);
              }}
              sx={{ display: "flex", flexDirection: "column", alignItems: "flex-start" }}
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
          ))}
        </Paper>
      )}
    </Box>
  );
}
