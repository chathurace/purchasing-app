import { apiFetch, downloadFile } from "./client";
import type { Document, Quotation, QuotationInput } from "../types/api";

export const listQuotations = () => apiFetch<Quotation[]>("/api/v1/quotations");

export const listQuotationsForPR = (prId: number) =>
  apiFetch<Quotation[]>(`/api/v1/purchase-requests/${prId}/quotations`);

export const getQuotation = (id: number) => apiFetch<Quotation>(`/api/v1/quotations/${id}`);

export const createQuotation = (prId: number, input: QuotationInput) =>
  apiFetch<Quotation>(`/api/v1/purchase-requests/${prId}/quotations`, { method: "POST", body: input });

export const updateQuotation = (id: number, input: QuotationInput) =>
  apiFetch<Quotation>(`/api/v1/quotations/${id}`, { method: "PUT", body: input });

export const selectQuotation = (id: number) =>
  apiFetch<Quotation>(`/api/v1/quotations/${id}/select`, { method: "POST" });

export const deleteQuotation = (id: number) =>
  apiFetch<void>(`/api/v1/quotations/${id}`, { method: "DELETE" });

// --- primary quotation PDFs: initial + final (each replaceable + removable) ---

export type QuotationPdfSlot = "initial" | "final";

const pdfPath = (id: number, slot: QuotationPdfSlot) =>
  `/api/v1/quotations/${id}/${slot}-quotation-document`;

export const uploadQuotationPDF = (id: number, slot: QuotationPdfSlot, file: File) => {
  const fd = new FormData();
  fd.append("file", file);
  return apiFetch<Quotation>(pdfPath(id, slot), { method: "POST", body: fd });
};

export const deleteQuotationPDF = (id: number, slot: QuotationPdfSlot) =>
  apiFetch<Quotation>(pdfPath(id, slot), { method: "DELETE" });

// --- other supporting documents (zero or more) ---

export const uploadQuotationDocument = (id: number, file: File) => {
  const fd = new FormData();
  fd.append("file", file);
  return apiFetch<Document>(`/api/v1/quotations/${id}/documents`, { method: "POST", body: fd });
};

export const deleteQuotationDocument = (id: number, docId: number) =>
  apiFetch<void>(`/api/v1/quotations/${id}/documents/${docId}`, { method: "DELETE" });

export const downloadQuotationDocument = (id: number, doc: Document) =>
  downloadFile(`/api/v1/quotations/${id}/documents/${doc.id}/download`, doc.filename);
