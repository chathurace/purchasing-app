import { apiFetch, downloadFile } from "./client";
import type { Document, GRN, GRNInput } from "../types/api";

export const listGRNs = () => apiFetch<GRN[]>("/api/v1/grns");

export const listGRNsForContract = (contractId: number) =>
  apiFetch<GRN[]>(`/api/v1/contracts/${contractId}/grns`);

export const getGRN = (id: number) => apiFetch<GRN>(`/api/v1/grns/${id}`);

export const createGRN = (contractId: number, input: GRNInput) =>
  apiFetch<GRN>(`/api/v1/contracts/${contractId}/grns`, { method: "POST", body: input });

export const updateGRN = (id: number, input: GRNInput) =>
  apiFetch<GRN>(`/api/v1/grns/${id}`, { method: "PUT", body: input });

export const deleteGRN = (id: number) =>
  apiFetch<void>(`/api/v1/grns/${id}`, { method: "DELETE" });

export const uploadGRNDocument = (id: number, file: File) => {
  const fd = new FormData();
  fd.append("file", file);
  return apiFetch<Document>(`/api/v1/grns/${id}/documents`, { method: "POST", body: fd });
};

export const deleteGRNDocument = (id: number, docId: number) =>
  apiFetch<void>(`/api/v1/grns/${id}/documents/${docId}`, { method: "DELETE" });

export const downloadGRNDocument = (id: number, doc: Document) =>
  downloadFile(`/api/v1/grns/${id}/documents/${doc.id}/download`, doc.filename);
