import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Box } from "@wso2/oxygen-ui";
import { RequisitionForm, emptyRequisition, requisitionTitle } from "../components/RequisitionForm";
import { createPurchaseRequest, uploadDocument } from "../api/purchaseRequests";
import { ApiError } from "../api/client";
import { useMe } from "../hooks/useMe";
import type { PurchaseRequestInput } from "../types/api";

export function NewPurchaseRequestPage() {
  const navigate = useNavigate();
  const qc = useQueryClient();
  const { data: me } = useMe();
  const [value, setValue] = useState<PurchaseRequestInput>(emptyRequisition());
  const [attachments, setAttachments] = useState<File[]>([]);
  const [error, setError] = useState<string | null>(null);

  // Prefill the requester's name/email from their account once loaded.
  useEffect(() => {
    if (!me) return;
    setValue((v) => ({
      ...v,
      details: {
        ...v.details,
        requester_name: v.details?.requester_name || me.name || "",
        requester_email: v.details?.requester_email || me.email || "",
      },
    }));
  }, [me]);

  const mutation = useMutation({
    mutationFn: async () => {
      const pr = await createPurchaseRequest({ ...value, title: requisitionTitle(value) });
      // Upload any supplier attachments staged in the form (best-effort — the PR
      // is already created, so a failed upload shouldn't block navigation).
      for (const file of attachments) {
        try {
          await uploadDocument(pr.id, file);
        } catch {
          /* ignore — the requester can re-attach from the PR page */
        }
      }
      return pr;
    },
    onSuccess: (pr) => {
      qc.invalidateQueries({ queryKey: ["purchase-requests"] });
      navigate(`/requests/${pr.id}`, { replace: true });
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to create request"),
  });

  return (
    <Box sx={{ maxWidth: 768, mx: "auto", p: { xs: 2, md: 4 } }}>
      <RequisitionForm
        value={value}
        onChange={setValue}
        onSubmit={() => {
          setError(null);
          mutation.mutate();
        }}
        submitting={mutation.isPending}
        submitLabel="Submit Purchase Requisition"
        requireDeclaration
        error={error}
        attachments={attachments}
        onAttachmentsChange={setAttachments}
      />
    </Box>
  );
}
