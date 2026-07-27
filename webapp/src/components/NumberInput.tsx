import { useEffect, useRef, useState, type FocusEvent, type InputHTMLAttributes } from "react";
import { TextField } from "@wso2/oxygen-ui";

type Props = Omit<InputHTMLAttributes<HTMLInputElement>, "value" | "onChange" | "type"> & {
  value: number;
  onChange: (value: number) => void;
  /** Number emitted when the field is cleared. Defaults to 0. */
  emptyValue?: number;
};

/**
 * Controlled numeric input that is fully keyboard-editable.
 *
 * A plain `<input type="number" value={n} onChange={e => Number(e.target.value)}>`
 * is sticky: clearing the field yields `Number("")` === 0, which immediately
 * re-renders the "0" back, so the existing digit can only be nudged with the
 * spinner arrows. This keeps a raw text buffer while the field is focused so the
 * user can clear it and type the whole number, committing the parsed value up.
 */
export function NumberInput({ value, onChange, emptyValue = 0, ...rest }: Props) {
  const [text, setText] = useState(() => String(value));
  const focused = useRef(false);

  // Mirror external value changes (form reset, programmatic updates) into the
  // buffer, but never while the user is actively typing.
  useEffect(() => {
    if (!focused.current) setText(String(value));
  }, [value]);

  const { onFocus, onBlur, className, ...inputRest } = rest;

  return (
    <TextField
      type="number"
      size="small"
      className={className}
      value={text}
      onChange={(e) => {
        const next = e.target.value;
        setText(next);
        if (next === "") {
          onChange(emptyValue);
        } else {
          const n = Number(next);
          if (Number.isFinite(n)) onChange(n);
        }
      }}
      inputProps={{
        ...inputRest,
        onFocus: (e: FocusEvent<HTMLInputElement>) => {
          focused.current = true;
          onFocus?.(e);
        },
        onBlur: (e: FocusEvent<HTMLInputElement>) => {
          focused.current = false;
          // Settle the display to the committed value (drops a trailing "."
          // or a stray leading zero, restores "0" if left empty).
          setText(String(value));
          onBlur?.(e);
        },
      }}
    />
  );
}

type NullableProps = Omit<InputHTMLAttributes<HTMLInputElement>, "value" | "onChange" | "type"> & {
  value: number | null;
  onChange: (value: number | null) => void;
};

/**
 * Like {@link NumberInput}, but an empty field is a meaningful `null` rather than
 * `0` — used for optional numbers (e.g. an entered invoice total that, when left
 * blank, falls back to a derived value). Same raw-buffer trick so the field can
 * be fully cleared and retyped.
 */
export function NullableNumberInput({ value, onChange, ...rest }: NullableProps) {
  const [text, setText] = useState(() => (value == null ? "" : String(value)));
  const focused = useRef(false);

  useEffect(() => {
    if (!focused.current) setText(value == null ? "" : String(value));
  }, [value]);

  const { onFocus, onBlur, className, ...inputRest } = rest;

  return (
    <TextField
      type="number"
      size="small"
      className={className}
      value={text}
      onChange={(e) => {
        const next = e.target.value;
        setText(next);
        if (next === "") {
          onChange(null);
        } else {
          const n = Number(next);
          if (Number.isFinite(n)) onChange(n);
        }
      }}
      inputProps={{
        ...inputRest,
        onFocus: (e: FocusEvent<HTMLInputElement>) => {
          focused.current = true;
          onFocus?.(e);
        },
        onBlur: (e: FocusEvent<HTMLInputElement>) => {
          focused.current = false;
          setText(value == null ? "" : String(value));
          onBlur?.(e);
        },
      }}
    />
  );
}
