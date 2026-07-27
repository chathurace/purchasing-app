import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  Box,
  Breadcrumbs,
  Button,
  Card,
  CardContent,
  IconButton,
  Link as MuiLink,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { X } from "@wso2/oxygen-ui-icons-react";
import { useContract } from "../hooks/useContracts";
import { InvoiceFields } from "../components/InvoiceFields";
import { createInvoice, uploadInvoiceDocument } from "../api/invoices";
import { ApiError } from "../api/client";
import { allocationsValid, conRef, invoiceEffectiveTotal } from "../types/api";
import type { InvoiceInput } from "../types/api";

function today(): string {
  return new Date().toISOString().slice(0, 10);
}

export function NewInvoicePage() {
  const { id } = useParams();
  const conId = Number(id);
  const navigate = useNavigate();
  const qc = useQueryClient();
  const { data: c } = useContract(conId);

  const [value, setValue] = useState<InvoiceInput>({
    vendor_invoice_no: "",
    invoice_date: today(),
    due_date: null,
    currency: "USD",
    note: "",
    allocation_mode: "percentage",
    entered_total: null,
    items: [],
    cost_allocations: [],
  });
  const [files, setFiles] = useState<File[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [prefilled, setPrefilled] = useState(false);

  // Once the contract loads, default the currency to the contract's and, when
  // its purchase request has a budget unit, pre-fill a single 100% allocation.
  useEffect(() => {
    if (c && !prefilled) {
      setValue((v) => ({
        ...v,
        currency: c.currency,
        cost_allocations: c.business_unit
          ? [{ business_unit_id: c.business_unit.id, value: 100 }]
          : v.cost_allocations,
      }));
      setPrefilled(true);
    }
  }, [c, prefilled]);

  const allocOk = allocationsValid(
    value.allocation_mode,
    value.cost_allocations,
    invoiceEffectiveTotal(value),
  );

  const mutation = useMutation({
    mutationFn: async () => {
      const inv = await createInvoice(conId, {
        ...value,
        items: value.items.filter((it) => it.description.trim() !== ""),
      });
      for (const f of files) {
        await uploadInvoiceDocument(inv.id, f);
      }
      return inv;
    },
    onSuccess: (inv) => {
      qc.invalidateQueries({ queryKey: ["invoices"] });
      qc.invalidateQueries({ queryKey: ["invoices", "contract", conId] });
      qc.invalidateQueries({ queryKey: ["contracts", conId] });
      navigate(`/invoices/${inv.id}`, { replace: true });
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to create invoice"),
  });

  const onPickFiles = (list: FileList | null) => {
    if (!list) return;
    const bad = Array.from(list).find((f) => !/\.(pdf|docx)$/i.test(f.name));
    if (bad) {
      setError(`Only .pdf and .docx files are allowed (got ${bad.name}).`);
      return;
    }
    setError(null);
    setFiles((prev) => [...prev, ...Array.from(list)]);
  };

  return (
    <Box sx={{ maxWidth: 672, mx: "auto", p: { xs: 2, md: 4 } }}>
      <Breadcrumbs sx={{ mb: 2 }} separator="/">
        <MuiLink component={Link} to="/contracts" color="primary" underline="hover">
          Contracts
        </MuiLink>
        <MuiLink component={Link} to={`/contracts/${conId}`} color="primary" underline="hover">
          {conRef(conId)}
        </MuiLink>
        <Typography color="text.secondary">New invoice</Typography>
      </Breadcrumbs>

      <Typography variant="h5" sx={{ fontWeight: 600 }}>
        New invoice
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5, mb: 3 }}>
        Recording an invoice against {c?.vendor?.name ? `${c.vendor.name}'s ` : "the "}contract {conRef(conId)}.
      </Typography>

      <Card variant="outlined">
        <CardContent>
          <InvoiceFields value={value} onChange={setValue} />

          <Box sx={{ mt: 2.5 }}>
            <Typography variant="subtitle2" sx={{ mb: 1 }}>
              Documents (PDF or DOCX)
            </Typography>
            <input
              type="file"
              multiple
              accept=".pdf,.docx"
              onChange={(e) => onPickFiles(e.target.files)}
            />
            {files.length > 0 && (
              <Stack spacing={1} sx={{ mt: 2 }}>
                {files.map((f, i) => (
                  <Box
                    key={i}
                    sx={{
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "space-between",
                      gap: 1,
                      px: 1,
                      py: 0.5,
                      borderRadius: 1,
                      bgcolor: "action.hover",
                    }}
                  >
                    <Typography variant="body2">{f.name}</Typography>
                    <IconButton
                      size="small"
                      aria-label="Remove"
                      onClick={() => setFiles((prev) => prev.filter((_, idx) => idx !== i))}
                    >
                      <X size={16} />
                    </IconButton>
                  </Box>
                ))}
              </Stack>
            )}
          </Box>

          {error && (
            <Alert severity="error" sx={{ mt: 2 }}>
              {error}
            </Alert>
          )}

          <Stack direction="row" spacing={1} sx={{ mt: 3 }}>
            <Button
              variant="contained"
              onClick={() => mutation.mutate()}
              disabled={mutation.isPending || !allocOk}
              title={allocOk ? undefined : "Assign budget units that add up before saving"}
            >
              {mutation.isPending ? "Saving…" : "Create invoice"}
            </Button>
            <Button
              variant="outlined"
              color="inherit"
              onClick={() => navigate(`/contracts/${conId}`)}
            >
              Cancel
            </Button>
          </Stack>
        </CardContent>
      </Card>
    </Box>
  );
}
