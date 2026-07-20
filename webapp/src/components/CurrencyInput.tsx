import { useId } from "react";
import { REQ_CURRENCIES } from "../types/api";

interface Props {
  value: string;
  onChange: (value: string) => void;
  className?: string;
  placeholder?: string;
  /** Cap the length (currency codes are 3 chars). Omit for no limit. */
  maxLength?: number;
  /** Dropdown suggestions; defaults to the common set (REQ_CURRENCIES). Any value
   *  may still be typed regardless. */
  options?: string[];
}

// CurrencyInput is an editable combobox for currency codes: a native <datalist>
// dropdown suggesting the common set (USD, LKR, GBP, AUD, INR, EUR — REQ_CURRENCIES)
// while still letting the user type any value. Input is upper-cased so codes stay
// canonical. Used everywhere a currency is entered so the behaviour is consistent.
export function CurrencyInput({
  value,
  onChange,
  className,
  placeholder = "e.g. USD",
  maxLength,
  options = REQ_CURRENCIES,
}: Props) {
  const listId = useId();
  return (
    <>
      <input
        list={listId}
        className={className}
        value={value}
        maxLength={maxLength}
        onChange={(e) => onChange(e.target.value.toUpperCase())}
        placeholder={placeholder}
      />
      <datalist id={listId}>
        {options.map((c) => (
          <option key={c} value={c} />
        ))}
      </datalist>
    </>
  );
}
