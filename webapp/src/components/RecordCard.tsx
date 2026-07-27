import { Link } from "react-router-dom";
import { alpha, Box, Button, Card, CardContent, Link as MuiLink, Stack, Typography } from "@wso2/oxygen-ui";
import { Plus } from "@wso2/oxygen-ui-icons-react";
import { StatusBadge } from "./StatusBadge";
import { EntityStatusBadge } from "./EntityStatusBadge";
import type { CaseRecord } from "../lib/caseGraph";
import type { ContractStatus, InvoiceStatus, PRStatus, QuotationStatus } from "../types/api";

// RecordRow renders one clickable reference to a case record, with its status
// badge. Records on the winning path (selected quotation / signed contract) are
// tinted so the active branch stands out among siblings.
export function RecordRow({ record }: { record: CaseRecord }) {
  return (
    <Box
      sx={(theme) => ({
        display: "flex",
        alignItems: "center",
        justifyContent: "space-between",
        gap: 1.5,
        ...(record.highlight && {
          mx: -1,
          px: 1,
          py: 0.5,
          borderRadius: 1,
          bgcolor: alpha(theme.palette.success.main, 0.08),
          boxShadow: `0 0 0 1px ${alpha(theme.palette.success.main, 0.25)}`,
        }),
      })}
    >
      <Box sx={{ minWidth: 0 }}>
        <MuiLink component={Link} to={record.to} sx={{ fontWeight: 600 }}>
          {record.ref}
        </MuiLink>
        {record.detail ? (
          <Typography component="span" variant="body2" color="text.secondary" sx={{ ml: 1 }}>
            {record.detail}
          </Typography>
        ) : null}
      </Box>
      {record.status == null ? null : record.kind === "pr" ? (
        <StatusBadge status={record.status as PRStatus} />
      ) : (
        <EntityStatusBadge status={record.status as QuotationStatus | ContractStatus | InvoiceStatus} />
      )}
    </Box>
  );
}

// RecordCard is the standard "rounded white card with a heading" used for the
// related-record sections. Pass `records` for the default list rendering, or
// `children` for custom content; `action` adds a right-aligned link (e.g. an
// "+ Add" launcher).
export function RecordCard({
  title,
  action,
  actionProminent = false,
  records,
  empty = "None.",
  emptyCta,
  children,
}: {
  title: string;
  action?: { to: string; label: string };
  // When true, render the action as a solid button and (if there are no
  // records) surface a clickable dashed-border CTA in place of the empty text,
  // for steps that are the natural next action in the flow.
  actionProminent?: boolean;
  records?: CaseRecord[];
  empty?: string;
  emptyCta?: { title: string; subtitle: string };
  children?: React.ReactNode;
}) {
  const noRecords = !records || records.length === 0;
  return (
    <Card variant="outlined" sx={{ mt: 3 }}>
      <CardContent>
        <Box sx={{ mb: 1.5, display: "flex", alignItems: "center", justifyContent: "space-between" }}>
          <Typography variant="h6" sx={{ fontWeight: 600 }}>
            {title}
          </Typography>
          {action ? (
            actionProminent ? (
              <Button
                component={Link}
                to={action.to}
                variant="contained"
                size="small"
                startIcon={<Plus size={16} />}
              >
                {action.label.replace(/^\+\s*/, "")}
              </Button>
            ) : (
              <MuiLink component={Link} to={action.to} variant="body2">
                {action.label}
              </MuiLink>
            )
          ) : null}
        </Box>
        {children ??
          (!noRecords ? (
            <Stack spacing={0.75}>
              {records!.map((r) => (
                <RecordRow key={`${r.kind}-${r.id}`} record={r} />
              ))}
            </Stack>
          ) : actionProminent && action && emptyCta ? (
            <Box
              component={Link}
              to={action.to}
              sx={{
                display: "block",
                textAlign: "center",
                textDecoration: "none",
                borderRadius: 2,
                border: "2px dashed",
                borderColor: "primary.light",
                bgcolor: (theme) => alpha(theme.palette.primary.main, 0.04),
                p: 2.5,
                transition: "border-color .15s, background-color .15s",
                "&:hover": {
                  borderColor: "primary.main",
                  bgcolor: (theme) => alpha(theme.palette.primary.main, 0.08),
                },
              }}
            >
              <Typography variant="body2" sx={{ fontWeight: 600 }} color="primary.main">
                {emptyCta.title}
              </Typography>
              <Typography variant="caption" color="text.secondary">
                {emptyCta.subtitle}
              </Typography>
            </Box>
          ) : (
            <Typography variant="body2" color="text.secondary">
              {empty}
            </Typography>
          ))}
      </CardContent>
    </Card>
  );
}
