import { useMe } from "./useMe";

// useFinanceAccess reports whether the signed-in user may perform procurement
// actions. UX-only — the backend enforces the same rule.
export function useFinanceAccess(): boolean {
  const { data: me } = useMe();
  return !!me?.roles.some(
    (r) => r === "finance" || r === "finance_admin" || r === "admin",
  );
}
