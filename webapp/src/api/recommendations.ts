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

// Adds (requires) a single approval card to the recommendation without touching
// the others. Returns the refreshed purchase request.
export const requestRecApproval = (prId: number, type: RecApprovalType) =>
  apiFetch<PurchaseRequest>(
    `/api/v1/purchase-requests/${prId}/recommendation/approvals/${type}/request`,
    { method: "POST" },
  );

// Removes a single approval card from the recommendation. Returns the refreshed
// purchase request.
export const removeRecApproval = (prId: number, type: RecApprovalType) =>
  apiFetch<PurchaseRequest>(
    `/api/v1/purchase-requests/${prId}/recommendation/approvals/${type}`,
    { method: "DELETE" },
  );

export const setRecApproval = (prId: number, type: RecApprovalType, approved: boolean) =>
  apiFetch<PurchaseRequest>(`/api/v1/purchase-requests/${prId}/recommendation/approvals/${type}`, {
    method: "POST",
    body: { approved },
  });

// Sets (or clears, with assigneeId null) the assignee of a team card
// (legal/security/compliance).
// When notify is true and an assignee is set, the assignee is emailed a link to
// the PR (CC the team email). Returns the refreshed purchase request.
export const setRecAssignee = (
  prId: number,
  type: RecApprovalType,
  assigneeId: number | null,
  notify: boolean,
) =>
  apiFetch<PurchaseRequest>(
    `/api/v1/purchase-requests/${prId}/recommendation/approvals/${type}/assignee`,
    { method: "PUT", body: { assignee_id: assigneeId, notify } },
  );

// Re-sends the assignment notification to a card's current assignee.
export const remindRecAssignee = (prId: number, type: RecApprovalType) =>
  apiFetch<void>(
    `/api/v1/purchase-requests/${prId}/recommendation/approvals/${type}/assignee/remind`,
    { method: "POST" },
  );

// Notifies every qualified budget approver of the recommendation's budget card.
export const remindBudgetApprovers = (prId: number) =>
  apiFetch<void>(
    `/api/v1/purchase-requests/${prId}/recommendation/approvals/budget/remind`,
    { method: "POST" },
  );

// budgetStepId attaches a budget-card comment to a specific step (the approval
// type must be "budget"); omit it for team-card comments.
export const addRecComment = (
  prId: number,
  approvalType: RecApprovalType,
  comment: string,
  budgetStepId?: number,
) =>
  apiFetch<RecComment>(`/api/v1/purchase-requests/${prId}/recommendation/comments`, {
    method: "POST",
    body: { approval_type: approvalType, comment, budget_step_id: budgetStepId ?? null },
  });

// --- Budget approval chain (serial steps on the budget card) ---

export const addBudgetStep = (prId: number, approverName: string, approverEmail: string) =>
  apiFetch<PurchaseRequest>(`/api/v1/purchase-requests/${prId}/recommendation/budget-steps`, {
    method: "POST",
    body: { approver_name: approverName, approver_email: approverEmail },
  });

export const updateBudgetStep = (
  prId: number,
  stepId: number,
  approverName: string,
  approverEmail: string,
) =>
  apiFetch<PurchaseRequest>(
    `/api/v1/purchase-requests/${prId}/recommendation/budget-steps/${stepId}`,
    { method: "PUT", body: { approver_name: approverName, approver_email: approverEmail } },
  );

export const deleteBudgetStep = (prId: number, stepId: number) =>
  apiFetch<PurchaseRequest>(
    `/api/v1/purchase-requests/${prId}/recommendation/budget-steps/${stepId}`,
    { method: "DELETE" },
  );

// decision is one of "approve" | "reject" | "revert".
export const setBudgetStepDecision = (
  prId: number,
  stepId: number,
  decision: "approve" | "reject" | "revert",
) =>
  apiFetch<PurchaseRequest>(
    `/api/v1/purchase-requests/${prId}/recommendation/budget-steps/${stepId}/decision`,
    { method: "POST", body: { decision } },
  );

export const remindBudgetStep = (prId: number, stepId: number) =>
  apiFetch<void>(
    `/api/v1/purchase-requests/${prId}/recommendation/budget-steps/${stepId}/remind`,
    { method: "POST" },
  );

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
