import { getAccessToken } from "../auth/userManager";

const BASE_URL = import.meta.env.VITE_API_BASE_URL || "";

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
  const token = await getAccessToken();
  const headers = new Headers(init.headers);
  if (token) headers.set("Authorization", `Bearer ${token}`);

  let body = init.body as BodyInit | undefined;
  if (body !== undefined && !(body instanceof FormData)) {
    headers.set("Content-Type", "application/json");
    body = JSON.stringify(init.body);
  }

  const res = await fetch(BASE_URL + path, { ...init, headers, body });

  if (!res.ok) {
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
  const res = await fetch(BASE_URL + path, { headers });
  if (!res.ok) throw new ApiError(res.status, `Download failed (${res.status})`, null);

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
