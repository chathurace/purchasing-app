import { useQuery } from "@tanstack/react-query";
import { getHome } from "../api/home";

// useHome powers the role-based home page. Polls like the list hooks so counts and
// activity stay fresh.
export function useHome() {
  return useQuery({
    queryKey: ["home"],
    queryFn: getHome,
    refetchInterval: 5000,
  });
}
