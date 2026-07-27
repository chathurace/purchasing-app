import { useCallback, useEffect, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Typography,
} from "@wso2/oxygen-ui";
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
    <Card component="section" variant="outlined">
      <CardContent>
        <Typography variant="h6">File storage (Google Drive)</Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
          Documents (quotations, contracts, invoices…) are stored in a Google Drive folder. Connect a
          Google account and choose the folder — changes take effect immediately.
        </Typography>

        {err && (
          <Alert severity="error" sx={{ mt: 1.5 }}>
            {err}
          </Alert>
        )}

        {resp && !prereqsOk && (
          <Alert severity="warning" sx={{ mt: 1.5 }}>
            Storage can't be configured from here until the deployment sets{" "}
            {!resp.oauth_client_configured && <code>storage.gdrive.oauth.client_id/client_secret/api_key</code>}
            {!resp.oauth_client_configured && !resp.secret_key_configured && " and "}
            {!resp.secret_key_configured && <code>security.secret_key</code>} in <code>config.yaml</code>.
          </Alert>
        )}

        {/* Status card */}
        <Box
          component="dl"
          sx={{
            mt: 2,
            display: "grid",
            gridTemplateColumns: "max-content 1fr",
            columnGap: 3,
            rowGap: 0.5,
            m: 0,
          }}
        >
          <Typography component="dt" variant="body2" color="text.secondary">
            Status
          </Typography>
          <Box component="dd" sx={{ m: 0 }}>
            {st?.configured && st?.healthy ? (
              <Typography component="span" variant="body2" sx={{ fontWeight: 600 }} color="success.main">
                ● Connected &amp; healthy
              </Typography>
            ) : st?.last_error ? (
              <Typography component="span" variant="body2" sx={{ fontWeight: 600 }} color="error.main">
                ● Not working
              </Typography>
            ) : (
              <Typography component="span" variant="body2" sx={{ fontWeight: 600 }} color="text.secondary">
                ● Not configured
              </Typography>
            )}
          </Box>
          <Typography component="dt" variant="body2" color="text.secondary">
            Account
          </Typography>
          <Typography component="dd" variant="body2" sx={{ m: 0 }}>
            {p?.account_email || st?.account_email || "—"}
          </Typography>
          <Typography component="dt" variant="body2" color="text.secondary">
            Folder
          </Typography>
          <Typography component="dd" variant="body2" sx={{ m: 0 }}>
            {st?.base_folder_name || st?.base_folder_id || "—"}
          </Typography>
          {st?.last_error && (
            <>
              <Typography component="dt" variant="body2" color="text.secondary">
                Detail
              </Typography>
              <Typography component="dd" variant="body2" color="error.main" sx={{ m: 0 }}>
                {st.last_error}
              </Typography>
            </>
          )}
        </Box>

        <Box sx={{ mt: 2, display: "flex", flexWrap: "wrap", gap: 1.5 }}>
          <Button variant="contained" onClick={connect} disabled={!canConnect}>
            {busy === "connect" ? "Connecting…" : connected ? "Reconnect account" : "Connect Google account"}
          </Button>
          <Button variant="outlined" color="inherit" onClick={chooseFolder} disabled={!canConnect || !connected}>
            {busy === "folder" ? "Opening…" : "Choose folder"}
          </Button>
        </Box>

        <Alert severity="warning" sx={{ mt: 2 }}>
          <strong>Note:</strong> changing the folder or account does not move existing files. Documents
          stored in a previous folder will no longer be downloadable through the app.
        </Alert>
      </CardContent>
    </Card>
  );
}
