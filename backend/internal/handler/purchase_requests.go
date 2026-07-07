package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/cs/purchasing-app/internal/email"
	"github.com/cs/purchasing-app/internal/middleware"
	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
	"github.com/cs/purchasing-app/internal/storage"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
)

// maxUploadBytes caps a single document upload (25 MiB).
const maxUploadBytes = 25 << 20

// allowedExtensions are the document types staff may upload.
var allowedExtensions = map[string]bool{".pdf": true, ".docx": true}

type PurchaseRequestsHandler struct {
	Repo       *repository.Repository
	Storage    storage.Store
	Mailer     email.Mailer
	AppBaseURL string
	Log        zerolog.Logger
}

// --- request/response payloads ---

type itemInput struct {
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
}

type linkInput struct {
	URL   string `json:"url"`
	Label string `json:"label"`
}

type prInput struct {
	Title        string      `json:"title"`
	CostCenter   string      `json:"cost_center"`
	CostCenterID *int64      `json:"cost_center_id"`
	Comments     string      `json:"comments"`
	Items        []itemInput `json:"items"`
	Links        []linkInput `json:"links"`
	ApproverIDs  []int64     `json:"approver_ids"`
	// Requisition form fields.
	Team                string          `json:"team"`
	Entity              string          `json:"entity"`
	Category            string          `json:"category"`
	EstimatedValue      float64         `json:"estimated_value"`
	Currency            string          `json:"currency"`
	BudgetApproverName  string          `json:"budget_approver_name"`
	BudgetApproverEmail string          `json:"budget_approver_email"`
	Details             json.RawMessage `json:"details"`
}

func (in prInput) toRepo() repository.PurchaseRequestInput {
	out := repository.PurchaseRequestInput{
		Title:               strings.TrimSpace(in.Title),
		CostCenter:          strings.TrimSpace(in.CostCenter),
		CostCenterID:        in.CostCenterID,
		Comments:            in.Comments,
		Team:                strings.TrimSpace(in.Team),
		Entity:              strings.TrimSpace(in.Entity),
		Category:            strings.TrimSpace(in.Category),
		EstimatedValue:      in.EstimatedValue,
		Currency:            strings.TrimSpace(in.Currency),
		BudgetApproverName:  strings.TrimSpace(in.BudgetApproverName),
		BudgetApproverEmail: strings.TrimSpace(in.BudgetApproverEmail),
		Details:             in.Details,
	}
	for _, it := range in.Items {
		desc := strings.TrimSpace(it.Description)
		if desc == "" {
			continue
		}
		out.Items = append(out.Items, repository.Item{Description: desc, Quantity: it.Quantity})
	}
	for _, ln := range in.Links {
		u := strings.TrimSpace(ln.URL)
		if u == "" {
			continue
		}
		out.Links = append(out.Links, repository.Link{URL: u, Label: strings.TrimSpace(ln.Label)})
	}
	return out
}

// --- access helpers ---

// callerCanView reports whether the caller may view the request: the owner,
// anyone with finance/finance_admin/admin, a named approver, or an actor on one
// of the procurement recommendation's approval cards (legal/security/budget).
func (h *PurchaseRequestsHandler) callerCanView(r *http.Request, pr *repository.PurchaseRequest) bool {
	user := middleware.UserFromCtx(r.Context())
	if user != nil && user.ID == pr.RequesterID {
		return true
	}
	if middleware.HasFinanceAccess(r.Context()) {
		return true
	}
	if user != nil {
		for _, a := range pr.Approvals {
			if a.ApproverID == user.ID {
				return true
			}
		}
	}
	if pr.Recommendation != nil {
		for _, a := range pr.Recommendation.Approvals {
			if ok, _ := h.canActOnRecType(r, pr, a.ApprovalType); ok {
				return true
			}
		}
	}
	return false
}

// canActOnRecType reports whether the caller may act on a recommendation approval
// card: legal/security require the matching role (admin passes); the budget owner
// card requires being the PR's cost-center owner (or admin).
func (h *PurchaseRequestsHandler) canActOnRecType(r *http.Request, pr *repository.PurchaseRequest, approvalType string) (bool, error) {
	ctx := r.Context()
	switch approvalType {
	case model.RecApprovalLegal:
		return middleware.HasRole(ctx, model.RoleLegal), nil
	case model.RecApprovalSecurity:
		return middleware.HasRole(ctx, model.RoleSecurity), nil
	case model.RecApprovalBudget:
		if middleware.HasRole(ctx, model.RoleAdmin) {
			return true, nil
		}
		user := middleware.UserFromCtx(ctx)
		if user == nil {
			return false, nil
		}
		return h.Repo.IsBudgetOwnerForPR(ctx, pr.ID, user.ID)
	}
	return false, nil
}

