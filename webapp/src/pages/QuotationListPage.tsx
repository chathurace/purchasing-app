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
import { useQuotations } from "../hooks/useQuotations";
import { EntityStatusBadge } from "../components/EntityStatusBadge";
import { VendorFilterNotice } from "../components/VendorFilterNotice";
import { formatMoney, quoRef } from "../types/api";

export function QuotationListPage() {
  const { data, isLoading, error } = useQuotations();
  const [params] = useSearchParams();
  const vendorFilter = Number(params.get("vendor")) || 0;
  const rows = vendorFilter ? (data ?? []).filter((q) => q.vendor_id === vendorFilter) : data ?? [];
  const vendorName = rows.find((q) => q.vendor)?.vendor?.name;

  return (
    <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
      <Typography variant="h5" sx={{ fontWeight: 600, mb: 3 }}>
        Quotations
      </Typography>

      {vendorFilter > 0 && (
        <VendorFilterNotice vendorId={vendorFilter} vendorName={vendorName} basePath="/quotations" />
      )}

      {isLoading && <CircularProgress size={24} />}
      {error && <Alert severity="error">Failed to load quotations.</Alert>}

      {data && rows.length === 0 && (
        <Paper variant="outlined" sx={{ p: 6, textAlign: "center", borderStyle: "dashed" }}>
          <Typography variant="body2" color="text.secondary">
            {vendorFilter > 0
              ? "No quotations for this vendor."
              : "No quotations yet. Open a purchase request to add one."}
          </Typography>
        </Paper>
      )}

      {data && rows.length > 0 && (
        <TableContainer component={Paper} variant="outlined">
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>Ref</TableCell>
                <TableCell>Vendor</TableCell>
                <TableCell>Amount</TableCell>
                <TableCell>Request</TableCell>
                <TableCell>Status</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {rows.map((q) => (
                <TableRow key={q.id} hover>
                  <TableCell>
                    <MuiLink component={Link} to={`/quotations/${q.id}`} sx={{ fontWeight: 500 }}>
                      {quoRef(q.id)}
                    </MuiLink>
                  </TableCell>
                  <TableCell>{q.vendor?.name ?? `Vendor #${q.vendor_id}`}</TableCell>
                  <TableCell>{formatMoney(q.total_amount, q.currency)}</TableCell>
                  <TableCell>
                    <MuiLink component={Link} to={`/requests/${q.purchase_request_id}`}>
                      {`#${q.purchase_request_id}`}
                    </MuiLink>
                  </TableCell>
                  <TableCell>
                    <EntityStatusBadge status={q.status} />
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
