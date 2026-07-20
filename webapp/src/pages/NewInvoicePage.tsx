import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
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
        cost_allocations: c.budget_unit
          ? [{ budget_unit_id: c.budget_unit.id, value: 100 }]
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
    <div className="mx-auto max-w-2xl">
      <div className="mb-4 flex items-center gap-2 text-sm text-gray-500">
        <Link to="/contracts" className="text-indigo-600">
          Contracts
        </Link>
        <span>/</span>
        <Link to={`/contracts/${conId}`} className="text-indigo-600">
          {conRef(conId)}
        </Link>
        <span>/</span>
        <span>New invoice</span>
      </div>
      <h1 className="mb-1 text-xl font-semibold text-gray-900">New invoice</h1>
      <p className="mb-6 text-sm text-gray-500">
        Recording an invoice against {c?.vendor?.name ? `${c.vendor.name}'s ` : "the "}contract {conRef(conId)}.
      </p>

      <div className="rounded border bg-white p-6">
        <InvoiceFields value={value} onChange={setValue} />

        <div className="mt-5">
          <label className="mb-1 block text-sm font-medium text-gray-700">
            Documents (PDF or DOCX)
          </label>
          <input
            type="file"
            multiple
            accept=".pdf,.docx"
            onChange={(e) => onPickFiles(e.target.files)}
            className="text-sm"
          />
          {files.length > 0 && (
            <ul className="mt-2 space-y-1 text-sm text-gray-600">
              {files.map((f, i) => (
                <li key={i} className="flex items-center justify-between rounded bg-gray-50 px-2 py-1">
                  <span>{f.name}</span>
                  <button
                    type="button"
                    className="text-gray-400 hover:text-red-600"
                    onClick={() => setFiles((prev) => prev.filter((_, idx) => idx !== i))}
                  >
                    Remove
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>

        {error && <p className="mt-4 text-sm text-red-600">{error}</p>}

        <div className="mt-6 flex gap-2">
          <button
            onClick={() => mutation.mutate()}
            disabled={mutation.isPending || !allocOk}
            title={allocOk ? undefined : "Assign budget units that add up before saving"}
            className="rounded bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-700 disabled:opacity-50"
          >
            {mutation.isPending ? "Saving…" : "Create invoice"}
          </button>
          <button
            onClick={() => navigate(`/contracts/${conId}`)}
            className="rounded border px-4 py-2 text-sm text-gray-700 hover:bg-gray-50"
          >
            Cancel
          </button>
        </div>
      </div>
    </div>
  );
}
