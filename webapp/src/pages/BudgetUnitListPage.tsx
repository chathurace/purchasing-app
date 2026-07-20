import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useBudgetUnits } from "../hooks/useBudgetUnits";
import { useCanManageBudgetUnits } from "../hooks/useCanManageBudgetUnits";
import { createBudgetUnit } from "../api/budgetUnits";
import { ApiError } from "../api/client";
import { BudgetUnitFields } from "../components/BudgetUnitFields";
import { budgetUnitRef, emptyBudgetUnit, formatMoney, type BudgetUnitInput } from "../types/api";

type StatusFilter = "active" | "inactive" | "all";

export function BudgetUnitListPage() {
  const canManage = useCanManageBudgetUnits();
  const { data: budgetUnits, isLoading, error } = useBudgetUnits();
  const qc = useQueryClient();

  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("active");
  const [adding, setAdding] = useState(false);
  const [draft, setDraft] = useState<BudgetUnitInput>(emptyBudgetUnit);
  const [formError, setFormError] = useState<string | null>(null);

  const create = useMutation({
    mutationFn: () => createBudgetUnit({ ...draft, name: draft.name.trim() }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["budget_units"] });
      setAdding(false);
      setDraft(emptyBudgetUnit);
      setFormError(null);
    },
    onError: (e) =>
      setFormError(e instanceof ApiError ? e.message : "Failed to create budget unit"),
  });

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return (budgetUnits ?? [])
      .filter((c) =>
        statusFilter === "all" ? true : statusFilter === "active" ? c.is_active : !c.is_active,
      )
      .filter((c) =>
        q === ""
          ? true
          : c.name.toLowerCase().includes(q) || c.code.toLowerCase().includes(q),
      );
  }, [budgetUnits, search, statusFilter]);

  if (!canManage) {
    return (
      <div className="rounded border border-dashed bg-white p-8 text-center text-gray-500">
        You need the admin or procurement_admin role to manage budget units.
      </div>
    );
  }

  return (
    <div>
      <div className="mb-1 flex items-center justify-between">
        <h1 className="text-xl font-semibold text-gray-900">Budget units</h1>
        <button
          type="button"
          onClick={() => {
            setAdding((a) => !a);
            setFormError(null);
            setDraft(emptyBudgetUnit);
          }}
          className="rounded bg-indigo-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-indigo-700"
        >
          {adding ? "Cancel" : "+ New budget unit"}
        </button>
      </div>
      <p className="mb-6 text-sm text-gray-500">
        Manage the budget units selectable on purchase requests. Each unit has value-based approval
        brackets that resolve the budget approver. Deactivate a unit to retire it without losing
        history — inactive units can no longer be selected on new requests.
      </p>

      {adding && (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            create.mutate();
          }}
          className="mb-6 rounded border bg-white p-4"
        >
          <BudgetUnitFields value={draft} onChange={setDraft} />
          {formError && <p className="mt-2 text-sm text-red-600">{formError}</p>}
          <div className="mt-4">
            <button
              type="submit"
              disabled={create.isPending || draft.name.trim() === ""}
              className="rounded bg-indigo-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-indigo-700 disabled:opacity-50"
            >
              {create.isPending ? "Adding…" : "Add budget unit"}
            </button>
          </div>
        </form>
      )}

      <div className="mb-4 flex flex-wrap items-center gap-3">
        <input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search by name or code…"
          className="w-72 rounded border px-3 py-1.5 text-sm"
        />
        <select
          value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value as StatusFilter)}
          className="rounded border px-2 py-1.5 text-sm text-gray-700"
        >
          <option value="active">Active</option>
          <option value="inactive">Inactive</option>
          <option value="all">All</option>
        </select>
      </div>

      {isLoading && <p className="text-gray-500">Loading…</p>}
      {error && <p className="text-red-600">Failed to load budget units.</p>}

      {budgetUnits && (
        <div className="overflow-hidden rounded border bg-white">
          <table className="w-full text-sm">
            <thead className="border-b bg-gray-50 text-left text-gray-500">
              <tr>
                <th className="px-4 py-2 font-medium">Budget unit</th>
                <th className="px-4 py-2 font-medium">Brackets</th>
                <th className="px-4 py-2 font-medium">Budget</th>
                <th className="px-4 py-2 font-medium">Status</th>
              </tr>
            </thead>
            <tbody>
              {filtered.length === 0 && (
                <tr>
                  <td colSpan={4} className="px-4 py-6 text-center text-gray-400">
                    No budget units match.
                  </td>
                </tr>
              )}
              {filtered.map((c) => (
                <tr key={c.id} className={`border-b last:border-0 ${c.is_active ? "" : "bg-gray-50"}`}>
                  <td className="px-4 py-3 align-top">
                    <Link to={`/budget-units/${c.id}`} className="font-medium text-indigo-600">
                      {c.name}
                    </Link>
                    <div className="text-xs text-gray-400">{c.code || budgetUnitRef(c.id)}</div>
                  </td>
                  <td className="px-4 py-3 align-top text-gray-700">
                    {c.brackets.length} {c.brackets.length === 1 ? "bracket" : "brackets"}
                  </td>
                  <td className="px-4 py-3 align-top text-gray-700">
                    {c.budget ? formatMoney(c.budget, c.currency) : "—"}
                  </td>
                  <td className="px-4 py-3 align-top">
                    {c.is_active ? (
                      <span className="rounded bg-green-100 px-2 py-0.5 text-xs font-medium text-green-700">
                        Active
                      </span>
                    ) : (
                      <span className="rounded bg-red-100 px-2 py-0.5 text-xs font-medium text-red-700">
                        Inactive
                      </span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
