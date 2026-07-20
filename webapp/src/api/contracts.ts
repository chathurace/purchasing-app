import { apiFetch, downloadFile } from "./client";
import type { Contract, ContractInput, Document } from "../types/api";

export const listContracts = () => apiFetch<Contract[]>("/api/v1/contracts");

export const getContract = (id: number) => apiFetch<Contract>(`/api/v1/contracts/${id}`);

export const updateContract = (id: number, input: ContractInput) =>
  apiFetch<Contract>(`/api/v1/contracts/${id}`, { method: "PUT", body: input });

// --- signed contract PDF (single; replaceable + removable) ---

export const uploadSignedDocument = (id: number, file: File, notes = "") => {
  const fd = new FormData();
  fd.append("file", file);
  fd.append("notes", notes);
  return apiFetch<Contract>(`/api/v1/contracts/${id}/signed-document`, { method: "POST", body: fd });
};

export const deleteSignedDocument = (id: number) =>
  apiFetch<Contract>(`/api/v1/contracts/${id}/signed-document`, { method: "DELETE" });

// --- draft contract PDFs (one or more; each with its own notes) ---

export const uploadContractDocument = (id: number, file: File, notes = "") => {
  const fd = new FormData();
  fd.append("file", file);
  fd.append("notes", notes);
  return apiFetch<Document>(`/api/v1/contracts/${id}/documents`, { method: "POST", body: fd });
};

// Update a contract document's notes (draft or signed PDF). Returns the contract.
export const updateContractDocumentNotes = (id: number, docId: number, notes: string) =>
  apiFetch<Contract>(`/api/v1/contracts/${id}/documents/${docId}`, {
    method: "PUT",
    body: { notes },
  });

export const deleteContractDocument = (id: number, docId: number) =>
  apiFetch<void>(`/api/v1/contracts/${id}/documents/${docId}`, { method: "DELETE" });

export const downloadContractDocument = (id: number, doc: Document) =>
  downloadFile(`/api/v1/contracts/${id}/documents/${doc.id}/download`, doc.filename);
