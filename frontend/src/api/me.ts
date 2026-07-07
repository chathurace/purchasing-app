import { apiFetch } from "./client";
import type { Me } from "../types/api";

export const getMe = () => apiFetch<Me>("/api/v1/me");
