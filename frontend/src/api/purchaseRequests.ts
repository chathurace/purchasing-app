import { apiFetch, downloadFile } from "./client";
import type {
  Document,
  PurchaseRequest,
  PurchaseRequestInput,
  RelatedDocuments,
} from "../types/api";

export type ApprovalDecision = "approve" | "reject";

// PRListScope mirrors the backend ?scope= filter: "mine" = only the caller's own
// submissions (Requests tab), "approvals" = only PRs awaiting the caller's
// decision (Approvals tab). Omit for the default role-based view.
export type PRListScope = "mine" | "approvals";

export const listPurchaseRequests = (scope?: PRListScope) =>
  apiFetch<PurchaseRequest[]>(
    scope ? `/api/v1/purchase-requests?scope=${scope}` : "/api/v1/purchase-requests",
  );

export const getPurchaseRequest = (id: number) =>
  apiFetch<PurchaseRequest>(`/api/v1/purchase-requests/${id}`);

// getRelatedDocuments returns the whole procurement case (PR + RFQs + quotations
// + contracts) anchored on the given PR. Finance-access only on the backend.
export const getRelatedDocuments = (prId: number) =>
  apiFetch<RelatedDocuments>(`/api/v1/purchase-requests/${prId}/related`);

export const createPurchaseRequest = (input: PurchaseRequestInput) =>
  apiFetch<PurchaseRequest>("/api/v1/purchase-requests", { method: "POST", body: input });

export const updatePurchaseRequest = (id: number, input: PurchaseRequestInput) =>
  apiFetch<PurchaseRequest>(`/api/v1/purchase-requests/${id}`, { method: "PUT", body: input });

export const rejectPurchaseRequest = (id: number, comment: string) =>
  apiFetch<PurchaseRequest>(`/api/v1/purchase-requests/${id}/reject`, {
    method: "POST",
    body: { comment },
  });

// --- approvals ---

export const addApprover = (id: number, approverId: number) =>
  apiFetch<PurchaseRequest>(`/api/v1/purchase-requests/${id}/approvers`, {
    method: "POST",
    body: { approver_id: approverId },
  });

export const removeApprover = (id: number, approverId: number) =>
  apiFetch<PurchaseRequest>(`/api/v1/purchase-requests/${id}/approvers/${approverId}`, {
    method: "DELETE",
  });

export const requestApprovalAgain = (id: number, approverId: number) =>
  apiFetch<PurchaseRequest>(`/api/v1/purchase-requests/${id}/approvers/${approverId}/request`, {
    method: "POST",
  });

export const recordApprovalDecision = (id: number, decision: ApprovalDecision, comment: string) =>
  apiFetch<PurchaseRequest>(`/api/v1/purchase-requests/${id}/approval`, {
    method: "POST",
    body: { decision, comment },
  });

export const uploadDocument = (id: number, file: File) => {
  const fd = new FormData();
  fd.append("file", file);
  return apiFetch<Document>(`/api/v1/purchase-requests/${id}/documents`, {
    method: "POST",
    body: fd,
  });
};

export const deleteDocument = (id: number, docID: number) =>
  apiFetch<void>(`/api/v1/purchase-requests/${id}/documents/${docID}`, { method: "DELETE" });

export const downloadDocument = (id: number, doc: Document) =>
  downloadFile(`/api/v1/purchase-requests/${id}/documents/${doc.id}/download`, doc.filename);
