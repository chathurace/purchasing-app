import { UserManager, WebStorageStateStore } from "oidc-client-ts";

export const userManager = new UserManager({
  authority: import.meta.env.VITE_OIDC_AUTHORITY,
  client_id: import.meta.env.VITE_OIDC_CLIENT_ID,
  redirect_uri: import.meta.env.VITE_OIDC_REDIRECT_URI,
  post_logout_redirect_uri: import.meta.env.VITE_OIDC_POST_LOGOUT_REDIRECT_URI,
  scope: import.meta.env.VITE_OIDC_SCOPE || "openid profile email",
  response_type: "code",
  automaticSilentRenew: true,
  // WSO2 IS does not always expose a working end_session endpoint in dev; we
  // log out locally only (see AuthContext.logout).
  userStore: new WebStorageStateStore({ store: window.localStorage }),
});

// We send the ID token (not the access token) as the API bearer: it is
// audience-restricted to this client and carries the OIDC profile claims
// (email, name) that the backend verifies. WSO2 access tokens often omit them.
export async function getAccessToken(): Promise<string | null> {
  const user = await userManager.getUser();
  return user?.id_token ?? user?.access_token ?? null;
}
