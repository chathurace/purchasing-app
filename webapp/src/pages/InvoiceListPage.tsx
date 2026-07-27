import { Link, useSearchParams } from "react-router-dom";
import {
  Alert,
  Box,
  CircularProgress,
  Link as MuiLink,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from "@wso2/oxygen-ui";
import { useInvoices } from "../hooks/useInvoices";
import { EntityStatusBadge } from "../components/EntityStatusBadge";
import { VendorFilterNotice } from "../components/VendorFilterNotice";
import { conRef, formatMoney, invRef } from "../types/api";

export function InvoiceListPage() {
  const { data, isLoading, error } = useInvoices();
  const [params] = useSearchParams();
  const vendorFilter = Number(params.get("vendor")) || 0;
  const rows = vendorFilter ? (data ?? []).filter((inv) => inv.vendor_id === vendorFilter) : data ?? [];
  const vendorName = rows.find((inv) => inv.vendor)?.vendor?.name;

  return (
    <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
      <Typography variant="h5" sx={{ fontWeight: 600, mb: 3 }}>
        Invoices
      </Typography>

      {vendorFilter > 0 && (
        <VendorFilterNotice vendorId={vendorFilter} vendorName={vendorName} basePath="/invoices" />
      )}

      {isLoading && <CircularProgress size={24} />}
      {error && <Alert severity="error">Failed to load invoices.</Alert>}

      {data && rows.length === 0 && (
        <Paper variant="outlined" sx={{ p: 6, textAlign: "center", borderStyle: "dashed" }}>
          <Typography variant="body2" color="text.secondary">
            {vendorFilter > 0 ? "No invoices for this vendor." : "No invoices yet. Record one from a signed contract."}
          </Typography>
        </Paper>
      )}

      {data && rows.length > 0 && (
        <TableContainer component={Paper} variant="outlined">
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>Ref</TableCell>
                <TableCell>Vendor invoice no.</TableCell>
                <TableCell>Vendor</TableCell>
                <TableCell>Amount</TableCell>
                <TableCell>Invoice date</TableCell>
                <TableCell>Contract</TableCell>
                <TableCell>Status</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {rows.map((inv) => (
                <TableRow key={inv.id} hover>
                  <TableCell>
                    <MuiLink component={Link} to={`/invoices/${inv.id}`} sx={{ fontWeight: 500 }}>
                      {invRef(inv.id)}
                    </MuiLink>
                  </TableCell>
                  <TableCell>
                    {inv.vendor_invoice_no || (
                      <Typography component="span" color="text.disabled">
                        —
                      </Typography>
                    )}
                  </TableCell>
                  <TableCell>{inv.vendor?.name ?? `Vendor #${inv.vendor_id}`}</TableCell>
                  <TableCell>{formatMoney(inv.total_amount, inv.currency)}</TableCell>
                  <TableCell>{inv.invoice_date}</TableCell>
                  <TableCell>
                    <MuiLink component={Link} to={`/contracts/${inv.contract_id}`}>
                      {conRef(inv.contract_id)}
                    </MuiLink>
                  </TableCell>
                  <TableCell>
                    <EntityStatusBadge status={inv.status} />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}
    </Box>
  );
}
