import { apiFetch } from "./client";
import type { HomeResponse } from "../types/api";

// getHome loads the role-based home dashboard (count tiles + recent activity),
// scoped to the caller's roles on the backend.
export const getHome = () => apiFetch<HomeResponse>("/api/v1/home");
