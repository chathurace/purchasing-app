import { useMe } from "./useMe";

// useCanManageBusinessUnits reports whether the signed-in user may manage the
// business-unit master (and the requisition-form config options) from their
// dedicated admin pages. Only admin / procurement_admin qualify. UX-only — the
// backend enforces the same rule (middleware.HasBusinessUnitAdmin).
export function useCanManageBusinessUnits(): boolean {
  const { data: me } = useMe();
  return !!me?.roles.some((r) => r === "admin" || r === "procurement_admin");
}
