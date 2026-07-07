import { useEffect, useRef, useState } from "react";
import type { VendorLookup } from "../types/api";

const inputCls =
  "w-full rounded border px-3 py-2 pr-9 text-sm focus:border-indigo-500 focus:outline-none";

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
    <div ref={wrapRef} className="relative">
      <input
        className={inputCls}
        value={value}
        onChange={(e) => {
          onType(e.target.value);
          setOpen(true);
        }}
        onFocus={() => setOpen(true)}
        placeholder="Search vendors or type a new supplier"
        role="combobox"
        aria-expanded={open}
        autoComplete="off"
      />
      <button
        type="button"
        tabIndex={-1}
        onClick={() => setOpen((o) => !o)}
        className="absolute inset-y-0 right-0 flex items-center px-2 text-gray-400 hover:text-gray-600"
        aria-label="Toggle vendor list"
      >
        <svg width="16" height="16" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
          <path
            fillRule="evenodd"
            d="M5.23 7.21a.75.75 0 011.06.02L10 11.06l3.71-3.83a.75.75 0 111.08 1.04l-4.25 4.39a.75.75 0 01-1.08 0L5.21 8.27a.75.75 0 01.02-1.06z"
            clipRule="evenodd"
          />
        </svg>
      </button>

      {open && (
        <ul className="absolute z-20 mt-1 max-h-60 w-full overflow-auto rounded border bg-white py-1 text-sm shadow-lg">
          {filtered.length === 0 ? (
            <li className="px-3 py-2 text-gray-400">
              No matching vendors — “{value.trim() || "…"}” will be added as a new supplier.
            </li>
          ) : (
            filtered.map((v) => (
              <li key={v.id}>
                <button
                  type="button"
                  onClick={() => {
                    onSelect(v);
                    setOpen(false);
                  }}
                  className="flex w-full items-center justify-between gap-2 px-3 py-2 text-left hover:bg-indigo-50"
                >
                  <span className="text-gray-900">{v.name}</span>
                  {!v.registered && (
                    <span className="shrink-0 text-xs text-gray-400">unregistered</span>
                  )}
                </button>
              </li>
            ))
          )}
        </ul>
      )}
    </div>
  );
}
