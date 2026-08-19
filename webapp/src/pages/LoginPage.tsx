import { useEffect, useState } from "react";
import { Navigate } from "react-router-dom";
import {
  alpha,
  Alert,
  Box,
  Button,
  Divider,
  Link as MuiLink,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { ShieldCheck, ShoppingCart } from "@wso2/oxygen-ui-icons-react";
import { useAuth } from "../auth/AuthContext";

// Layout + copy follow resources/ProQ-login.html (two-panel sign-in: brand
// story on the left, the SSO action on the right). Colours are deliberately
// *not* taken from that mockup — every surface, accent and text tone below
// resolves through the app's WSO2 theme palette.

/** The procurement lifecycle rail on the brand panel. */
const FLOW_STEPS = ["Request", "Approve", "Order", "Receive", "Pay"];

/** Headline figures along the bottom of the brand panel. */
const BRAND_FACTS = [
  { value: "E2E", label: "Supply chain cycle" },
  { value: "1", label: "Global procurement flow" },
  { value: "0", label: "POs missed — No PO, No Payment" },
];

const STEP_INTERVAL_MS = 1400;

/**
 * Cycles the highlighted flow step, as the mockup's rail animation does.
 * Returns -1 (all steps lit) when the visitor prefers reduced motion.
 */
function useActiveFlowStep(count: number) {
  const [active, setActive] = useState(0);
  const reduceMotion =
    typeof window !== "undefined" && window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  useEffect(() => {
    if (reduceMotion) return;
    const timer = window.setInterval(() => {
      setActive((i) => (i + 1) % count);
    }, STEP_INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, [count, reduceMotion]);

  return reduceMotion ? -1 : active;
}

export function LoginPage() {
  const { authenticated, loading, loginError, sessionExpired, login } = useAuth();
  const activeStep = useActiveFlowStep(FLOW_STEPS.length);

  if (loading)
    return (
      <Box sx={{ p: 4, color: "text.secondary" }}>
        <Typography color="text.secondary">Loading…</Typography>
      </Box>
    );
  if (authenticated) return <Navigate to="/" replace />;

  return (
    <Box
      sx={{
        minHeight: "100vh",
        display: "grid",
        // `1fr` floors at min-content, so a long pill / headline would widen the
        // column past the viewport — minWidth: 0 on each panel keeps them in.
        gridTemplateColumns: { xs: "minmax(0, 1fr)", md: "minmax(0, 1.1fr) minmax(0, 1fr)" },
      }}
    >
      {/* ── LEFT / BRAND ─────────────────────────────────────────────── */}
      <Box
        component="aside"
        sx={{
          position: "relative",
          overflow: "hidden",
          minWidth: 0,
          display: "flex",
          flexDirection: "column",
          justifyContent: "space-between",
          gap: { xs: 4.5, md: 0 },
          px: { xs: 3.5, md: 8 },
          py: { xs: 5, md: 7 },
          minHeight: { xs: "46vh", md: "auto" },
          // The theme's ink tone, used as a surface for the brand panel.
          bgcolor: "text.primary",
          color: (theme) => theme.palette.getContrastText(theme.palette.text.primary),
          // Faint grid texture.
          backgroundImage: `linear-gradient(${alpha("#fff", 0.045)} 1px, transparent 1px),
              linear-gradient(90deg, ${alpha("#fff", 0.045)} 1px, transparent 1px)`,
          backgroundSize: "56px 56px",
        }}
      >
        {/* Accent glow, in the theme's primary colour. */}
        <Box
          aria-hidden
          sx={{
            position: "absolute",
            width: 640,
            height: 640,
            borderRadius: "50%",
            bottom: -280,
            left: -180,
            pointerEvents: "none",
            background: (theme) =>
              `radial-gradient(circle, ${alpha(theme.palette.primary.main, 0.35)} 0%, transparent 65%)`,
          }}
        />

        <Stack direction="row" alignItems="center" spacing={1.25} sx={{ position: "relative" }}>
          <Box
            sx={{
              width: 44,
              height: 44,
              borderRadius: "12px",
              flex: "none",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              bgcolor: "primary.main",
              color: "primary.contrastText",
            }}
          >
            <ShoppingCart size={22} />
          </Box>
          <Typography sx={{ fontSize: 22, fontWeight: 700, letterSpacing: "-0.3px" }}>
            WSO2 Purchasing App
          </Typography>
        </Stack>

        <Box sx={{ position: "relative", maxWidth: 520 }}>
          <Typography
            sx={{
              fontSize: 12,
              letterSpacing: "2.5px",
              textTransform: "uppercase",
              fontWeight: 500,
              color: "primary.main",
              mb: 2.5,
            }}
          >
            WSO2 · End-to-End Supply Chain Management
          </Typography>

          <Typography
            component="h1"
            sx={{
              fontSize: "clamp(34px, 3.6vw, 52px)",
              fontWeight: 700,
              lineHeight: 1.08,
              letterSpacing: "-1.2px",
              mb: 2.75,
            }}
          >
            Every purchase.
            <br />
            One <Box component="span" sx={{ color: "primary.main" }}>governed</Box> flow.
          </Typography>

          <Typography
            sx={{
              fontSize: 16.5,
              lineHeight: 1.65,
              color: alpha("#fff", 0.72),
              mb: 5.5,
            }}
          >
            The WSO2 Purchasing App is WSO2's internal platform for managing the end-to-end supply
            chain cycle — raise requests, route approvals, and track every order from requisition to
            payment, across all WSO2 entities worldwide.
          </Typography>

          {/* Lifecycle rail: pills joined by hairlines, one lit at a time. */}
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              flexWrap: "wrap",
              rowGap: 1.75,
              // Narrow panels wrap the rail, where the joining hairlines would
              // dangle at a row start — a plain gap reads better there.
              columnGap: { xs: 1, md: 0 },
            }}
          >
            {FLOW_STEPS.map((step, i) => {
              const isActive = activeStep === -1 || activeStep === i;
              return (
                <Box key={step} sx={{ display: "flex", alignItems: "center" }}>
                  {i > 0 && (
                    <Box
                      aria-hidden
                      sx={{
                        width: 20,
                        height: "1px",
                        flex: "none",
                        bgcolor: alpha("#fff", 0.22),
                        display: { xs: "none", md: "block" },
                      }}
                    />
                  )}
                  <Box
                    sx={{
                      display: "flex",
                      alignItems: "center",
                      gap: 1,
                      px: 1.5,
                      py: 1,
                      borderRadius: 999,
                      whiteSpace: "nowrap",
                      fontSize: 12.5,
                      letterSpacing: "0.5px",
                      color: alpha("#fff", 0.85),
                      border: "1px solid",
                      borderColor: (theme) =>
                        isActive ? alpha(theme.palette.primary.main, 0.9) : alpha("#fff", 0.14),
                      bgcolor: (theme) =>
                        isActive ? alpha(theme.palette.primary.main, 0.14) : alpha("#fff", 0.03),
                      transition: "border-color .3s, background-color .3s",
                      "@media (prefers-reduced-motion: reduce)": { transition: "none" },
                    }}
                  >
                    <Box
                      aria-hidden
                      sx={{
                        width: 7,
                        height: 7,
                        borderRadius: "50%",
                        flex: "none",
                        bgcolor: (theme) =>
                          isActive ? theme.palette.primary.main : alpha("#fff", 0.35),
                        boxShadow: (theme) =>
                          isActive ? `0 0 10px ${alpha(theme.palette.primary.main, 0.9)}` : "none",
                        transition: "background-color .3s, box-shadow .3s",
                        "@media (prefers-reduced-motion: reduce)": { transition: "none" },
                      }}
                    />
                    {step}
                  </Box>
                </Box>
              );
            })}
          </Box>
        </Box>

        <Stack
          direction="row"
          sx={{ position: "relative", gap: 5.5, display: { xs: "none", md: "flex" } }}
        >
          {BRAND_FACTS.map((fact) => (
            <Box key={fact.value}>
              <Typography sx={{ fontSize: 22, fontWeight: 600, letterSpacing: "-0.3px" }}>
                {fact.value}
              </Typography>
              <Typography sx={{ fontSize: 12.5, color: alpha("#fff", 0.55) }}>
                {fact.label}
              </Typography>
            </Box>
          ))}
        </Stack>
      </Box>

      {/* ── RIGHT / SIGN IN ──────────────────────────────────────────── */}
      <Box
        component="main"
        sx={{
          position: "relative",
          minWidth: 0,
          display: "flex",
          flexDirection: "column",
          alignItems: "center",
          justifyContent: "center",
          px: { xs: 3, md: 6 },
          pt: { xs: 6, md: 7 },
          pb: { xs: 10.5, md: 7 },
          bgcolor: "background.default",
          backgroundImage: (theme) =>
            `radial-gradient(circle at 85% 12%, ${alpha(theme.palette.primary.main, 0.08)} 0%, transparent 45%)`,
        }}
      >
        <Box sx={{ width: "100%", maxWidth: 400 }}>
          <Typography
            component="h2"
            sx={{ fontSize: 27, fontWeight: 600, letterSpacing: "-0.5px", mb: 1.25 }}
          >
            Welcome back
          </Typography>
          <Typography sx={{ fontSize: 14.5, lineHeight: 1.6, color: "text.secondary", mb: 4.25 }}>
            Sign in with your WSO2 account to raise a purchase request or track an existing one.
          </Typography>

          {/* Only shown when a working session was rejected mid-use (a 60-day
              idle expiry, a revoked session, or "sign out everywhere") — not to
              a visitor who simply has not signed in yet. */}
          {sessionExpired && !loginError && (
            <Alert severity="info" sx={{ mb: 2 }}>
              Your session has expired. Please sign in again to continue.
            </Alert>
          )}

          {loginError && (
            <Alert severity="error" sx={{ mb: 2 }}>
              {loginError}
            </Alert>
          )}

          <Button
            variant="contained"
            fullWidth
            size="large"
            onClick={login}
            startIcon={<ShieldCheck size={18} />}
            sx={{ py: 1.5, fontSize: 15, fontWeight: 600 }}
          >
            Continue with WSO2 SSO
          </Button>

          <Typography
            sx={{ mt: 2.25, fontSize: 13, lineHeight: 1.6, color: "text.secondary", textAlign: "center" }}
          >
            Access is limited to WSO2 team members.
            <br />
            Trouble signing in?{" "}
            <MuiLink href="mailto:procurement@wso2.com" underline="hover" sx={{ fontWeight: 500 }}>
              Contact the Procurement team
            </MuiLink>
          </Typography>

          <Divider sx={{ mt: 4.75, mb: 2.75 }} />

          <Stack direction="row" justifyContent="space-between" sx={{ fontSize: 12.5 }}>
            <MuiLink href="#" underline="hover" color="text.secondary" sx={{ fontSize: 12.5 }}>
              Procurement policy (SOP-85000)
            </MuiLink>
            <MuiLink href="#" underline="hover" color="text.secondary" sx={{ fontSize: 12.5 }}>
              Help &amp; FAQs
            </MuiLink>
          </Stack>
        </Box>

        <Typography
          sx={{
            position: "absolute",
            bottom: 26,
            fontSize: 12,
            letterSpacing: "0.2px",
            color: "text.secondary",
            opacity: 0.7,
          }}
        >
          Internal purchasing management ·{" "}
          <Box component="b" sx={{ fontWeight: 600, color: "text.primary" }}>
            WSO2
          </Box>
        </Typography>
      </Box>
    </Box>
  );
}
