import { useCallback, useEffect, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError } from "../api/client";
import {
  connectStorage,
  getStorageParams,
  getStorageStatus,
  setStorageFolder,
  type StorageParams,
} from "../api/storage";

const DRIVE_SCOPE = "https://www.googleapis.com/auth/drive.file";

// The Google Identity Services and Picker libraries attach untyped globals.
declare global {
  interface Window {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    gapi: any;
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    google: any;
  }
}

// loadScript injects a <script> once and resolves when it has loaded.
function loadScript(src: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const existing = document.querySelector(`script[src="${src}"]`);
    if (existing) {
      if (existing.getAttribute("data-loaded")) return resolve();
      existing.addEventListener("load", () => resolve());
      existing.addEventListener("error", () => reject(new Error(`load ${src}`)));
      return;
    }
    const s = document.createElement("script");
    s.src = src;
    s.async = true;
    s.onload = () => {
      s.setAttribute("data-loaded", "1");
      resolve();
    };
    s.onerror = () => reject(new Error(`load ${src}`));
    document.head.appendChild(s);
  });
}

async function loadGoogle(): Promise<void> {
  await Promise.all([
    loadScript("https://apis.google.com/js/api.js"),
    loadScript("https://accounts.google.com/gsi/client"),
  ]);
  await new Promise<void>((resolve) => window.gapi.load("picker", resolve));
}

