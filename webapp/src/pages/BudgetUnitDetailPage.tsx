import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useBudgetUnit, useBudgetUnitInvoices, useBudgetUnitUsage } from "../hooks/useBudgetUnits";
import { useCanManageBudgetUnits } from "../hooks/useCanManageBudgetUnits";
import { updateBudgetUnit } from "../api/budgetUnits";
import { ApiError } from "../api/client";
import { BudgetUnitFields } from "../components/BudgetUnitFields";
import {
  budgetUnitRef,
  formatMoney,
  type BudgetUnit,
  type BudgetUnitBracket,
  type BudgetUnitInput,
  type BudgetUnitInvoiceCategory,
} from "../types/api";

const labelCls = "font-medium text-gray-500";

function toInput(c: BudgetUnit): BudgetUnitInput {
  return {
    code: c.code,
    name: c.name,
    description: c.description,
    budget: c.budget,
    currency: c.currency,
    is_active: c.is_active,
    default_approver_id: c.default_approver_id,
    brackets: c.brackets.map((b) => ({
      currency: b.currency,
      min_value: b.min_value,
      max_value: b.max_value,
      approver_ids: b.approvers.map((u) => u.id),
    })),
  };
}

export function BudgetUnitDetailPage() {
  const { id } = useParams();
  const budgetUnitId = Number(id);
  const canManage = useCanManageBudgetUnits();
  const qc = useQueryClient();
  const { data: budgetUnit, isLoading, error } = useBudgetUnit(budgetUnitId);
  const { data: usage } = useBudgetUnitUsage(budgetUnitId);
  const { data: invoices } = useBudgetUnitInvoices(budgetUnitId);

  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState<BudgetUnitInput | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["budget_units"] });
    qc.invalidateQueries({ queryKey: ["budget_units", budgetUnitId] });
  };

  const save = useMutation({
    mutationFn: (input: BudgetUnitInput) =>
      updateBudgetUnit(budgetUnitId, { ...input, name: input.name.trim() }),
    onSuccess: () => {
      invalidate();
      setEditing(false);
      setActionError(null);
    },
    onError: (e) => setActionError(e instanceof ApiError ? e.message : "Failed to save"),
  });

  const toggleActive = useMutation({
    mutationFn: () =>
      updateBudgetUnit(budgetUnitId, {
        ...toInput(budgetUnit!),
        is_active: !budgetUnit!.is_active,
      }),
    onSuccess: invalidate,
    onError: (e) => setActionError(e instanceof ApiError ? e.message : "Failed to update status"),
  });

  if (!canManage) {
    return (
      <div className="rounded border border-dashed bg-white p-8 text-center text-gray-500">
        You need the admin or procurement_admin role to manage budget units.
      </div>
    );
  }
  if (isLoading) return <p className="text-gray-500">Loading…</p>;
  if (error || !budgetUnit) return <p className="text-red-600">Failed to load budget unit.</p>;

  const startEdit = () => {
    setDraft(toInput(budgetUnit));
    setEditing(true);
    setActionError(null);
  };

  return (
    <div className="mx-auto max-w-2xl">
      <div className="mb-4 flex items-center gap-2 text-sm text-gray-500">
        <Link to="/budget-units" className="text-indigo-600">
          Budget units
        </Link>
        <span>/</span>
        <span>{budgetUnit.code || budgetUnitRef(budgetUnit.id)}</span>
      </div>

      <div className="mb-4 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">{budgetUnit.name}</h1>
          <div className="mt-1">
            {budgetUnit.is_active ? (
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
              {budgetUnit.is_active ? "Deactivate" : "Reactivate"}
            </button>
          </div>
        )}
      </div>

      {actionError && <p className="mb-4 text-sm text-red-600">{actionError}</p>}

      <div className="rounded border bg-white p-6">
        {editing && draft ? (
          <div className="space-y-4">
            <BudgetUnitFields value={draft} onChange={setDraft} />
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
          <>
            <dl className="grid grid-cols-2 gap-x-6 gap-y-4 text-sm">
              <Field label="Code" value={budgetUnit.code} />
              <Field
                label="Budget"
                value={budgetUnit.budget ? formatMoney(budgetUnit.budget, budgetUnit.currency) : ""}
              />
              <Field label="Budget currency" value={budgetUnit.currency} />
              <Field
                label="Default approver"
                value={
                  budgetUnit.default_approver
                    ? budgetUnit.default_approver.name || budgetUnit.default_approver.email
                    : ""
                }
              />
              <div className="col-span-2">
                <dt className={labelCls}>Description</dt>
                <dd className="mt-0.5 whitespace-pre-wrap text-gray-900">
                  {budgetUnit.description || "—"}
                </dd>
              </div>
            </dl>
            <div className="mt-5">
              <h2 className="mb-2 text-sm font-medium text-gray-900">Approval brackets</h2>
              {budgetUnit.brackets.length === 0 ? (
                <p className="text-sm text-gray-400">No brackets configured.</p>
              ) : (
                <ol className="space-y-2">
                  {budgetUnit.brackets.map((b, i) => (
                    <BracketView key={b.id} index={i} bracket={b} />
                  ))}
                </ol>
              )}
            </div>
          </>
        )}
      </div>

      {/* Invoices allocated to this budget unit, by status */}
      <div className="mt-6 rounded border bg-white p-6">
        <h2 className="mb-1 font-medium text-gray-900">Invoices</h2>
        <p className="mb-3 text-xs text-gray-400">
          Each invoice's allocated share to this budget unit, grouped by status.
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
            <Link to={`/requests?budget_unit=${budgetUnit.id}`} className="rounded border p-3 hover:bg-gray-50">
              <div className="text-2xl font-semibold text-gray-900">{usage.purchase_requests}</div>
              <div className="text-xs text-gray-500">Purchase requests</div>
            </Link>
          </div>
        )}
        <p className="mt-3 text-xs text-gray-400">
          Budget units referenced by these records cannot be deleted — deactivate instead.
        </p>
      </div>
    </div>
  );
}

function BracketView({ index, bracket }: { index: number; bracket: BudgetUnitBracket }) {
  const range =
    bracket.max_value == null
      ? `${formatMoney(bracket.min_value, bracket.currency)} and above`
      : `${formatMoney(bracket.min_value, bracket.currency)} – ${formatMoney(bracket.max_value, bracket.currency)}`;
  return (
    <li className="rounded border border-gray-200 px-3 py-2">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium text-gray-800">
          {index + 1}. {range}
        </span>
      </div>
      <div className="mt-1 text-sm text-gray-600">
        {bracket.approvers.length === 0
          ? "No approvers"
          : bracket.approvers.map((u) => u.name || u.email).join(", ")}
      </div>
    </li>
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

function InvoiceCategory({ label, category }: { label: string; category: BudgetUnitInvoiceCategory }) {
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
