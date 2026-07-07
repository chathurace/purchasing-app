import { apiFetch, downloadFile } from "./client";
import type { Document, Invoice, InvoiceInput, InvoiceStatus } from "../types/api";

export const listInvoices = () => apiFetch<Invoice[]>("/api/v1/invoices");

export const listInvoicesForContract = (contractId: number) =>
  apiFetch<Invoice[]>(`/api/v1/contracts/${contractId}/invoices`);

export const getInvoice = (id: number) => apiFetch<Invoice>(`/api/v1/invoices/${id}`);

export const createInvoice = (contractId: number, input: InvoiceInput) =>
  apiFetch<Invoice>(`/api/v1/contracts/${contractId}/invoices`, { method: "POST", body: input });

export const updateInvoice = (id: number, input: InvoiceInput) =>
  apiFetch<Invoice>(`/api/v1/invoices/${id}`, { method: "PUT", body: input });

export const setInvoiceStatus = (id: number, status: InvoiceStatus) =>
  apiFetch<Invoice>(`/api/v1/invoices/${id}/status`, { method: "PUT", body: { status } });

export const deleteInvoice = (id: number) =>
  apiFetch<void>(`/api/v1/invoices/${id}`, { method: "DELETE" });

export const uploadInvoiceDocument = (id: number, file: File) => {
  const fd = new FormData();
  fd.append("file", file);
  return apiFetch<Document>(`/api/v1/invoices/${id}/documents`, { method: "POST", body: fd });
};

export const deleteInvoiceDocument = (id: number, docId: number) =>
  apiFetch<void>(`/api/v1/invoices/${id}/documents/${docId}`, { method: "DELETE" });

export const downloadInvoiceDocument = (id: number, doc: Document) =>
  downloadFile(`/api/v1/invoices/${id}/documents/${doc.id}/download`, doc.filename);