// attachRecActionable fills the recommendation's my_actionable_types for the
// caller (which cards they may toggle/comment on), so the UI can show controls
// only where allowed. Safe to call when the PR has no recommendation.
func (h *PurchaseRequestsHandler) attachRecActionable(r *http.Request, pr *repository.PurchaseRequest) {
	if pr == nil || pr.Recommendation == nil {
		return
	}
	actionable := []string{}
	for _, a := range pr.Recommendation.Approvals {
		if ok, _ := h.canActOnRecType(r, pr, a.ApprovalType); ok {
			actionable = append(actionable, a.ApprovalType)
		}
	}
	pr.Recommendation.MyActionableTypes = actionable
}

// callerCanEdit reports whether the caller may modify the request: the owner
// while it is in an editable state, or an admin (forward-compat).
func (h *PurchaseRequestsHandler) callerCanEdit(r *http.Request, pr *repository.PurchaseRequest) bool {
	if middleware.HasRole(r.Context(), model.RoleAdmin) {
		return true
	}
	user := middleware.UserFromCtx(r.Context())
	return user != nil && user.ID == pr.RequesterID && model.IsEditable(pr.Status)
}

// --- handlers ---

// List returns all requests for finance*/admin, or — for other users — the
// requests they own, have been asked to approve, or carry a procurement
// recommendation card they can act on (legal/security/budget owner).
func (h *PurchaseRequestsHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	seesAll := middleware.HasFinanceAccess(ctx)
	user := middleware.UserFromCtx(ctx)

	// ?scope=mine  -> only PRs the caller submitted (Requests tab for staff/approvers)
	// ?scope=approvals -> only PRs awaiting the caller's decision (Approvals tab)
	// (absent) -> legacy behavior: seesAll for finance, union otherwise.
	var scope repository.PRListScope
	switch r.URL.Query().Get("scope") {
	case "mine":
		scope = repository.PRScopeMine
	case "approvals":
		scope = repository.PRScopeApprovals
	}

	prs, err := h.Repo.ListPurchaseRequests(ctx, user.ID, seesAll,
		middleware.HasRole(ctx, model.RoleLegal), middleware.HasRole(ctx, model.RoleSecurity), scope)
	if err != nil {
		h.Log.Error().Err(err).Msg("list purchase requests")
		writeError(w, http.StatusInternalServerError, "failed to list requests")
		return
	}
	if prs == nil {
		prs = []*repository.PurchaseRequest{}
	}
	writeJSON(w, http.StatusOK, prs)
}

func (h *PurchaseRequestsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var in prInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	user := middleware.UserFromCtx(r.Context())

	repoIn := in.toRepo()
	// Approvers are optional now — procurement sign-off is handled by the
	// recommendation, not a PR-level approver gate. Validate only any provided.
	if len(in.ApproverIDs) > 0 {
		approverIDs, errMsg := h.validateApprovers(r.Context(), user.ID, in.ApproverIDs)
		if errMsg != "" {
			writeError(w, http.StatusBadRequest, errMsg)
			return
		}
		repoIn.ApproverIDs = approverIDs
	}
	pr, err := h.Repo.CreatePurchaseRequest(r.Context(), user.ID, repoIn)
	if err != nil {
		h.Log.Error().Err(err).Msg("create purchase request")
		writeError(w, http.StatusInternalServerError, "failed to create request")
		return
	}
	if len(pr.Approvals) > 0 {
		h.notifyApprovalRequested(pr, pr.Approvals, user)
	}
	writeJSON(w, http.StatusCreated, pr)
}

