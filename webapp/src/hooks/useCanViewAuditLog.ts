import { useMe } from "./useMe";

// useCanViewAuditLog reports whether the signed-in user may open the Audit log
// page (the read view over the process/audit event logs). Only admin /
// procurement_admin qualify — the log includes sensitive actions (role grants,
// deactivations). UX-only — the backend enforces the same rule
// (EventsHandler.requireAccess).
export function useCanViewAuditLog(): boolean {
  const { data: me } = useMe();
  return !!me?.roles.some((r) => r === "admin" || r === "procurement_admin");
}
