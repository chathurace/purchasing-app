import { useMe } from "./useMe";

// useCanManageCostCenters reports whether the signed-in user may manage the cost
// center master from the dedicated Cost centers page. Only admin / finance_admin
// qualify. UX-only — the backend enforces the same rule
// (middleware.HasCostCenterAdmin).
export function useCanManageCostCenters(): boolean {
  const { data: me } = useMe();
  return !!me?.roles.some((r) => r === "admin" || r === "finance_admin");
}
