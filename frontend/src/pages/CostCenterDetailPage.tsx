import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useCostCenter, useCostCenterInvoices, useCostCenterUsage } from "../hooks/useCostCenters";
import { useCanManageCostCenters } from "../hooks/useCanManageCostCenters";
import { updateCostCenter } from "../api/costCenters";
import { ApiError } from "../api/client";
import { CostCenterFields } from "../components/CostCenterFields";
import {
  costCenterRef,
  formatMoney,
  type CostCenter,
  type CostCenterInput,
  type CostCenterInvoiceCategory,
} from "../types/api";

const labelCls = "font-medium text-gray-500";

function toInput(c: CostCenter): CostCenterInput {
  return {
    code: c.code,
    name: c.name,
    description: c.description,
    primary_owner_id: c.primary_owner_id,
    budget: c.budget,
    currency: c.currency,
    is_active: c.is_active,
    secondary_owner_ids: c.secondary_owners.map((u) => u.id),
  };
}

export function CostCenterDetailPage() {
  const { id } = useParams();
  const costCenterId = Number(id);
  const canManage = useCanManageCostCenters();
  const qc = useQueryClient();
  const { data: costCenter, isLoading, error } = useCostCenter(costCenterId);
  const { data: usage } = useCostCenterUsage(costCenterId);
  const { data: invoices } = useCostCenterInvoices(costCenterId);

  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState<CostCenterInput | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["cost_centers"] });
    qc.invalidateQueries({ queryKey: ["cost_centers", costCenterId] });
  };

  const save = useMutation({
    mutationFn: (input: CostCenterInput) =>
      updateCostCenter(costCenterId, { ...input, name: input.name.trim() }),
    onSuccess: () => {
      invalidate();
      setEditing(false);
      setActionError(null);
    },
    onError: (e) => setActionError(e instanceof ApiError ? e.message : "Failed to save"),
  });

  const toggleActive = useMutation({
    mutationFn: () =>
      updateCostCenter(costCenterId, {
        ...toInput(costCenter!),
        is_active: !costCenter!.is_active,
      }),
    onSuccess: invalidate,
    onError: (e) => setActionError(e instanceof ApiError ? e.message : "Failed to update status"),
  });

  if (!canManage) {
    return (
      <div className="rounded border border-dashed bg-white p-8 text-center text-gray-500">
        You need the admin or finance_admin role to manage cost centers.
      </div>
    );
  }
  if (isLoading) return <p className="text-gray-500">Loading…</p>;
  if (error || !costCenter) return <p className="text-red-600">Failed to load cost center.</p>;

  const startEdit = () => {
    setDraft(toInput(costCenter));
    setEditing(true);
    setActionError(null);
  };

  return (
    <div className="mx-auto max-w-2xl">
      <div className="mb-4 flex items-center gap-2 text-sm text-gray-500">
        <Link to="/cost-centers" className="text-indigo-600">
          Cost centers
        </Link>
        <span>/</span>
        <span>{costCenter.code || costCenterRef(costCenter.id)}</span>
      </div>

      <div className="mb-4 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">{costCenter.name}</h1>
          <div className="mt-1">
            {costCenter.is_active ? (
              <span className="rounded bg-green-100 px-2 py-0.5 text-xs font-medium text-green-700">
                Active
              </span>
            ) : (
              <span className="rounded bg-red-100 px-2 py-0.5 text-xs font-medium text-red-700">
                Inactive
              </span>
            )}
          </div>
        </div>
        {!editing && (
          <div className="flex gap-2">
            <button
              onClick={startEdit}
              className="rounded border px-3 py-2 text-sm text-gray-700 hover:bg-gray-50"
            >
              Edit details
            </button>
            <button
              onClick={() => toggleActive.mutate()}
              disabled={toggleActive.isPending}
              className="rounded border px-3 py-2 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-50"
            >
              {costCenter.is_active ? "Deactivate" : "Reactivate"}
            </button>
          </div>
        )}
      </div>

      {actionError && <p className="mb-4 text-sm text-red-600">{actionError}</p>}

      <div className="rounded border bg-white p-6">
        {editing && draft ? (
          <div className="space-y-4">
            <CostCenterFields value={draft} onChange={setDraft} />
            <div className="flex gap-2">
              <button
                onClick={() => save.mutate(draft)}
                disabled={save.isPending || draft.name.trim() === ""}
                className="rounded bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-700 disabled:opacity-50"
              >
                {save.isPending ? "Saving…" : "Save changes"}
              </button>
              <button
                onClick={() => {
                  setEditing(false);
                  setActionError(null);
                }}
                className="rounded border px-4 py-2 text-sm text-gray-700 hover:bg-gray-50"
              >
                Cancel
              </button>
            </div>
          </div>
        ) : (
          <dl className="grid grid-cols-2 gap-x-6 gap-y-4 text-sm">
            <Field label="Code" value={costCenter.code} />
            <Field
              label="Budget"
              value={costCenter.budget ? formatMoney(costCenter.budget, costCenter.currency) : ""}
            />
            <Field
              label="Primary owner"
              value={
                costCenter.primary_owner
                  ? costCenter.primary_owner.name || costCenter.primary_owner.email
                  : ""
              }
            />
            <div>
              <dt className={labelCls}>Secondary owners</dt>
              <dd className="mt-0.5 text-gray-900">
                {costCenter.secondary_owners.length === 0
                  ? "—"
                  : costCenter.secondary_owners.map((u) => u.name || u.email).join(", ")}
              </dd>
            </div>
            <div className="col-span-2">
              <dt className={labelCls}>Description</dt>
              <dd className="mt-0.5 whitespace-pre-wrap text-gray-900">
                {costCenter.description || "—"}
              </dd>
            </div>
          </dl>
        )}
      </div>

      {/* Invoices allocated to this cost center, by status */}
      <div className="mt-6 rounded border bg-white p-6">
        <h2 className="mb-1 font-medium text-gray-900">Invoices</h2>
        <p className="mb-3 text-xs text-gray-400">
          Each invoice's allocated share to this cost center, grouped by status.
        </p>
        {!invoices ? (
          <p className="text-sm text-gray-400">Loading…</p>
        ) : (
          <div className="grid grid-cols-3 gap-3">
            <InvoiceCategory label="Pending" category={invoices.pending} />
            <InvoiceCategory label="Approved" category={invoices.approved} />
            <InvoiceCategory label="Paid" category={invoices.paid} />
          </div>
        )}
      </div>

      {/* Where used */}
      <div className="mt-6 rounded border bg-white p-6">
        <h2 className="mb-3 font-medium text-gray-900">Where used</h2>
        {!usage ? (
          <p className="text-sm text-gray-400">Loading…</p>
        ) : (
          <div className="grid grid-cols-2 gap-3 text-center">
            <Link to={`/requests?cost_center=${costCenter.id}`} className="rounded border p-3 hover:bg-gray-50">
              <div className="text-2xl font-semibold text-gray-900">{usage.purchase_requests}</div>
              <div className="text-xs text-gray-500">Purchase requests</div>
            </Link>
          </div>
        )}
        <p className="mt-3 text-xs text-gray-400">
          Cost centers referenced by these records cannot be deleted — deactivate instead.
        </p>
      </div>
    </div>
  );
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className={labelCls}>{label}</dt>
      <dd className="mt-0.5 text-gray-900">{value || "—"}</dd>
    </div>
  );
}

function InvoiceCategory({ label, category }: { label: string; category: CostCenterInvoiceCategory }) {
  return (
    <div className="rounded border p-3">
      <div className="flex items-baseline justify-between">
        <span className="text-sm font-medium text-gray-700">{label}</span>
        <span className="text-xs text-gray-400">
          {category.count} {category.count === 1 ? "invoice" : "invoices"}
        </span>
      </div>
      <div className="mt-2 space-y-0.5">
        {category.totals.length === 0 ? (
          <div className="text-lg font-semibold text-gray-400">—</div>
        ) : (
          category.totals.map((t) => (
            <div key={t.currency} className="text-base font-semibold text-gray-900">
              {formatMoney(t.amount, t.currency)}
            </div>
          ))
        )}
      </div>
    </div>
  );
}
