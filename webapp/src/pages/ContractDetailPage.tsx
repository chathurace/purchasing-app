import { Link, useParams } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  Box,
  Card,
  CardContent,
  CircularProgress,
  Link as MuiLink,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { useContract } from "../hooks/useContracts";
import { useProcurementAccess } from "../hooks/useProcurementAccess";
import { EntityStatusBadge } from "../components/EntityStatusBadge";
import { ContractContent } from "../components/ContractContent";
import { RelatedEntities } from "../components/RelatedEntities";
import { ChainStepper } from "../components/ChainStepper";
import { DirectParentCard } from "../components/CaseSections";
import { ContractFulfillment } from "../components/ContractFulfillment";
import { conRef, formatMoney } from "../types/api";

export function ContractDetailPage() {
  const { id } = useParams();
  const conId = Number(id);
  const qc = useQueryClient();
  const { data: c, isLoading, error } = useContract(conId);
  const procurement = useProcurementAccess();

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["contracts", conId] });
    qc.invalidateQueries({ queryKey: ["contracts"] });
    qc.invalidateQueries({ queryKey: ["purchase-requests"] });
  };

  if (isLoading)
    return (
      <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
        <CircularProgress size={24} />
      </Box>
    );
  if (error || !c)
    return (
      <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
        <Alert severity="error">Failed to load contract.</Alert>
      </Box>
    );

  return (
    <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
      <Stack direction="row" spacing={1} alignItems="center" sx={{ mb: 2 }}>
        <MuiLink component={Link} to="/contracts" variant="body2">
          Contracts
        </MuiLink>
        <Typography variant="body2" color="text.secondary">
          /
        </Typography>
        <Typography variant="body2" color="text.secondary">
          {conRef(c.id)}
        </Typography>
      </Stack>

      <ChainStepper prId={c.purchase_request_id} current={{ kind: "contract", id: c.id }} />

      <Box sx={{ mb: 2 }}>
        <Typography variant="h5" sx={{ fontWeight: 600 }}>
          {c.title || conRef(c.id)}
        </Typography>
        <Stack direction="row" spacing={1.5} alignItems="center" flexWrap="wrap" sx={{ mt: 1 }}>
          <EntityStatusBadge status={c.status} />
          <Typography variant="body2" color="text.secondary">
            {c.vendor?.name ?? `Vendor #${c.vendor_id}`}
          </Typography>
          <Typography variant="body2" color="text.secondary" aria-hidden>
            ·
          </Typography>
          <Typography variant="body2" color="text.secondary">
            {formatMoney(c.total_amount, c.currency)}
          </Typography>
        </Stack>
      </Box>

      <Card variant="outlined">
        <CardContent>
          <Typography variant="subtitle1" sx={{ fontWeight: 600, mb: 2 }}>
            Contract
          </Typography>
          <ContractContent contract={c} canEdit={procurement} invalidate={invalidate} />
        </CardContent>
      </Card>

      <DirectParentCard prId={c.purchase_request_id} current={{ kind: "contract", id: c.id }} />

      {/* Fulfillment: GRNs and invoices (once signed) — procurement-only; approvers
          get a read-only view of the contract itself without fulfillment. */}
      {procurement && <ContractFulfillment contract={c} />}

      <RelatedEntities prId={c.purchase_request_id} current={{ kind: "contract", id: c.id }} />
    </Box>
  );
}
