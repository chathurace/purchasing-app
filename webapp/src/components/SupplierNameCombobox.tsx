import { useEffect, useRef, useState } from "react";
import { Box, IconButton, InputAdornment, MenuItem, Paper, TextField, Typography } from "@wso2/oxygen-ui";
import { ChevronDown } from "@wso2/oxygen-ui-icons-react";
import type { VendorLookup } from "../types/api";

interface Props {
  value: string;
  options: VendorLookup[];
  // onType fires for free-typed text (no vendor selected from the list).
  onType: (name: string) => void;
  // onSelect fires when an existing vendor is chosen from the dropdown.
  onSelect: (vendor: VendorLookup) => void;
}

// SupplierNameCombobox is an editable dropdown over the vendor master: the user
// can pick an existing vendor (which auto-fills the rest of the supplier
// fields) or type a brand-new supplier name. Unlike a native <datalist> it
// renders a visible, clickable dropdown in every browser.
export function SupplierNameCombobox({ value, options, onType, onSelect }: Props) {
  const [open, setOpen] = useState(false);
  const wrapRef = useRef<HTMLDivElement>(null);

  // Close the dropdown on any click outside the component.
  useEffect(() => {
    if (!open) return;
    const onDocClick = (e: MouseEvent) => {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDocClick);
    return () => document.removeEventListener("mousedown", onDocClick);
  }, [open]);

  const q = value.trim().toLowerCase();
  const filtered = q
    ? options.filter((v) => v.name.toLowerCase().includes(q))
    : options;

  return (
    <Box ref={wrapRef} sx={{ position: "relative" }}>
      <TextField
        size="small"
        fullWidth
        value={value}
        onChange={(e) => {
          onType(e.target.value);
          setOpen(true);
        }}
        onFocus={() => setOpen(true)}
        placeholder="Search vendors or type a new supplier"
        autoComplete="off"
        inputProps={{ role: "combobox", "aria-expanded": open }}
        InputProps={{
          endAdornment: (
            <InputAdornment position="end">
              <IconButton
                size="small"
                tabIndex={-1}
                edge="end"
                onClick={() => setOpen((o) => !o)}
                aria-label="Toggle vendor list"
              >
                <ChevronDown size={16} />
              </IconButton>
            </InputAdornment>
          ),
        }}
      />

      {open && (
        <Paper
          variant="outlined"
          sx={{
            position: "absolute",
            zIndex: 20,
            mt: 0.5,
            width: "100%",
            maxHeight: 240,
            overflow: "auto",
            py: 0.5,
          }}
        >
          {filtered.length === 0 ? (
            <Box sx={{ px: 1.5, py: 1 }}>
              <Typography variant="body2" color="text.secondary">
                No matching vendors — “{value.trim() || "…"}” will be added as a new supplier.
              </Typography>
            </Box>
          ) : (
            filtered.map((v) => (
              <MenuItem
                key={v.id}
                onClick={() => {
                  onSelect(v);
                  setOpen(false);
                }}
                sx={{ display: "flex", justifyContent: "space-between", gap: 1 }}
              >
                <Typography variant="body2" color="text.primary">
                  {v.name}
                </Typography>
                {!v.registered && (
                  <Typography variant="caption" color="text.secondary" sx={{ flexShrink: 0 }}>
                    unregistered
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
