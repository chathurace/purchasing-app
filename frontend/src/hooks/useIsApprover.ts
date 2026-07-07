import { useMe } from "./useMe";

// useIsApprover reports whether the signed-in user is an approver on any PR they
// didn't submit (server-computed is_approver flag). Drives the approver-only nav
// tabs. UX-only — the backend enforces access on each endpoint.
export function useIsApprover(): boolean {
  const { data: me } = useMe();
  return !!me?.is_approver;
}
