import { ReactNode } from "react";
import { Navigate } from "react-router-dom";
import { Box, Typography } from "@wso2/oxygen-ui";
import { useAuth } from "./AuthContext";

export function RequireAuth({ children }: { children: ReactNode }) {
  const { user, loading } = useAuth();
  if (loading) {
    return (
      <Box sx={{ p: 4 }}>
        <Typography color="text.secondary">Loading…</Typography>
      </Box>
    );
  }
  if (!user) {
    return <Navigate to="/login" replace />;
  }
  return <>{children}</>;
}
