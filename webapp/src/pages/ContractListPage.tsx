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
import { useContracts } from "../hooks/useContracts";
import { EntityStatusBadge } from "../components/EntityStatusBadge";
import { VendorFilterNotice } from "../components/VendorFilterNotice";
import { conRef, formatMoney } from "../types/api";

export function ContractListPage() {
  const { data, isLoading, error } = useContracts();
  const [params] = useSearchParams();
  const vendorFilter = Number(params.get("vendor")) || 0;
  const rows = vendorFilter ? (data ?? []).filter((c) => c.vendor_id === vendorFilter) : data ?? [];
  const vendorName = rows.find((c) => c.vendor)?.vendor?.name;

  return (
    <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
      <Box sx={{ mb: 3 }}>
        <Typography variant="h5" sx={{ fontWeight: 600 }}>
          Contracts
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
          Vendor contracts across all purchase requests.
        </Typography>
      </Box>

      {vendorFilter > 0 && (
        <VendorFilterNotice vendorId={vendorFilter} vendorName={vendorName} basePath="/contracts" />
      )}

      {isLoading && <CircularProgress size={24} />}
      {error && <Alert severity="error">Failed to load contracts.</Alert>}

      {data && rows.length === 0 && (
        <Paper variant="outlined" sx={{ p: 6, textAlign: "center", borderStyle: "dashed" }}>
          <Typography variant="body2" color="text.secondary">
            {vendorFilter > 0 ? "No contracts for this vendor." : "No contracts yet. Create one from a quotation."}
          </Typography>
        </Paper>
      )}

      {data && rows.length > 0 && (
        <TableContainer component={Paper} variant="outlined">
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>Ref</TableCell>
                <TableCell>Title</TableCell>
                <TableCell>Vendor</TableCell>
                <TableCell>Amount</TableCell>
                <TableCell>Request</TableCell>
                <TableCell>Status</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {rows.map((c) => (
                <TableRow key={c.id} hover>
                  <TableCell>
                    <MuiLink component={Link} to={`/contracts/${c.id}`} sx={{ fontWeight: 500 }}>
                      {conRef(c.id)}
                    </MuiLink>
                  </TableCell>
                  <TableCell>
                    {c.title || (
                      <Typography component="span" color="text.disabled">
                        —
                      </Typography>
                    )}
                  </TableCell>
                  <TableCell>{c.vendor?.name ?? `Vendor #${c.vendor_id}`}</TableCell>
                  <TableCell>{formatMoney(c.total_amount, c.currency)}</TableCell>
                  <TableCell>
                    <MuiLink component={Link} to={`/requests/${c.purchase_request_id}`}>
                      {`#${c.purchase_request_id}`}
                    </MuiLink>
                  </TableCell>
                  <TableCell>
                    <EntityStatusBadge status={c.status} />
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
