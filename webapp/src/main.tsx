import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { OxygenUIThemeProvider, WSO2Theme } from "@wso2/oxygen-ui";
import { AuthProvider } from "./auth/AuthContext";
import App from "./App";
import "./App.css";

// ── Light-only lock ─────────────────────────────────────────────────────────
// WSO2Theme ships both light and dark colorSchemes and MUI's CssVars provider
// defaults `mode` to `system`, so on a dark-mode OS the app would render dark.
// The redesign is light-only for now (no color-scheme toggle in the UI), so we
// pin the mode to light before React mounts: seed MUI's mode storage key
// (`mui-mode`, the CssVars default) and set the `data-color-scheme` selector
// attribute WSO2Theme reads. Without this, MUI's default `mode: "system"` would
// render dark on a dark-mode OS. Remove this (and add a toggle) for dark mode.
try {
  window.localStorage.setItem("mui-mode", "light");
} catch {
  /* private mode / storage disabled — the attribute below still applies */
}
document.documentElement.setAttribute("data-color-scheme", "light");

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: 1, refetchOnWindowFocus: false } },
});

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <OxygenUIThemeProvider theme={WSO2Theme}>
      <QueryClientProvider client={queryClient}>
        <AuthProvider>
          <BrowserRouter>
            <App />
          </BrowserRouter>
        </AuthProvider>
      </QueryClientProvider>
    </OxygenUIThemeProvider>
  </React.StrictMode>,
);
