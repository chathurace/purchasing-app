import { apiFetch, downloadFile } from "./client";
import type {
  Contract,
  Document,
  PurchaseRequest,
  RecApprovalType,
  RecComment,
  RecommendationInput,
} from "../types/api";

// All recommendation mutations return the refreshed purchase request (with the
// recommendation embedded), except adding a comment which returns the new
// comment so its documents can be attached.

export const createRecommendation = (prId: number, input: RecommendationInput) =>
  apiFetch<PurchaseRequest>(`/api/v1/purchase-requests/${prId}/recommendation`, {
    method: "POST",
    body: input,
  });

export const updateRecommendation = (prId: number, input: RecommendationInput) =>
  apiFetch<PurchaseRequest>(`/api/v1/purchase-requests/${prId}/recommendation`, {
    method: "PUT",
    body: input,
  });

export const deleteRecommendation = (prId: number) =>
  apiFetch<void>(`/api/v1/purchase-requests/${prId}/recommendation`, { method: "DELETE" });

// Drafts the contract attached to the recommendation (the optional contract
// card). Returns the new contract so its PDF can then be uploaded against it.
export const createRecommendationContract = (prId: number, description: string) =>
  apiFetch<Contract>(`/api/v1/purchase-requests/${prId}/recommendation/contract`, {
    method: "POST",
    body: { description },
  });

// Removes the contract attached to the recommendation (only while draft) and
// unlinks it. Returns the refreshed purchase request.
export const deleteRecommendationContract = (prId: number) =>
  apiFetch<PurchaseRequest>(`/api/v1/purchase-requests/${prId}/recommendation/contract`, {
    method: "DELETE",
  });

// --- RFI (request for information) card ---

export const setRecommendationRFI = (prId: number, description: string) =>
  apiFetch<PurchaseRequest>(`/api/v1/purchase-requests/${prId}/recommendation/rfi`, {
    method: "PUT",
    body: { description },
  });

export const deleteRecommendationRFI = (prId: number) =>
  apiFetch<PurchaseRequest>(`/api/v1/purchase-requests/${prId}/recommendation/rfi`, {
    method: "DELETE",
  });

export const uploadRecRFIDocument = (prId: number, file: File) => {
  const fd = new FormData();
  fd.append("file", file);
  return apiFetch<Document>(`/api/v1/purchase-requests/${prId}/recommendation/rfi/documents`, {
    method: "POST",
    body: fd,
  });
};

export const deleteRecRFIDocument = (prId: number, docId: number) =>
  apiFetch<void>(`/api/v1/purchase-requests/${prId}/recommendation/rfi/documents/${docId}`, {
    method: "DELETE",
  });

export const downloadRecRFIDocument = (prId: number, doc: Document) =>
  downloadFile(
    `/api/v1/purchase-requests/${prId}/recommendation/rfi/documents/${doc.id}/download`,
    doc.filename,
  );

export const setRecApproval = (prId: number, type: RecApprovalType, approved: boolean) =>
  apiFetch<PurchaseRequest>(`/api/v1/purchase-requests/${prId}/recommendation/approvals/${type}`, {
    method: "POST",
    body: { approved },
  });

export const addRecComment = (prId: number, approvalType: RecApprovalType, comment: string) =>
  apiFetch<RecComment>(`/api/v1/purchase-requests/${prId}/recommendation/comments`, {
    method: "POST",
    body: { approval_type: approvalType, comment },
  });

export const uploadRecCommentDocument = (prId: number, commentId: number, file: File) => {
  const fd = new FormData();
  fd.append("file", file);
  return apiFetch<Document>(
    `/api/v1/purchase-requests/${prId}/recommendation/comments/${commentId}/documents`,
    { method: "POST", body: fd },
  );
};

export const downloadRecCommentDocument = (prId: number, commentId: number, doc: Document) =>
  downloadFile(
    `/api/v1/purchase-requests/${prId}/recommendation/comments/${commentId}/documents/${doc.id}/download`,
    doc.filename,
  );