// StorageSettings lets an admin connect a Google account and pick the base folder
// for Drive file storage, hot-swapping the live store. Admin-only (also enforced
// server-side).
export function StorageSettings() {
  const qc = useQueryClient();
  const status = useQuery({ queryKey: ["storage_status"], queryFn: getStorageStatus });
  const params = useQuery({ queryKey: ["storage_params"], queryFn: getStorageParams });
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState<null | "connect" | "folder">(null);
  const [googleReady, setGoogleReady] = useState(false);

  useEffect(() => {
    loadGoogle()
      .then(() => setGoogleReady(true))
      .catch(() => setErr("Failed to load Google libraries (check your network / CSP)."));
  }, []);

  const refresh = () => {
    qc.invalidateQueries({ queryKey: ["storage_status"] });
    qc.invalidateQueries({ queryKey: ["storage_params"] });
  };
  const onErr = (e: unknown) => setErr(e instanceof ApiError ? e.message : e instanceof Error ? e.message : "Action failed");

  const g = () => window.google;

  const connect = useCallback(() => {
    const p = params.data;
    if (!p) return;
    setErr(null);
    setBusy("connect");
    const codeClient = g().accounts.oauth2.initCodeClient({
      client_id: p.client_id,
      scope: DRIVE_SCOPE,
      ux_mode: "popup",
      callback: async (resp: { code?: string; error?: string }) => {
        try {
          if (resp.error) throw new Error(resp.error);
          if (!resp.code) throw new Error("no authorization code returned");
          await connectStorage(resp.code);
          refresh();
        } catch (e) {
          onErr(e);
        } finally {
          setBusy(null);
        }
      },
    });
    codeClient.requestCode();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [params.data]);

  const openPicker = (p: StorageParams, token: string) => {
    const gg = g();
    const mkView = () =>
      new gg.picker.DocsView(gg.picker.ViewId.FOLDERS).setSelectFolderEnabled(true).setIncludeFolders(true);
    const myDrive = mkView().setOwnedByMe(true);
    const shared = mkView().setEnableDrives(true);
    new gg.picker.PickerBuilder()
      .enableFeature(gg.picker.Feature.SUPPORT_DRIVES)
      .setAppId(p.app_id)
      .setDeveloperKey(p.api_key)
      .setOAuthToken(token)
      .addView(myDrive)
      .addView(shared)
      .setTitle("Pick the storage folder")
      .setCallback(async (data: { action: string; docs?: Array<{ id: string; name?: string }> }) => {
        if (data.action === gg.picker.Action.CANCEL) {
          setBusy(null);
          return;
        }
        if (data.action !== gg.picker.Action.PICKED) return;
        try {
          const f = data.docs![0];
          await setStorageFolder(f.id, f.name ?? "");
          refresh();
        } catch (e) {
          onErr(e);
        } finally {
          setBusy(null);
        }
      })
      .build()
      .setVisible(true);
  };

  const chooseFolder = useCallback(() => {
    const p = params.data;
    if (!p) return;
    setErr(null);
    setBusy("folder");
    const tokenClient = g().accounts.oauth2.initTokenClient({
      client_id: p.client_id,
      scope: DRIVE_SCOPE,
      callback: (resp: { access_token?: string; error?: string }) => {
        if (resp.error || !resp.access_token) {
          onErr(new Error(resp.error ?? "authorization failed"));
          setBusy(null);
          return;
        }
        openPicker(p, resp.access_token);
      },
    });
    tokenClient.requestAccessToken({ prompt: "" });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [params.data]);

  const resp = status.data;
  const st = resp?.status;
  const p = params.data;
  const prereqsOk = !!resp?.oauth_client_configured && !!resp?.secret_key_configured;
  const canConnect = googleReady && prereqsOk && !!p && !busy;
  const connected = !!p?.connected;

  return (
    <section className="rounded border bg-white p-5">
      <h2 className="text-lg font-semibold text-gray-900">File storage (Google Drive)</h2>
      <p className="mt-1 text-sm text-gray-500">
        Documents (quotations, contracts, invoices…) are stored in a Google Drive folder. Connect a
        Google account and choose the folder — changes take effect immediately.
      </p>

      {err && (
        <div className="mt-3 rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{err}</div>
      )}

      {resp && !prereqsOk && (
        <div className="mt-3 rounded border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800">
          Storage can't be configured from here until the deployment sets{" "}
          {!resp.oauth_client_configured && <code>storage.gdrive.oauth.client_id/client_secret/api_key</code>}
          {!resp.oauth_client_configured && !resp.secret_key_configured && " and "}
          {!resp.secret_key_configured && <code>security.secret_key</code>} in <code>config.yaml</code>.
        </div>
      )}

      {/* Status card */}
      <dl className="mt-4 grid grid-cols-[max-content_1fr] gap-x-6 gap-y-1 text-sm">
        <dt className="text-gray-500">Status</dt>
        <dd>
          {st?.configured && st?.healthy ? (
            <span className="font-medium text-green-700">● Connected &amp; healthy</span>
          ) : st?.last_error ? (
            <span className="font-medium text-red-700">● Not working</span>
          ) : (
            <span className="font-medium text-gray-500">● Not configured</span>
          )}
        </dd>
        <dt className="text-gray-500">Account</dt>
        <dd className="text-gray-800">{p?.account_email || st?.account_email || "—"}</dd>
        <dt className="text-gray-500">Folder</dt>
        <dd className="text-gray-800">
          {st?.base_folder_name || st?.base_folder_id || "—"}
        </dd>
        {st?.last_error && (
          <>
            <dt className="text-gray-500">Detail</dt>
            <dd className="text-red-700">{st.last_error}</dd>
          </>
        )}
      </dl>

      <div className="mt-4 flex flex-wrap gap-3">
        <button
          type="button"
          onClick={connect}
          disabled={!canConnect}
          className="rounded bg-gray-900 px-3 py-2 text-sm font-medium text-white disabled:opacity-40"
        >
          {busy === "connect" ? "Connecting…" : connected ? "Reconnect account" : "Connect Google account"}
        </button>
        <button
          type="button"
          onClick={chooseFolder}
          disabled={!canConnect || !connected}
          className="rounded border border-gray-300 px-3 py-2 text-sm font-medium text-gray-800 disabled:opacity-40"
        >
          {busy === "folder" ? "Opening…" : "Choose folder"}
        </button>
      </div>

      <p className="mt-4 rounded border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800">
        <strong>Note:</strong> changing the folder or account does not move existing files. Documents
        stored in a previous folder will no longer be downloadable through the app.
      </p>
    </section>
  );
}
