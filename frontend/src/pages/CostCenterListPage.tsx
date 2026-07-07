import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useCostCenters } from "../hooks/useCostCenters";
import { useCanManageCostCenters } from "../hooks/useCanManageCostCenters";
import { createCostCenter } from "../api/costCenters";
import { ApiError } from "../api/client";
import { CostCenterFields } from "../components/CostCenterFields";
import { costCenterRef, emptyCostCenter, formatMoney, type CostCenterInput } from "../types/api";

type StatusFilter = "active" | "inactive" | "all";

export function CostCenterListPage() {
  const canManage = useCanManageCostCenters();
  const { data: costCenters, isLoading, error } = useCostCenters();
  const qc = useQueryClient();

  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("active");
  const [adding, setAdding] = useState(false);
  const [draft, setDraft] = useState<CostCenterInput>(emptyCostCenter);
  const [formError, setFormError] = useState<string | null>(null);

  const create = useMutation({
    mutationFn: () => createCostCenter({ ...draft, name: draft.name.trim() }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["cost_centers"] });
      setAdding(false);
      setDraft(emptyCostCenter);
      setFormError(null);
    },
    onError: (e) =>
      setFormError(e instanceof ApiError ? e.message : "Failed to create cost center"),
  });

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return (costCenters ?? [])
      .filter((c) =>
        statusFilter === "all" ? true : statusFilter === "active" ? c.is_active : !c.is_active,
      )
      .filter((c) =>
        q === ""
          ? true
          : c.name.toLowerCase().includes(q) ||
            c.code.toLowerCase().includes(q) ||
            (c.primary_owner?.name ?? "").toLowerCase().includes(q) ||
            (c.primary_owner?.email ?? "").toLowerCase().includes(q),
      );
  }, [costCenters, search, statusFilter]);

  if (!canManage) {
    return (
      <div className="rounded border border-dashed bg-white p-8 text-center text-gray-500">
        You need the admin or finance_admin role to manage cost centers.
      </div>
    );
  }

  return (
    <div>
      <div className="mb-1 flex items-center justify-between">
        <h1 className="text-xl font-semibold text-gray-900">Cost centers</h1>
        <button
          type="button"
          onClick={() => {
            setAdding((a) => !a);
            setFormError(null);
            setDraft(emptyCostCenter);
          }}
          className="rounded bg-indigo-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-indigo-700"
        >
          {adding ? "Cancel" : "+ New cost center"}
        </button>
      </div>
      <p className="mb-6 text-sm text-gray-500">
        Manage the cost centers selectable on purchase requests. Deactivate a cost center to retire
        it without losing history — inactive cost centers can no longer be selected on new requests.
      </p>

      {adding && (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            create.mutate();
          }}
          className="mb-6 rounded border bg-white p-4"
        >
          <CostCenterFields value={draft} onChange={setDraft} />
          {formError && <p className="mt-2 text-sm text-red-600">{formError}</p>}
          <div className="mt-4">
            <button
              type="submit"
              disabled={create.isPending || draft.name.trim() === ""}
              className="rounded bg-indigo-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-indigo-700 disabled:opacity-50"
            >
              {create.isPending ? "Adding…" : "Add cost center"}
            </button>
          </div>
        </form>
      )}

      <div className="mb-4 flex flex-wrap items-center gap-3">
        <input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search by name, code or owner…"
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
      {error && <p className="text-red-600">Failed to load cost centers.</p>}

      {costCenters && (
        <div className="overflow-hidden rounded border bg-white">
          <table className="w-full text-sm">
            <thead className="border-b bg-gray-50 text-left text-gray-500">
              <tr>
                <th className="px-4 py-2 font-medium">Cost center</th>
                <th className="px-4 py-2 font-medium">Primary owner</th>
                <th className="px-4 py-2 font-medium">Budget</th>
                <th className="px-4 py-2 font-medium">Status</th>
              </tr>
            </thead>
            <tbody>
              {filtered.length === 0 && (
                <tr>
                  <td colSpan={4} className="px-4 py-6 text-center text-gray-400">
                    No cost centers match.
                  </td>
                </tr>
              )}
              {filtered.map((c) => (
                <tr key={c.id} className={`border-b last:border-0 ${c.is_active ? "" : "bg-gray-50"}`}>
                  <td className="px-4 py-3 align-top">
                    <Link to={`/cost-centers/${c.id}`} className="font-medium text-indigo-600">
                      {c.name}
                    </Link>
                    <div className="text-xs text-gray-400">{c.code || costCenterRef(c.id)}</div>
                  </td>
                  <td className="px-4 py-3 align-top text-gray-700">
                    {c.primary_owner ? (
                      <div>{c.primary_owner.name || c.primary_owner.email}</div>
                    ) : (
                      "—"
                    )}
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
