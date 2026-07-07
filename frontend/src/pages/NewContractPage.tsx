import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useQuotation } from "../hooks/useQuotations";
import { ContractFields } from "../components/ContractFields";
import { createContractFromQuotation, uploadContractDocument } from "../api/contracts";
import { ApiError } from "../api/client";
import { quoRef } from "../types/api";
import type { ContractInput } from "../types/api";

export function NewContractPage() {
  const { id } = useParams();
  const quoId = Number(id);
  const navigate = useNavigate();
  const qc = useQueryClient();
  const { data: q } = useQuotation(quoId);

  const [value, setValue] = useState<ContractInput>({
    title: "",
    total_amount: 0,
    currency: "USD",
    terms: "",
  });
  const [files, setFiles] = useState<File[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [prefilled, setPrefilled] = useState(false);

  // Prefill totals/currency from the source quotation once it loads.
  useEffect(() => {
    if (q && !prefilled) {
      setValue((v) => ({
        ...v,
        title: v.title || (q.vendor?.name ? `Contract — ${q.vendor.name}` : ""),
        total_amount: q.total_amount,
        currency: q.currency,
      }));
      setPrefilled(true);
    }
  }, [q, prefilled]);

  const mutation = useMutation({
    mutationFn: async () => {
      const c = await createContractFromQuotation(quoId, value);
      for (const f of files) {
        await uploadContractDocument(c.id, f);
      }
      return c;
    },
    onSuccess: (c) => {
      qc.invalidateQueries({ queryKey: ["contracts"] });
      qc.invalidateQueries({ queryKey: ["quotations", quoId] });
      qc.invalidateQueries({ queryKey: ["purchase-requests"] });
      navigate(`/contracts/${c.id}`, { replace: true });
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to create contract"),
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
        <Link to="/quotations" className="text-indigo-600">
          Quotations
        </Link>
        <span>/</span>
        <Link to={`/quotations/${quoId}`} className="text-indigo-600">
          {quoRef(quoId)}
        </Link>
        <span>/</span>
        <span>New contract</span>
      </div>
      <h1 className="mb-6 text-xl font-semibold text-gray-900">New contract</h1>

      <div className="rounded border bg-white p-6">
        <ContractFields value={value} onChange={setValue} />

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
            disabled={mutation.isPending}
            className="rounded bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-700 disabled:opacity-50"
          >
            {mutation.isPending ? "Saving…" : "Create contract"}
          </button>
          <button
            onClick={() => navigate(`/quotations/${quoId}`)}
            className="rounded border px-4 py-2 text-sm text-gray-700 hover:bg-gray-50"
          >
            Cancel
          </button>
        </div>
      </div>
    </div>
  );
}
