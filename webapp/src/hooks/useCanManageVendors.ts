import { useMe } from "./useMe";

// useCanManageVendors reports whether the signed-in user may manage the vendor
// master from the dedicated Vendors page. Only admin / procurement_admin qualify.
// UX-only — the backend enforces the same rule (middleware.HasVendorAdmin).
export function useCanManageVendors(): boolean {
  const { data: me } = useMe();
  return !!me?.roles.some((r) => r === "admin" || r === "procurement_admin");
}
