import { Navigate } from "react-router-dom";
import { Alert, Box, Button, Card, CardContent, Typography } from "@wso2/oxygen-ui";
import { ShoppingCart } from "@wso2/oxygen-ui-icons-react";
import { useAuth } from "../auth/AuthContext";

export function LoginPage() {
  const { user, loading, loginError, login } = useAuth();
  if (loading)
    return (
      <Box sx={{ p: 4, color: "text.secondary" }}>
        <Typography color="text.secondary">Loading…</Typography>
      </Box>
    );
  if (user) return <Navigate to="/" replace />;

  return (
    <Box
      sx={{
        minHeight: "100vh",
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        px: 2,
      }}
    >
      <Box sx={{ width: "100%", maxWidth: 380 }}>
        <Card variant="outlined">
          <CardContent sx={{ p: 4 }}>
            <Box sx={{ mb: 3, display: "flex", flexDirection: "column", alignItems: "center", textAlign: "center" }}>
              <Box
                sx={{
                  mb: 2,
                  width: 48,
                  height: 48,
                  borderRadius: "14px",
                  bgcolor: "primary.main",
                  color: "primary.contrastText",
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                }}
              >
                <ShoppingCart size={24} />
              </Box>
              <Typography variant="h5" sx={{ fontWeight: 600 }}>
                WSO2 Purchasing App
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                Sign in with your company account.
              </Typography>
            </Box>
            {loginError && (
              <Alert severity="error" sx={{ mb: 2 }}>
                {loginError}
              </Alert>
            )}
            <Button variant="contained" fullWidth size="large" onClick={login}>
              Sign in
            </Button>
          </CardContent>
        </Card>
        <Typography variant="caption" color="text.secondary" sx={{ display: "block", mt: 3, textAlign: "center" }}>
          Internal purchasing management · WSO2
        </Typography>
      </Box>
    </Box>
  );
}
