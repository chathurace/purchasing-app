import { getAccessToken } from "../auth/userManager";

// Runtime window.config (Choreo file mount) wins over the build-time VITE_ var.
const BASE_URL = window.config?.apiBaseUrl ?? import.meta.env.VITE_API_BASE_URL ?? "";

/**
 * Whether to send the backend session cookie with API requests
 * (docs/sessions.md).
 *
 * `include` is required for the cookie to be sent at all, and it also makes the
 * browser *require* `Access-Control-Allow-Credentials` on every cross-origin
 * response — so switching it on where the API gateway does not return that
 * header breaks every request rather than just skipping the cookie. It is
 * therefore on exactly where it is known to work:
 *
 *   - a same-origin or same-host API (`apiBaseUrl` empty/relative, or
 *     http://localhost:8081 from http://localhost:5173 in dev): same site, so a
 *     SameSite=Lax cookie is sent and our own CORS middleware allows credentials
 *   - a cross-site API only on explicit opt-in via config.js
 *     `apiAllowCredentials: true`, which also needs the backend on
 *     `session.cookie_samesite: none` and the gateway configured to allow
 *     credentials
 *
 * Otherwise requests fall back to Bearer tokens exactly as before, so deploying
 * this change is harmless on a cross-site setup that has not been prepared.
 */
export function apiCredentials(): RequestCredentials {
  if (typeof window.config?.apiAllowCredentials === "boolean") {
    return window.config.apiAllowCredentials ? "include" : "same-origin";
  }
  if (BASE_URL === "" || BASE_URL.startsWith("/")) return "include";
  try {
    // Same host ⇒ same site (the port is irrelevant to SameSite), which is the
    // dev setup and any reverse-proxied deployment.
    if (new URL(BASE_URL, window.location.href).hostname === window.location.hostname) return "include";
  } catch {
    // Malformed base URL — fall through to the safe default.
  }
  return "same-origin";
}

/** True when the backend session cookie can actually be used. */
export const sessionCookiesUsable = () => apiCredentials() === "include";

export class ApiError extends Error {
  status: number;
  body: unknown;
  constructor(status: number, message: string, body: unknown) {
    super(message);
    this.status = status;
    this.body = body;
  }
}

type FetchInit = Omit<RequestInit, "body"> & { body?: unknown };

export async function apiFetch<T>(path: string, init: FetchInit = {}): Promise<T> {
  // The Bearer token is normally absent — the session cookie is the credential.
  // It is still attached when this tab holds a live one, because that token is
  // what POST /api/v1/auth/session uses to mint the cookie in the first place,
  // and it is the fallback wherever cookies are unusable (see apiCredentials).
  const token = await getAccessToken();
  const headers = new Headers(init.headers);
  if (token) headers.set("Authorization", `Bearer ${token}`);

  let body = init.body as BodyInit | undefined;
  if (body !== undefined && !(body instanceof FormData)) {
    headers.set("Content-Type", "application/json");
    body = JSON.stringify(init.body);
  }

  const res = await fetch(BASE_URL + path, { ...init, credentials: apiCredentials(), headers, body });

  if (!res.ok) {
    // A 401 means the backend session cookie is gone, expired or revoked (or,
    // in the Bearer fallback, that the token is dead) — the app must ask for a
    // fresh sign-in. AuthContext listens for this; it ignores the expected 401
    // from its own boot-time probe.
    if (res.status === 401) {
      window.dispatchEvent(new CustomEvent("session-expired"));
    }
    let parsed: unknown = null;
    let message = `Request failed (${res.status})`;
    try {
      parsed = await res.json();
      if (parsed && typeof parsed === "object" && "error" in parsed) {
        message = String((parsed as { error: unknown }).error);
      }
    } catch {
      /* non-JSON error body */
    }
    throw new ApiError(res.status, message, parsed);
  }

  if (res.status === 204) return undefined as T;
  const ct = res.headers.get("Content-Type") ?? "";
  if (ct.includes("application/json")) return (await res.json()) as T;
  return undefined as T;
}

// downloadUrl builds an absolute URL for streamed file responses.
export function apiUrl(path: string): string {
  return BASE_URL + path;
}

// downloadFile fetches a protected file and triggers a browser download,
// preserving the server-provided filename.
export async function downloadFile(path: string, fallbackName: string): Promise<void> {
  const token = await getAccessToken();
  const headers = new Headers();
  if (token) headers.set("Authorization", `Bearer ${token}`);
  const res = await fetch(BASE_URL + path, { credentials: apiCredentials(), headers });
  if (!res.ok) {
    if (res.status === 401) window.dispatchEvent(new CustomEvent("session-expired"));
    throw new ApiError(res.status, `Download failed (${res.status})`, null);
  }

  const blob = await res.blob();
  let filename = fallbackName;
  const cd = res.headers.get("Content-Disposition");
  const match = cd?.match(/filename="?([^"]+)"?/);
  if (match) filename = match[1];

  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}
