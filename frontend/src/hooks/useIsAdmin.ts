import { useMe } from "./useMe";

// useIsAdmin reports whether the signed-in user is an admin. UX-only — the
// backend enforces the same rule on every user-management endpoint.
export function useIsAdmin(): boolean {
  const { data: me } = useMe();
  return !!me?.roles.includes("admin");
}
