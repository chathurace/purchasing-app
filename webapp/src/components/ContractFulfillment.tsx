import { Link } from "react-router-dom";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Link as MuiLink,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { Plus } from "@wso2/oxygen-ui-icons-react";
import { useGRNsForContract } from "../hooks/useGrns";
import { useInvoicesForContract } from "../hooks/useInvoices";
import { EntityStatusBadge } from "./EntityStatusBadge";
import { formatMoney, grnRef, invRef } from "../types/api";
import type { Contract } from "../types/api";

// EmptyCta is the dashed clickable prompt shown when a section has no records yet.
function EmptyCta({ to, title, subtitle }: { to: string; title: string; subtitle: string }) {
  return (
    <Box
      component={Link}
      to={to}
      sx={{
        display: "block",
        p: 2.5,
        textAlign: "center",
        borderRadius: 2,
        border: "2px dashed",
        borderColor: "primary.light",
        bgcolor: "action.hover",
        textDecoration: "none",
        transition: "background-color 0.2s, border-color 0.2s",
        "&:hover": { borderColor: "primary.main", bgcolor: "action.selected" },
      }}
    >
      <Typography variant="subtitle2" sx={{ color: "primary.main" }}>
        {title}
      </Typography>
      <Typography variant="caption" sx={{ color: "text.secondary" }}>
        {subtitle}
      </Typography>
    </Box>
  );
}

// ContractFulfillment shows the goods-received notes and invoices recorded
// against a signed contract, plus the invoiced-vs-contract total summary with an
// over-billing warning. Only meaningful once the contract is signed.
export function ContractFulfillment({ contract }: { contract: Contract }) {
  const signed = contract.status === "signed";
  const { data: grns } = useGRNsForContract(contract.id, signed);
  const { data: invoices } = useInvoicesForContract(contract.id, signed);

  if (!signed) {
    return (
      <Card variant="outlined" sx={{ mt: 3 }}>
        <CardContent>
          <Typography variant="h6" sx={{ mb: 0.5 }}>
            Goods received &amp; invoices
          </Typography>
          <Typography variant="body2" sx={{ color: "text.disabled" }}>
            Available once the contract is signed. Sign the order above to start recording GRNs and
            invoices.
          </Typography>
        </CardContent>
      </Card>
    );
  }

  const invoicedTotal = contract.invoiced_total ?? 0;
  const remaining = contract.total_amount - invoicedTotal;
  const overBilled = invoicedTotal > contract.total_amount;

  return (
    <>
      {/* Goods received notes */}
      <Card variant="outlined" sx={{ mt: 3 }}>
        <CardContent>
          <Box sx={{ mb: 1.5, display: "flex", alignItems: "center", justifyContent: "space-between" }}>
            <Typography variant="h6">Goods received (GRNs)</Typography>
            <Button
              component={Link}
              to={`/contracts/${contract.id}/grns/new`}
              variant="contained"
              startIcon={<Plus size={16} />}
            >
              New GRN
            </Button>
          </Box>
          {!grns ? (
            <Typography variant="body2" sx={{ color: "text.disabled" }}>
              Loading…
            </Typography>
          ) : grns.length === 0 ? (
            <EmptyCta
              to={`/contracts/${contract.id}/grns/new`}
              title="Record the first GRN"
              subtitle="Log goods received against this signed contract."
            />
          ) : (
            <Stack divider={<Box sx={{ borderTop: 1, borderColor: "divider" }} />}>
              {grns.map((g) => (
                <Box
                  key={g.id}
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    py: 1,
                  }}
                >
                  <MuiLink component={Link} to={`/grns/${g.id}`} sx={{ textDecoration: "none" }}>
                    {grnRef(g.id)}
                  </MuiLink>
                  <Typography variant="body2" sx={{ color: "text.secondary" }}>
                    received {g.received_date}
                    {g.received_by ? ` · ${g.received_by}` : ""}
                  </Typography>
                </Box>
              ))}
            </Stack>
          )}
        </CardContent>
      </Card>

      {/* Invoices */}
      <Card variant="outlined" sx={{ mt: 3 }}>
        <CardContent>
          <Box sx={{ mb: 1.5, display: "flex", alignItems: "center", justifyContent: "space-between" }}>
            <Typography variant="h6">Invoices</Typography>
            <Button
              component={Link}
              to={`/contracts/${contract.id}/invoices/new`}
              variant="contained"
              startIcon={<Plus size={16} />}
            >
              New invoice
            </Button>
          </Box>

          {/* Invoiced-vs-contract summary (over-billing warning) */}
          <Alert severity={overBilled ? "error" : "info"} icon={false} sx={{ mb: 1.5 }}>
            Invoiced {formatMoney(invoicedTotal, contract.currency)} of{" "}
            {formatMoney(contract.total_amount, contract.currency)}
            {overBilled ? (
              <Box component="span" sx={{ fontWeight: 600 }}>
                {" "}
                — over contract by{" "}
                {formatMoney(invoicedTotal - contract.total_amount, contract.currency)}
              </Box>
            ) : (
              <span> · {formatMoney(remaining, contract.currency)} remaining</span>
            )}
          </Alert>

          {!invoices ? (
            <Typography variant="body2" sx={{ color: "text.disabled" }}>
              Loading…
            </Typography>
          ) : invoices.length === 0 ? (
            <EmptyCta
              to={`/contracts/${contract.id}/invoices/new`}
              title="Record the first invoice"
              subtitle="Enter a vendor invoice billed against this contract."
            />
          ) : (
            <Stack divider={<Box sx={{ borderTop: 1, borderColor: "divider" }} />}>
              {invoices.map((inv) => (
                <Box
                  key={inv.id}
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    py: 1,
                  }}
                >
                  <MuiLink component={Link} to={`/invoices/${inv.id}`} sx={{ textDecoration: "none" }}>
                    {inv.vendor_invoice_no
                      ? `${invRef(inv.id)} · ${inv.vendor_invoice_no}`
                      : invRef(inv.id)}
                  </MuiLink>
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1, color: "text.secondary" }}>
                    <Typography variant="body2" sx={{ color: "text.secondary" }}>
                      {formatMoney(inv.total_amount, inv.currency)}
                    </Typography>
                    <EntityStatusBadge status={inv.status} />
                  </Box>
                </Box>
              ))}
            </Stack>
          )}
        </CardContent>
      </Card>
    </>
  );
}
