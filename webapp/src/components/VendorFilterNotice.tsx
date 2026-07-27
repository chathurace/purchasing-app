import { Link } from "react-router-dom";
import { Alert, Box, Link as MuiLink } from "@wso2/oxygen-ui";
import { vendorRef } from "../types/api";

// Shown on the entity list pages when they are filtered to a single vendor via
// the `?vendor=<id>` query param (e.g. from a vendor's "where used" panel).
export function VendorFilterNotice({
  vendorId,
  vendorName,
  basePath,
}: {
  vendorId: number;
  vendorName?: string;
  basePath: string;
}) {
  return (
    <Alert
      severity="info"
      sx={{ mb: 2, alignItems: "center" }}
      action={
        <MuiLink component={Link} to={basePath} variant="body2">
          Show all
        </MuiLink>
      }
    >
      Showing records for{" "}
      <Box component="span" sx={{ fontWeight: 600 }}>
        {vendorName || vendorRef(vendorId)}
      </Box>
    </Alert>
  );
}
