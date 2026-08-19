import { apiFetch } from "./client";

export interface SessionResponse {
  /** ISO timestamp at which the session dies if left idle until then. */
  expires_at: string;
}

/**
 * Exchanges the current IdP token for the backend's own session cookie
 * (see backend/internal/handler/auth_session.go, docs/sessions.md).
 *
 * This is the one call that still needs the OIDC token. Everything after it
 * authenticates with the HttpOnly cookie the backend sets here, which is why
 * the ID token expiring — or the browser being restarted — no longer signs the
 * user out. The cookie is set by the server and is never visible to this code.
 */
export const startSession = () => apiFetch<SessionResponse>("/api/v1/auth/session", { method: "POST" });

/**
 * Ends the current session server-side and clears the cookie. Called on sign
 * out — clearing the cookie alone would leave a token that still works for the
 * rest of its lifetime if it had already been captured.
 */
export const endSession = () => apiFetch<void>("/api/v1/auth/session", { method: "DELETE" });

/** Ends every session this user has, on every device ("sign out everywhere"). */
export const endAllSessions = () =>
  apiFetch<{ revoked: number }>("/api/v1/auth/sessions", { method: "DELETE" });