// validateApprovers normalizes the requested approver IDs: deduped, with the
// requester removed, and every id confirmed to be an active user. It returns the
// cleaned list, or a non-empty user-facing error message on failure.
func (h *PurchaseRequestsHandler) validateApprovers(ctx context.Context, requesterID int64, ids []int64) ([]int64, string) {
	active, err := h.Repo.ListActiveUsers(ctx)
	if err != nil {
		h.Log.Error().Err(err).Msg("validate approvers: list users")
		return nil, "failed to validate approvers"
	}
	activeSet := make(map[int64]bool, len(active))
	for _, u := range active {
		activeSet[u.ID] = true
	}
	seen := map[int64]bool{}
	out := []int64{}
	for _, id := range ids {
		if id == requesterID {
			return nil, "you cannot list yourself as an approver"
		}
		if seen[id] {
			continue
		}
		if !activeSet[id] {
			return nil, "one or more selected approvers are not valid active users"
		}
		seen[id] = true
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil, "select at least one approver"
	}
	return out, ""
}

func (h *PurchaseRequestsHandler) Get(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	if !h.callerCanView(r, pr) {
		writeError(w, http.StatusForbidden, "not allowed to view this request")
		return
	}
	h.attachRecActionable(r, pr)
	writeJSON(w, http.StatusOK, pr)
}

// relatedDocuments bundles the procurement records of a single case (one PR and
// its quotations and contracts) so a detail page can show all the records
// associated with whatever entity is being viewed.
type relatedDocuments struct {
	PurchaseRequest prSummary               `json:"purchase_request"`
	Quotations      []*repository.Quotation `json:"quotations"`
	Contracts       []*repository.Contract  `json:"contracts"`
	GRNs            []*repository.GRN       `json:"grns"`
	Invoices        []*repository.Invoice   `json:"invoices"`
}

type prSummary struct {
	ID        int64   `json:"id"`
	Reference *string `json:"reference"`
	Title     string  `json:"title"`
	Status    string  `json:"status"`
}

// Related returns every procurement record linked to the case anchored on this
// purchase request. Gated to finance access so it never widens the read scope of
// the per-entity list endpoints it draws from.
func (h *PurchaseRequestsHandler) Related(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasFinanceAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "finance access required")
		return
	}
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	quotations, err := h.Repo.ListQuotationsForPR(r.Context(), pr.ID)
	if err != nil {
		h.Log.Error().Err(err).Msg("related: list quotations")
		writeError(w, http.StatusInternalServerError, "failed to load related documents")
		return
	}
	contracts, err := h.Repo.ListContracts(r.Context(), &pr.ID)
	if err != nil {
		h.Log.Error().Err(err).Msg("related: list contracts")
		writeError(w, http.StatusInternalServerError, "failed to load related documents")
		return
	}
	grns, err := h.Repo.ListGRNsForPR(r.Context(), pr.ID)
	if err != nil {
		h.Log.Error().Err(err).Msg("related: list grns")
		writeError(w, http.StatusInternalServerError, "failed to load related documents")
		return
	}
	invoices, err := h.Repo.ListInvoicesForPR(r.Context(), pr.ID)
	if err != nil {
		h.Log.Error().Err(err).Msg("related: list invoices")
		writeError(w, http.StatusInternalServerError, "failed to load related documents")
		return
	}
	writeJSON(w, http.StatusOK, relatedDocuments{
		PurchaseRequest: prSummary{ID: pr.ID, Reference: pr.Reference, Title: pr.Title, Status: pr.Status},
		Quotations:      quotations,
		Contracts:       contracts,
		GRNs:            grns,
		Invoices:        invoices,
	})
}

