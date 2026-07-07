import { createContext, useContext, useEffect, useState, ReactNode } from "react";
import type { User } from "oidc-client-ts";
import { userManager, oidcAuthority, postLogoutRedirectUri } from "./userManager";

interface AuthState {
  user: User | null;
  loading: boolean;
  loginError: string | null;
  login: () => void;
  logout: () => void;
}

const AuthCtx = createContext<AuthState>({
  user: null,
  loading: true,
  loginError: null,
  login: () => {},
  logout: () => {},
});

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [loginError, setLoginError] = useState<string | null>(null);

  useEffect(() => {
    userManager
      .getUser()
      .then((u) => setUser(u && !u.expired ? u : null))
      .finally(() => setLoading(false));

    const onLoaded = (u: User) => setUser(u);
    const onUnloaded = () => setUser(null);
    userManager.events.addUserLoaded(onLoaded);
    userManager.events.addUserUnloaded(onUnloaded);
    userManager.events.addAccessTokenExpired(onUnloaded);
    return () => {
      userManager.events.removeUserLoaded(onLoaded);
      userManager.events.removeUserUnloaded(onUnloaded);
      userManager.events.removeAccessTokenExpired(onUnloaded);
    };
  }, []);

  const login = () => {
    setLoginError(null);
    userManager.signinRedirect().catch((err) => {
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
    // Local logout only — clears stored tokens and returns to /login without
    // round-tripping the IdP's end-session endpoint.
    void userManager.removeUser().then(() => {
      window.location.href = postLogoutRedirectUri;
    });
  };

  return <AuthCtx.Provider value={{ user, loading, loginError, login, logout }}>{children}</AuthCtx.Provider>;
}

export function useAuth() {
  return useContext(AuthCtx);
}
