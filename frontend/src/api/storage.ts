import { apiFetch } from "./client";

// StorageStatus mirrors storage.Status on the backend.
export interface StorageStatus {
  backend: string;
  configured: boolean;
  healthy: boolean;
  base_folder_id?: string;
  base_folder_name?: string;
  account_email?: string;
  last_error?: string;
}

// StorageStatusResponse is the /storage/status and mutation response shape.
export interface StorageStatusResponse {
  status: StorageStatus;
  oauth_client_configured: boolean;
  secret_key_configured: boolean;
}

// StorageParams are the non-secret bits the browser needs for the Google flows.
export interface StorageParams {
  client_id: string;
  api_key: string;
  app_id: string;
  connected: boolean;
  account_email: string;
}

export const getStorageStatus = () =>
  apiFetch<StorageStatusResponse>("/api/v1/storage/status");

export const getStorageParams = () =>
  apiFetch<StorageParams>("/api/v1/storage/params");

export const connectStorage = (code: string) =>
  apiFetch<StorageStatusResponse>("/api/v1/storage/connect", {
    method: "POST",
    body: { code },
  });

export const setStorageFolder = (base_folder_id: string, name: string) =>
  apiFetch<StorageStatusResponse>("/api/v1/storage/folder", {
    method: "POST",
    body: { base_folder_id, name },
  });