func (h *PurchaseRequestsHandler) Update(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	if !h.callerCanEdit(r, pr) {
		writeError(w, http.StatusForbidden, "this request can no longer be edited")
		return
	}
	var in prInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.Repo.UpdatePurchaseRequest(r.Context(), pr.ID, in.toRepo()); err != nil {
		h.Log.Error().Err(err).Msg("update purchase request")
		writeError(w, http.StatusInternalServerError, "failed to update request")
		return
	}
	updated, err := h.Repo.GetPurchaseRequest(r.Context(), pr.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reload request")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// Reject marks a purchase request as rejected with a required comment and
// cancels its open quotations/unsigned contracts. Finance access only.
func (h *PurchaseRequestsHandler) Reject(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasFinanceAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "finance access required")
		return
	}
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	if pr.Status == model.StatusRejected || pr.Status == model.StatusCancelled ||
		pr.Status == model.StatusOrderSigned || pr.Status == model.StatusCompleted {
		writeError(w, http.StatusConflict, "request can no longer be rejected")
		return
	}
	var in struct {
		Comment string `json:"comment"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(in.Comment) == "" {
		writeError(w, http.StatusBadRequest, "a rejection comment is required")
		return
	}
	user := middleware.UserFromCtx(r.Context())
	if err := h.Repo.RejectPurchaseRequest(r.Context(), pr.ID, strings.TrimSpace(in.Comment), user.ID); err != nil {
		h.Log.Error().Err(err).Msg("reject purchase request")
		writeError(w, http.StatusInternalServerError, "failed to reject request")
		return
	}
	updated, err := h.Repo.GetPurchaseRequest(r.Context(), pr.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reload request")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// --- approvals ---

// callerCanManageApprovers reports whether the caller may add/remove approvers or
// re-request a rejected approval: the owner while the request is editable, or an
// admin. Mirrors callerCanEdit.
func (h *PurchaseRequestsHandler) callerCanManageApprovers(r *http.Request, pr *repository.PurchaseRequest) bool {
	if middleware.HasRole(r.Context(), model.RoleAdmin) {
		return true
	}
	user := middleware.UserFromCtx(r.Context())
	return user != nil && user.ID == pr.RequesterID && model.IsEditable(pr.Status)
}

// AddApprover adds an approver to the request (owner, while editable).
func (h *PurchaseRequestsHandler) AddApprover(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	if !h.callerCanManageApprovers(r, pr) {
		writeError(w, http.StatusForbidden, "approvers can only be changed while the request is editable")
		return
	}
	var in struct {
		ApproverID int64 `json:"approver_id"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ids, errMsg := h.validateApprovers(r.Context(), pr.RequesterID, []int64{in.ApproverID})
	if errMsg != "" {
		writeError(w, http.StatusBadRequest, errMsg)
		return
	}
	approval, err := h.Repo.AddApprover(r.Context(), pr.ID, ids[0])
	if err != nil {
		h.Log.Error().Err(err).Msg("add approver")
		writeError(w, http.StatusInternalServerError, "failed to add approver")
		return
	}
	requester := middleware.UserFromCtx(r.Context())
	if approval.Status == model.PRApprovalPending {
		h.notifyApprovalRequested(pr, []repository.PRApproval{*approval}, requester)
	}
	h.reloadPR(w, r, pr.ID)
}

// RemoveApprover removes an approver from the request (owner, while editable).
// The last remaining approver cannot be removed.
func (h *PurchaseRequestsHandler) RemoveApprover(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	if !h.callerCanManageApprovers(r, pr) {
		writeError(w, http.StatusForbidden, "approvers can only be changed while the request is editable")
		return
	}
	approverID, err := parseID(r, "approverID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid approver id")
		return
	}
	if err := h.Repo.RemoveApprover(r.Context(), pr.ID, approverID); err != nil {
		if errors.Is(err, repository.ErrInvalidState) {
			writeError(w, http.StatusConflict, "a request must keep at least one approver")
			return
		}
		h.Log.Error().Err(err).Msg("remove approver")
		writeError(w, http.StatusInternalServerError, "failed to remove approver")
		return
	}
	h.reloadPR(w, r, pr.ID)
}

// RequestApprovalAgain resets a rejected approval to pending so the approver can
// reconsider (owner, while editable).
func (h *PurchaseRequestsHandler) RequestApprovalAgain(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	if !h.callerCanManageApprovers(r, pr) {
		writeError(w, http.StatusForbidden, "approvals can only be re-requested while the request is editable")
		return
	}
	approverID, err := parseID(r, "approverID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid approver id")
		return
	}
	if err := h.Repo.RequestApprovalAgain(r.Context(), pr.ID, approverID); err != nil {
		if errors.Is(err, repository.ErrInvalidState) {
			writeError(w, http.StatusConflict, "only a rejected approval can be re-requested")
			return
		}
		h.Log.Error().Err(err).Msg("re-request approval")
		writeError(w, http.StatusInternalServerError, "failed to re-request approval")
		return
	}
	requester := middleware.UserFromCtx(r.Context())
	if approval, err := h.Repo.GetApproval(r.Context(), pr.ID, approverID); err == nil {
		h.notifyApprovalRequested(pr, []repository.PRApproval{*approval}, requester)
	}
	h.reloadPR(w, r, pr.ID)
}

