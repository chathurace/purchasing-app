import { ReactNode } from "react";
import { Navigate } from "react-router-dom";
import { Box, Typography } from "@wso2/oxygen-ui";
import { useAuth } from "./AuthContext";

export function RequireAuth({ children }: { children: ReactNode }) {
  // `authenticated`, not the OIDC user: in the steady state the backend session
  // cookie is the credential and this tab holds no live IdP token at all.
  const { authenticated, loading } = useAuth();
  if (loading) {
    return (
      <Box sx={{ p: 4 }}>
        <Typography color="text.secondary">Loading…</Typography>
      </Box>
    );
  }
  if (!authenticated) {
    return <Navigate to="/login" replace />;
  }
  return <>{children}</>;
}
