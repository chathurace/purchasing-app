import { useMe } from "./useMe";

// useCanManageBudgetUnits reports whether the signed-in user may manage the
// budget-unit master (and the requisition-form config options) from their
// dedicated admin pages. Only admin / procurement_admin qualify. UX-only — the
// backend enforces the same rule (middleware.HasBudgetUnitAdmin).
export function useCanManageBudgetUnits(): boolean {
  const { data: me } = useMe();
  return !!me?.roles.some((r) => r === "admin" || r === "procurement_admin");
}
