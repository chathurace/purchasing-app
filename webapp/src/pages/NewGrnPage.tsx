import { useState } from "react";
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
import { GRNFields } from "../components/GRNFields";
import { createGRN, uploadGRNDocument } from "../api/grns";
import { ApiError } from "../api/client";
import { conRef } from "../types/api";
import type { GRNInput } from "../types/api";

// today returns the local date as YYYY-MM-DD for date-input defaults.
function today(): string {
  return new Date().toISOString().slice(0, 10);
}

export function NewGrnPage() {
  const { id } = useParams();
  const conId = Number(id);
  const navigate = useNavigate();
  const qc = useQueryClient();
  const { data: c } = useContract(conId);

  const [value, setValue] = useState<GRNInput>({
    received_date: today(),
    received_by: "",
    note: "",
    items: [],
  });
  const [files, setFiles] = useState<File[]>([]);
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: async () => {
      const g = await createGRN(conId, {
        ...value,
        items: value.items.filter((it) => it.description.trim() !== ""),
      });
      for (const f of files) {
        await uploadGRNDocument(g.id, f);
      }
      return g;
    },
    onSuccess: (g) => {
      qc.invalidateQueries({ queryKey: ["grns"] });
      qc.invalidateQueries({ queryKey: ["grns", "contract", conId] });
      navigate(`/grns/${g.id}`, { replace: true });
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to create GRN"),
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
        <Typography color="text.secondary">New GRN</Typography>
      </Breadcrumbs>

      <Typography variant="h5" sx={{ fontWeight: 600 }}>
        New goods received note
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5, mb: 3 }}>
        Recording receipt against {c?.vendor?.name ? `${c.vendor.name}'s ` : "the "}contract {conRef(conId)}.
      </Typography>

      <Card variant="outlined">
        <CardContent>
          <GRNFields value={value} onChange={setValue} />

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
              disabled={mutation.isPending}
            >
              {mutation.isPending ? "Saving…" : "Create GRN"}
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
