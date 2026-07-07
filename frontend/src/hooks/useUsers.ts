import { useQuery } from "@tanstack/react-query";
import { listUsers } from "../api/users";

export function useUsers(enabled = true) {
  return useQuery({
    queryKey: ["users"],
    queryFn: listUsers,
    enabled,
    refetchInterval: 5000,
  });
}