// RecordApprovalDecision records the calling approver's own decision (approve or
// reject) with a comment. Only a named approver on the request may act, and only
// on their own row.
func (h *PurchaseRequestsHandler) RecordApprovalDecision(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	user := middleware.UserFromCtx(r.Context())
	isApprover, err := h.Repo.IsApprover(r.Context(), pr.ID, user.ID)
	if err != nil {
		h.Log.Error().Err(err).Msg("check approver")
		writeError(w, http.StatusInternalServerError, "failed to record decision")
		return
	}
	if !isApprover {
		writeError(w, http.StatusForbidden, "only a named approver can act on this request")
		return
	}
	if pr.Status == model.StatusRejected || pr.Status == model.StatusCancelled {
		writeError(w, http.StatusConflict, "this request is closed")
		return
	}
	var in struct {
		Decision string `json:"decision"` // approve | reject
		Comment  string `json:"comment"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var status string
	switch in.Decision {
	case "approve":
		status = model.PRApprovalApproved
	case "reject":
		status = model.PRApprovalRejected
	default:
		writeError(w, http.StatusBadRequest, "decision must be approve or reject")
		return
	}
	comment := strings.TrimSpace(in.Comment)
	if status == model.PRApprovalRejected && comment == "" {
		writeError(w, http.StatusBadRequest, "a comment is required when rejecting")
		return
	}
	if err := h.Repo.RecordApprovalDecision(r.Context(), pr.ID, user.ID, status, comment); err != nil {
		if errors.Is(err, repository.ErrInvalidState) {
			writeError(w, http.StatusForbidden, "only a named approver can act on this request")
			return
		}
		h.Log.Error().Err(err).Msg("record approval decision")
		writeError(w, http.StatusInternalServerError, "failed to record decision")
		return
	}
	h.notifyDecision(pr, user, status, comment)
	h.reloadPR(w, r, pr.ID)
}

// reloadPR re-reads the request and writes it as the response (used by the
// approval mutations so the client gets the fresh approval state).
func (h *PurchaseRequestsHandler) reloadPR(w http.ResponseWriter, r *http.Request, id int64) {
	pr, err := h.Repo.GetPurchaseRequest(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reload request")
		return
	}
	h.attachRecActionable(r, pr)
	writeJSON(w, http.StatusOK, pr)
}

// --- approval email notifications (best-effort) ---

// prDisplayRef returns a PR's human-readable reference, falling back to the
// system id (#42) when none has been assigned. The reference is a display label
// whose format may change over time, kept separate from the immutable id.
func prDisplayRef(pr *repository.PurchaseRequest) string {
	if pr.Reference != nil && *pr.Reference != "" {
		return *pr.Reference
	}
	return fmt.Sprintf("#%d", pr.ID)
}

func (h *PurchaseRequestsHandler) prURL(pr *repository.PurchaseRequest) string {
	ref := prDisplayRef(pr)
	if h.AppBaseURL == "" {
		return ref
	}
	return fmt.Sprintf("%s (%s/requests/%d)", ref, strings.TrimRight(h.AppBaseURL, "/"), pr.ID)
}

// sendAsync fires a single email on a background goroutine so SMTP latency never
// blocks the HTTP response. Failures are logged, never surfaced to the user.
func (h *PurchaseRequestsHandler) sendAsync(to, subject, body string) {
	if h.Mailer == nil || strings.TrimSpace(to) == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := h.Mailer.Send(ctx, to, subject, body); err != nil {
			h.Log.Warn().Err(err).Str("to", to).Msg("approval email send failed")
		}
	}()
}

// notifyApprovalRequested emails each pending approver that their sign-off is
// requested on the PR.
func (h *PurchaseRequestsHandler) notifyApprovalRequested(pr *repository.PurchaseRequest, approvals []repository.PRApproval, requester *repository.User) {
	title := pr.Title
	if title == "" {
		title = prDisplayRef(pr)
	}
	who := requester.Name
	if who == "" {
		who = requester.Email
	}
	subject := fmt.Sprintf("Approval requested: %s", title)
	for _, a := range approvals {
		if a.Approver == nil {
			continue
		}
		body := fmt.Sprintf(
			"Hi,\n\n%s has asked you to approve purchase request %s — \"%s\".\n\n"+
				"Please review and approve or reject it here:\n%s\n",
			who, prDisplayRef(pr), title, h.prURL(pr))
		h.sendAsync(a.Approver.Email, subject, body)
	}
}

// notifyDecision emails the requester that an approver has made a decision.
func (h *PurchaseRequestsHandler) notifyDecision(pr *repository.PurchaseRequest, approver *repository.User, status, comment string) {
	if pr.Requester == nil {
		return
	}
	title := pr.Title
	if title == "" {
		title = prDisplayRef(pr)
	}
	who := approver.Name
	if who == "" {
		who = approver.Email
	}
	verb := "approved"
	if status == model.PRApprovalRejected {
		verb = "rejected"
	}
	subject := fmt.Sprintf("Approval %s: %s", verb, title)
	body := fmt.Sprintf("Hi,\n\n%s has %s your purchase request %s — \"%s\".\n",
		who, verb, prDisplayRef(pr), title)
	if comment != "" {
		body += fmt.Sprintf("\nComment:\n%s\n", comment)
	}
	body += fmt.Sprintf("\nView it here:\n%s\n", h.prURL(pr))
	h.sendAsync(pr.Requester.Email, subject, body)
}

func (h *PurchaseRequestsHandler) UploadDocument(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	if !h.callerCanEdit(r, pr) {
		writeError(w, http.StatusForbidden, "this request can no longer be edited")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes+1024)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid or too-large upload")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowedExtensions[ext] {
		writeError(w, http.StatusBadRequest, "only .pdf and .docx files are allowed")
		return
	}
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	storedPath, size, err := h.Storage.Save(pr.ID, header.Filename, file)
	if err != nil {
		h.Log.Error().Err(err).Msg("save document")
		writeError(w, http.StatusInternalServerError, "failed to store file")
		return
	}
	user := middleware.UserFromCtx(r.Context())
	doc, err := h.Repo.AddDocument(r.Context(), pr.ID, header.Filename, storedPath, contentType, size, user.ID)
	if err != nil {
		_ = h.Storage.Delete(storedPath)
		h.Log.Error().Err(err).Msg("record document")
		writeError(w, http.StatusInternalServerError, "failed to record file")
		return
	}
	writeJSON(w, http.StatusCreated, doc)
}

func (h *PurchaseRequestsHandler) DownloadDocument(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	if !h.callerCanView(r, pr) {
		writeError(w, http.StatusForbidden, "not allowed to view this request")
		return
	}
	docID, err := parseID(r, "docID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid document id")
		return
	}
	doc, err := h.Repo.GetDocument(r.Context(), pr.ID, docID)
	if err != nil {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	f, err := h.Storage.Open(doc.StoredPath)
	if err != nil {
		h.Log.Error().Err(err).Str("path", doc.StoredPath).Msg("open document")
		writeError(w, http.StatusInternalServerError, "failed to open file")
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", doc.ContentType)
	w.Header().Set("Content-Disposition", "attachment; filename=\""+sanitizeHeaderFilename(doc.Filename)+"\"")
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, f); err != nil {
		h.Log.Warn().Err(err).Msg("stream document")
	}
}

func (h *PurchaseRequestsHandler) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.load(w, r)
	if !ok {
		return
	}
	if !h.callerCanEdit(r, pr) {
		writeError(w, http.StatusForbidden, "this request can no longer be edited")
		return
	}
	docID, err := parseID(r, "docID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid document id")
		return
	}
	doc, err := h.Repo.GetDocument(r.Context(), pr.ID, docID)
	if err != nil {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	if err := h.Repo.DeleteDocument(r.Context(), pr.ID, docID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete document")
		return
	}
	_ = h.Storage.Delete(doc.StoredPath)
	w.WriteHeader(http.StatusNoContent)
}

// load fetches the request by URL id, writing the appropriate error response if
// not found. The boolean is false when the caller should stop.
func (h *PurchaseRequestsHandler) load(w http.ResponseWriter, r *http.Request) (*repository.PurchaseRequest, bool) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request id")
		return nil, false
	}
	pr, err := h.Repo.GetPurchaseRequest(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "request not found")
			return nil, false
		}
		h.Log.Error().Err(err).Msg("get purchase request")
		writeError(w, http.StatusInternalServerError, "failed to load request")
		return nil, false
	}
	return pr, true
}

// sanitizeHeaderFilename strips characters that would break the Content-Disposition header.
func sanitizeHeaderFilename(name string) string {
	name = strings.ReplaceAll(name, "\"", "")
	name = strings.ReplaceAll(name, "\\", "")
	name = strings.ReplaceAll(name, "\r", "")
	name = strings.ReplaceAll(name, "\n", "")
	return name
}
