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
import { useGRNs } from "../hooks/useGrns";
import { VendorFilterNotice } from "../components/VendorFilterNotice";
import { conRef, grnRef } from "../types/api";

export function GRNListPage() {
  const { data, isLoading, error } = useGRNs();
  const [params] = useSearchParams();
  const vendorFilter = Number(params.get("vendor")) || 0;
  const rows = vendorFilter ? (data ?? []).filter((g) => g.vendor_id === vendorFilter) : data ?? [];
  const vendorName = rows.find((g) => g.vendor)?.vendor?.name;

  return (
    <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
      <Typography variant="h5" sx={{ fontWeight: 600, mb: 3 }}>
        Goods received notes
      </Typography>

      {vendorFilter > 0 && (
        <VendorFilterNotice vendorId={vendorFilter} vendorName={vendorName} basePath="/grns" />
      )}

      {isLoading && <CircularProgress size={24} />}
      {error && <Alert severity="error">Failed to load GRNs.</Alert>}

      {data && rows.length === 0 && (
        <Paper variant="outlined" sx={{ p: 6, textAlign: "center", borderStyle: "dashed" }}>
          <Typography variant="body2" color="text.secondary">
            {vendorFilter > 0 ? "No GRNs for this vendor." : "No GRNs yet. Record one from a signed contract."}
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
                <TableCell>Received</TableCell>
                <TableCell>Contract</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {rows.map((g) => (
                <TableRow key={g.id} hover>
                  <TableCell>
                    <MuiLink component={Link} to={`/grns/${g.id}`} sx={{ fontWeight: 500 }}>
                      {grnRef(g.id)}
                    </MuiLink>
                  </TableCell>
                  <TableCell>{g.vendor?.name ?? `Vendor #${g.vendor_id}`}</TableCell>
                  <TableCell>{g.received_date}</TableCell>
                  <TableCell>
                    <MuiLink component={Link} to={`/contracts/${g.contract_id}`}>
                      {conRef(g.contract_id)}
                    </MuiLink>
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
