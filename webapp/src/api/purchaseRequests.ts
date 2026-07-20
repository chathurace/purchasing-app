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
// + contracts) anchored on the given PR. Procurement-access only on the backend.
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

// --- team lead approval ---

export const recordTeamLeadDecision = (id: number, decision: ApprovalDecision, notes: string) =>
  apiFetch<PurchaseRequest>(`/api/v1/purchase-requests/${id}/team-lead-approval`, {
    method: "POST",
    body: { decision, notes },
  });

// Reopens a decided team-lead approval — moves it back to pending (edit decision).
export const moveTeamLeadToPending = (id: number) =>
  apiFetch<PurchaseRequest>(`/api/v1/purchase-requests/${id}/team-lead-approval`, {
    method: "POST",
    body: { decision: "pending", notes: "" },
  });

// Re-sends the pending-approval notification to the PR's team lead.
export const remindTeamLead = (id: number) =>
  apiFetch<void>(`/api/v1/purchase-requests/${id}/team-lead-reminder`, { method: "POST" });

// Changes the PR's named team lead (email) while approval is still pending.
export const updateTeamLeadEmail = (id: number, teamLeadEmail: string) =>
  apiFetch<PurchaseRequest>(`/api/v1/purchase-requests/${id}/team-lead-email`, {
    method: "PUT",
    body: { team_lead_email: teamLeadEmail },
  });

// --- PR assignment ---

// Assigns (assigneeId) or unassigns (null) the PR's procurement owner. Procurement
// users self-assign / claim; procurement_admin assigns or reassigns anyone.
export const setPRAssignee = (id: number, assigneeId: number | null) =>
  apiFetch<PurchaseRequest>(`/api/v1/purchase-requests/${id}/assignee`, {
    method: "PUT",
    body: { assignee_id: assigneeId },
  });

export const addPRCollaborator = (id: number, userId: number) =>
  apiFetch<PurchaseRequest>(`/api/v1/purchase-requests/${id}/collaborators`, {
    method: "POST",
    body: { user_id: userId },
  });

export const removePRCollaborator = (id: number, userId: number) =>
  apiFetch<PurchaseRequest>(`/api/v1/purchase-requests/${id}/collaborators/${userId}`, {
    method: "DELETE",
  });

// setBudgetApprover updates only the PR's named budget approver (procurement; used
// from the recommendation's budget card to reconcile it with the designated one).
export const setBudgetApprover = (id: number, name: string, email: string) =>
  apiFetch<PurchaseRequest>(`/api/v1/purchase-requests/${id}/budget-approver`, {
    method: "PUT",
    body: { name, email },
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
