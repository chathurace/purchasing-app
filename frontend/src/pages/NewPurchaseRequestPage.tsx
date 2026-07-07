import { useEffect, useState } from "react";
import { useNavigate, Link } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { RequisitionForm, emptyRequisition, requisitionTitle } from "../components/RequisitionForm";
import { createPurchaseRequest } from "../api/purchaseRequests";
import { ApiError } from "../api/client";
import { useMe } from "../hooks/useMe";
import type { PurchaseRequestInput } from "../types/api";

export function NewPurchaseRequestPage() {
  const navigate = useNavigate();
  const qc = useQueryClient();
  const { data: me } = useMe();
  const [value, setValue] = useState<PurchaseRequestInput>(emptyRequisition());
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
    mutationFn: () => createPurchaseRequest({ ...value, title: requisitionTitle(value) }),
    onSuccess: (pr) => {
      qc.invalidateQueries({ queryKey: ["purchase-requests"] });
      navigate(`/requests/${pr.id}`, { replace: true });
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to create request"),
  });

  return (
    <div className="mx-auto max-w-3xl">
      <div className="mb-4 flex items-center gap-2 text-sm text-gray-500">
        <Link to="/requests" className="text-indigo-600">
          Requests
        </Link>
        <span>/</span>
        <span>New</span>
      </div>
      <h1 className="mb-6 text-xl font-semibold text-gray-900">New purchase requisition</h1>

      <RequisitionForm
        value={value}
        onChange={setValue}
        onSubmit={() => {
          setError(null);
          mutation.mutate();
        }}
        submitting={mutation.isPending}
        submitLabel="Submit requisition"
        requireDeclaration
        error={error}
      />
    </div>
  );
}
