import { createContext, useContext, useEffect, useRef, useState, ReactNode } from "react";
import type { User } from "oidc-client-ts";
import { useQueryClient } from "@tanstack/react-query";
import { userManager, oidcAuthority, postLogoutRedirectUri } from "./userManager";
import { getMe } from "../api/me";
import { startSession, endSession } from "../api/session";
import { sessionCookiesUsable } from "../api/client";

interface AuthState {
  /**
   * The OIDC user, when this tab still holds IdP tokens. Null in the steady
   * state — after the ID token expires (or the browser restarts) the app is
   * authenticated by the backend session cookie alone, so use `authenticated`
   * to decide whether someone is signed in. This is only for the OIDC profile.
   */
  user: User | null;
  /** True once either the session cookie or an IdP token has been accepted. */
  authenticated: boolean;
  loading: boolean;
  loginError: string | null;
  /** True when a previously working session was rejected (a 401 mid-session). */
  sessionExpired: boolean;
  login: () => void;
  logout: () => void;
}

const AuthCtx = createContext<AuthState>({
  user: null,
  authenticated: false,
  loading: true,
  loginError: null,
  sessionExpired: false,
  login: () => {},
  logout: () => {},
});

/**
 * Authentication state.
 *
 * The app runs on a backend-issued session (BFF pattern — see
 * backend/internal/handler/auth_session.go and docs/sessions.md). The IdP is
 * used once, to prove who the user is; the backend then issues an HttpOnly
 * cookie that lasts session.ttl_days (60 by default). That is what stops the
 * roughly-daily sign-outs: Asgardeo's token and session lifetimes are managed
 * by another team and cannot be raised, and the app used to be logged out as
 * soon as the ID token expired.
 *
 * Boot order, cheapest and most common first:
 *   1. Cookie — a plain getMe(). Succeeds on essentially every load after the
 *      first sign-in, including after a browser restart, with no IdP contact.
 *   2. IdP tokens still held by this tab (the /callback redirect just ran, or
 *      the token has not expired yet), traded for a cookie by startSession().
 *   3. Neither — render the login page.
 */
export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [authenticated, setAuthenticated] = useState(false);
  const [loading, setLoading] = useState(true);
  const [loginError, setLoginError] = useState<string | null>(null);
  const [sessionExpired, setSessionExpired] = useState(false);
  const queryClient = useQueryClient();
  // De-duplicates concurrent handshakes: boot and the userLoaded event can both
  // fire one, and each mint would orphan the other's session row.
  const handshake = useRef<Promise<boolean> | null>(null);
  // False until the boot probe has finished, so its expected 401 for a visitor
  // who is simply not signed in yet is not reported as an expiry.
  const booted = useRef(false);

  useEffect(() => {
    /**
     * Trades the IdP token this tab holds for a backend session cookie. Falls
     * back to a plain getMe() when cookies are unusable (a cross-site API — see
     * apiCredentials) or the backend has sessions disabled, which keeps this tab
     * working on Bearer tokens alone.
     */
    const establishSession = (): Promise<boolean> => {
      if (!sessionCookiesUsable()) {
        return getMe()
          .then(() => true)
          .catch(() => false);
      }
      if (!handshake.current) {
        handshake.current = startSession()
          .then(() => true)
          .catch(() => getMe().then(() => true).catch(() => false))
          .finally(() => {
            handshake.current = null;
          });
      }
      return handshake.current;
    };

    const boot = async () => {
      // /callback is mid-handshake: CallbackPage completes the code exchange,
      // which raises userLoaded and runs the handshake from there. Probing for a
      // session here would race it.
      if (window.location.pathname === "/callback") {
        booted.current = true;
        setLoading(false);
        return;
      }

      // 1. An existing backend session cookie — no IdP involvement at all.
      try {
        await getMe();
        setAuthenticated(true);
        booted.current = true;
        setLoading(false);
        return;
      } catch {
        // 401 (no/expired cookie) or the backend is unreachable; fall through.
      }

      // 2. IdP tokens still in this tab.
      const oidcUser = await userManager.getUser();
      if (oidcUser && !oidcUser.expired) {
        setUser(oidcUser);
        setAuthenticated(await establishSession());
      }
      // 3. Otherwise RequireAuth sends the visitor to /login.
      booted.current = true;
      setLoading(false);
    };

    void boot();

    // Fires after the /callback redirect completes the code exchange.
    const onLoaded = (u: User) => {
      setUser(u);
      setSessionExpired(false);
      // A different person may be signing in — never show them cached data.
      queryClient.clear();
      void establishSession().then(setAuthenticated);
    };
    // Clears only the OIDC half; the backend session is independent and is
    // ended explicitly by logout().
    const onUnloaded = () => setUser(null);
    userManager.events.addUserLoaded(onLoaded);
    userManager.events.addUserUnloaded(onUnloaded);
    // The IdP token expiring is deliberately NOT treated as a logout any more:
    // that was the direct cause of the daily sign-outs. Expiry is detected only
    // from the API's 401s, dispatched by apiFetch.
    const onSessionExpired = () => {
      if (!booted.current) return; // the boot probe's own 401 — a normal cold start
      setSessionExpired(true);
      setAuthenticated(false);
      setUser(null);
      void queryClient.cancelQueries();
    };
    window.addEventListener("session-expired", onSessionExpired);

    return () => {
      userManager.events.removeUserLoaded(onLoaded);
      userManager.events.removeUserUnloaded(onUnloaded);
      window.removeEventListener("session-expired", onSessionExpired);
    };
  }, []);

  const login = () => {
    setLoginError(null);
    // Force the IdP to re-prompt for credentials rather than silently
    // reusing an existing IdP session. Because logout is local-only (we do
    // not round-trip the IdP's end-session endpoint — see below), without
    // this a previously signed-in user would be re-authenticated
    // automatically instead of being able to sign in as someone else.
    userManager.signinRedirect({ prompt: "login" }).catch((err) => {
      // signinRedirect first fetches the IdP's OIDC metadata. In dev this is a
      // self-signed https endpoint (WSO2 on :9443); if the browser hasn't
      // trusted that cert the fetch fails and the click would otherwise appear
      // to do nothing. Surface it instead of swallowing the rejection.
      const detail = err instanceof Error ? err.message : String(err);
      setLoginError(
        `Could not start sign-in: ${detail}. If the identity provider uses a self-signed ` +
          `certificate, open ${oidcAuthority} once and accept the warning, then retry.`,
      );
    });
  };

  const logout = () => {
    // End the backend session first — revoking it server-side, so a copied
    // cookie stops working immediately instead of living out its 60 days. Then
    // clear the local tokens; we do not round-trip the IdP's end-session
    // endpoint (it is not reliably available in dev).
    void endSession()
      .catch(() => {
        /* best effort — never leave the user stuck on a failed sign-out */
      })
      .then(() => {
        setAuthenticated(false);
        queryClient.clear();
        return userManager.removeUser();
      })
      .then(() => {
        window.location.href = postLogoutRedirectUri;
      });
  };

  return (
    <AuthCtx.Provider value={{ user, authenticated, loading, loginError, sessionExpired, login, logout }}>
      {children}
    </AuthCtx.Provider>
  );
}

export function useAuth() {
  return useContext(AuthCtx);
}
