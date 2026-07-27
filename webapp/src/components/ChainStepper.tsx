import { Fragment } from "react";
import { Link } from "react-router-dom";
import { Box, Chip, Link as MuiLink, Typography } from "@wso2/oxygen-ui";
import { useProcurementAccess } from "../hooks/useProcurementAccess";
import { useRelatedDocuments } from "../hooks/usePurchaseRequests";
import { buildSteps } from "../lib/caseGraph";
import type { CaseCurrent, StepView } from "../lib/caseGraph";

// ChainStepper shows where the current record sits in the procurement chain
// (Request ▸ Quotation ▸ Contract): ancestors link to the specific record on
// this record's path, the current stage is highlighted, and downstream stages
// show how many records hang off it. Procurement-access only (it reads /related).
export function ChainStepper({ prId, current }: { prId: number; current: CaseCurrent }) {
  const procurement = useProcurementAccess();
  const { data } = useRelatedDocuments(prId, procurement);
  if (!procurement || !data) return null;

  const steps = buildSteps(data, current);
  return (
    <Box
      component="nav"
      sx={{
        mb: 2,
        display: "flex",
        flexWrap: "wrap",
        alignItems: "center",
        gap: 0.5,
        px: 1.5,
        py: 1,
        border: 1,
        borderColor: "divider",
        borderRadius: 1,
        bgcolor: "background.paper",
      }}
    >
      {steps.map((step, i) => (
        <Fragment key={step.kind}>
          {i > 0 && (
            <Typography component="span" sx={{ px: 0.25, color: "text.disabled" }}>
              ›
            </Typography>
          )}
          <Step step={step} />
        </Fragment>
      ))}
    </Box>
  );
}

function Step({ step }: { step: StepView }) {
  if (step.state === "current") {
    return (
      <Chip
        size="small"
        color="primary"
        label={`${step.label}${step.ref ? ` · ${step.ref}` : ""}`}
      />
    );
  }
  if (step.state === "ancestor") {
    if (!step.to) {
      return (
        <Typography variant="caption" sx={{ fontWeight: 500, color: "text.disabled" }}>
          {step.label} · —
        </Typography>
      );
    }
    return (
      <MuiLink
        component={Link}
        to={step.to}
        sx={{
          fontSize: 12,
          fontWeight: 500,
          px: 1,
          py: 0.25,
          borderRadius: 1,
          textDecoration: "none",
          "&:hover": { bgcolor: "action.hover" },
        }}
      >
        {step.label} · {step.ref}
      </MuiLink>
    );
  }
  // descendant
  return (
    <Typography variant="caption" sx={{ color: "text.disabled" }}>
      {step.label}
      {step.count ? ` · ${step.count}` : ""}
    </Typography>
  );
}
