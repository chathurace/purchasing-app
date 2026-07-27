import { useCallback, useEffect, useRef } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { listDirectory } from "../api/users";
import type { DirectoryUser } from "../types/api";

const KEY = ["users", "directory"];

// useDirectory loads the org user directory (email/name) from the connected
// identity server for name/email autocomplete. The full list is fetched once and
// cached; the pickers filter it client-side. `refresh` forces a fresh SCIM fetch
// (rate-limited server-side) and updates the cache — call it when a typed prefix
// matched nothing, in case a newly-added directory user isn't cached yet.
export function useDirectory(enabled = true) {
  const qc = useQueryClient();
  const query = useQuery({
    queryKey: KEY,
    queryFn: () => listDirectory(false),
    enabled,
    staleTime: 5 * 60 * 1000,
  });
  const refreshing = useRef(false);
  const refresh = useCallback(async () => {
    if (refreshing.current) return;
    refreshing.current = true;
    try {
      const fresh = await listDirectory(true);
      qc.setQueryData<DirectoryUser[]>(KEY, fresh);
    } catch {
      // A failed forced refresh is non-fatal — keep whatever's cached.
    } finally {
      refreshing.current = false;
    }
  }, [qc]);
  return { ...query, refresh };
}

// useRefreshOnNoMatch triggers a one-time directory refresh (debounced) when the
// current query is non-trivial yet matches nothing loaded — so a just-added
// directory user can still be found. It fires at most once per distinct query.
export function useRefreshOnNoMatch(
  query: string,
  matchCount: number,
  refresh: () => void,
) {
  const lastTried = useRef("");
  useEffect(() => {
    const q = query.trim().toLowerCase();
    if (q.length < 2 || matchCount > 0 || lastTried.current === q) return;
    const t = setTimeout(() => {
      lastTried.current = q;
      refresh();
    }, 350);
    return () => clearTimeout(t);
  }, [query, matchCount, refresh]);
}
