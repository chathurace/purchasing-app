import { useQuery } from "@tanstack/react-query";
import { lookupUsers } from "../api/users";

// useUserLookup loads the active-user directory for the approver picker. Any
// authenticated user may call it; results are id/email/name only.
export function useUserLookup(enabled = true) {
  return useQuery({
    queryKey: ["users", "lookup"],
    queryFn: lookupUsers,
    enabled,
  });
}
