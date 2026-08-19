import { UserManager, WebStorageStateStore } from "oidc-client-ts";

// Config precedence: runtime window.config (Choreo file mount) → build-time
// VITE_* env (.env for local dev) → sensible defaults. This lets a single
// production build be pointed at any environment without a rebuild.
export const oidcAuthority =
  window.config?.oidcAuthority ?? import.meta.env.VITE_OIDC_AUTHORITY;
const oidcClientId =
  window.config?.oidcClientId ?? import.meta.env.VITE_OIDC_CLIENT_ID;

// Redirect URIs default to the current origin so they follow the deployed host
// automatically. VITE_OIDC_* still overrides for local dev (e.g. a non-default
// port); in Choreo those vars are absent so the origin-derived values are used.
const redirectUri =
  import.meta.env.VITE_OIDC_REDIRECT_URI ?? `${window.location.origin}/callback`;
export const postLogoutRedirectUri =
  import.meta.env.VITE_OIDC_POST_LOGOUT_REDIRECT_URI ?? `${window.location.origin}/login`;

export const userManager = new UserManager({
  authority: oidcAuthority,
  client_id: oidcClientId,
  redirect_uri: redirectUri,
  post_logout_redirect_uri: postLogoutRedirectUri,
  scope: import.meta.env.VITE_OIDC_SCOPE || "openid profile email",
  response_type: "code",
  // Silent renew is deliberately OFF: the app's session is the backend cookie
  // (docs/sessions.md), so the ID token is needed exactly once — to mint that
  // cookie — and letting it expire is no longer a logout. Renewing it in a
  // hidden iframe would also require registering a silent-callback redirect URI
  // with Asgardeo for no gain.
  automaticSilentRenew: false,
  // WSO2 IS does not always expose a working end_session endpoint in dev; we
  // log out locally only (see AuthContext.logout).
  userStore: new WebStorageStateStore({ store: window.localStorage }),
});

// We send the ID token (not the access token) as the API bearer: it is
// audience-restricted to this client and carries the OIDC profile claims
// (email, name) that the backend verifies. WSO2 access tokens often omit them.
//
// An expired user is treated as no token at all: it would be rejected anyway,
// and the backend checks the session cookie before the Authorization header, so
// sending a dead token would only add noise to the logs.
export async function getAccessToken(): Promise<string | null> {
  const user = await userManager.getUser();
  if (!user || user.expired) return null;
  return user.id_token ?? user.access_token ?? null;
}
