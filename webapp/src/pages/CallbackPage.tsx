import { useEffect, useRef, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { Box, Link as MuiLink, Typography } from "@wso2/oxygen-ui";
import { userManager } from "../auth/userManager";

export function CallbackPage() {
  const navigate = useNavigate();
  const [error, setError] = useState<string | null>(null);
  const handled = useRef(false);

  useEffect(() => {
    if (handled.current) return; // guard against StrictMode double-mount
    handled.current = true;
    userManager
      .signinRedirectCallback()
      .then(() => navigate("/", { replace: true }))
      .catch((e) => setError(e instanceof Error ? e.message : "Sign-in failed"));
  }, [navigate]);

  if (error) {
    return (
      <Box sx={{ p: 4 }}>
        <Typography color="error">Sign-in failed: {error}</Typography>
        <MuiLink component={Link} to="/login">
          Back to login
        </MuiLink>
      </Box>
    );
  }
  return (
    <Box sx={{ p: 4 }}>
      <Typography color="text.secondary">Signing you in…</Typography>
    </Box>
  );
}
