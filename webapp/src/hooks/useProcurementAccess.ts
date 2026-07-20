import { useMe } from "./useMe";

// useProcurementAccess reports whether the signed-in user may perform procurement
// actions. UX-only — the backend enforces the same rule.
export function useProcurementAccess(): boolean {
  const { data: me } = useMe();
  return !!me?.roles.some(
    (r) => r === "procurement" || r === "procurement_admin" || r === "admin",
  );
}
