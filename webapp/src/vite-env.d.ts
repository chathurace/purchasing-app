/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_BASE_URL: string;
  readonly VITE_OIDC_AUTHORITY: string;
  readonly VITE_OIDC_CLIENT_ID: string;
  readonly VITE_OIDC_REDIRECT_URI: string;
  readonly VITE_OIDC_POST_LOGOUT_REDIRECT_URI: string;
  readonly VITE_OIDC_SCOPE: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}

// Runtime config injected by public/config.js (empty in dev, overridden by a
// Choreo file mount in production). Takes priority over the build-time VITE_*
// vars so the same build can be pointed at any environment without a rebuild.
interface AppConfig {
  apiBaseUrl?: string;
  oidcAuthority?: string;
  oidcClientId?: string;
}

interface Window {
  config?: AppConfig;
}
