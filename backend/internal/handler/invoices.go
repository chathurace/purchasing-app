package handler

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/cs/purchasing-app/internal/middleware"
	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
	"github.com/cs/purchasing-app/internal/storage"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
)

type InvoicesHandler struct {
	Repo    *repository.Repository
	Storage storage.Store
	Log     zerolog.Logger
}

type invoiceItemInput struct {
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
}

type invoiceAllocInput struct {
	BudgetUnitID int64   `json:"budget_unit_id"`
	Value        float64 `json:"value"`
}

type invoiceInput struct {
	VendorInvoiceNo string              `json:"vendor_invoice_no"`
	InvoiceDate     string              `json:"invoice_date"`
	DueDate         *string             `json:"due_date"`
	Currency        string              `json:"currency"`
	Note            string              `json:"note"`
	AllocationMode  string              `json:"allocation_mode"`
	EnteredTotal    *float64            `json:"entered_total"`
	Items           []invoiceItemInput  `json:"items"`
	CostAllocations []invoiceAllocInput `json:"cost_allocations"`
}

func (in invoiceInput) toRepo() repository.InvoiceInput {
	items := make([]repository.InvoiceItem, 0, len(in.Items))
	for _, it := range in.Items {
		desc := strings.TrimSpace(it.Description)
		if desc == "" {
			continue
		}
		items = append(items, repository.InvoiceItem{Description: desc, Quantity: it.Quantity, UnitPrice: it.UnitPrice})
	}
	allocs := make([]repository.CostAllocation, 0, len(in.CostAllocations))
	for _, a := range in.CostAllocations {
		allocs = append(allocs, repository.CostAllocation{BudgetUnitID: a.BudgetUnitID, Value: a.Value})
	}
	mode := strings.ToLower(strings.TrimSpace(in.AllocationMode))
	if mode == "" {
		mode = model.AllocByPercentage
	}
	return repository.InvoiceInput{
		VendorInvoiceNo: strings.TrimSpace(in.VendorInvoiceNo),
		InvoiceDate:     strings.TrimSpace(in.InvoiceDate),
		DueDate:         in.DueDate,
		Currency:        strings.ToUpper(strings.TrimSpace(in.Currency)),
		Note:            in.Note,
		AllocationMode:  mode,
		EnteredTotal:    in.EnteredTotal,
		Items:           items,
		CostAllocations: allocs,
	}
}

// validateAllocations enforces the budget-unit allocation rules: a valid mode,
// at least one allocation, no duplicate or invalid budget units, and a total
// that adds up (100% in percentage mode, or the invoice total in amount mode).
// Returns a user-facing message and false when invalid.
func validateAllocations(in repository.InvoiceInput, total float64) (string, bool) {
	if !model.ValidAllocationMode(in.AllocationMode) {
		return "allocation mode must be 'percentage' or 'amount'", false
	}
	if len(in.CostAllocations) == 0 {
		return "at least one budget-unit allocation is required", false
	}
	seen := map[int64]bool{}
	var sum float64
	for _, a := range in.CostAllocations {
		if a.BudgetUnitID <= 0 {
			return "each allocation must reference a budget unit", false
		}
		if seen[a.BudgetUnitID] {
			return "each budget unit can appear only once in the allocation", false
		}
		seen[a.BudgetUnitID] = true
		if a.Value < 0 {
			return "allocation values cannot be negative", false
		}
		sum += a.Value
	}
	const eps = 0.01
	if in.AllocationMode == model.AllocByPercentage && math.Abs(sum-100) > eps {
		return fmt.Sprintf("percentage allocations must add up to 100%% (got %.2f%%)", sum), false
	}
	if in.AllocationMode == model.AllocByAmount && math.Abs(sum-total) > eps {
		return fmt.Sprintf("amount allocations must add up to the invoice total of %.2f (got %.2f)", total, sum), false
	}
	return "", true
}

// invoiceItemsTotal sums quantity * unit_price across the (already filtered)
// repo line items — the same derivation the repository persists.
func invoiceItemsTotal(items []repository.InvoiceItem) float64 {
	var total float64
	for _, it := range items {
		total += it.Quantity * it.UnitPrice
	}
	return total
}

// invoiceEffectiveTotal is the invoice value: the directly-entered total when
// supplied (it takes priority), otherwise the line-items sum.
func invoiceEffectiveTotal(in repository.InvoiceInput) float64 {
	if in.EnteredTotal != nil {
		return *in.EnteredTotal
	}
	return invoiceItemsTotal(in.Items)
}

// validateInvoice checks invoice-level rules before persisting: a non-negative
// entered total, and cost-center allocations that add up against the effective
// total. A line-items/entered-total mismatch is intentionally not an error — the
// entered total wins and the UI only warns.
func validateInvoice(in repository.InvoiceInput) (string, bool) {
	if in.EnteredTotal != nil && *in.EnteredTotal < 0 {
		return "invoice total cannot be negative", false
	}
	return validateAllocations(in, invoiceEffectiveTotal(in))
}

// Create records a vendor invoice against a signed contract.
func (h *InvoicesHandler) Create(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasProcurementAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "procurement access required")
		return
	}
	contractID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid contract id")
		return
	}
	var in invoiceInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	repoIn := in.toRepo()
	if msg, ok := validateInvoice(repoIn); !ok {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	user := middleware.UserFromCtx(r.Context())
	inv, err := h.Repo.CreateInvoice(r.Context(), contractID, repoIn, user.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "contract not found")
			return
		}
		if errors.Is(err, repository.ErrInvalidState) {
			writeError(w, http.StatusConflict, "an invoice can only be recorded against a signed contract")
			return
		}
		reqLog(r).Error().Err(err).Msg("create invoice")
		writeError(w, http.StatusInternalServerError, "failed to create invoice")
		return
	}
	recordProcessEvent(r, h.Repo, inv.PurchaseRequestID, model.ProcessCreateInvoice, "")
	writeJSON(w, http.StatusCreated, inv)
}

// ListForContract returns the invoices of one contract.
func (h *InvoicesHandler) ListForContract(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasProcurementAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "procurement access required")
		return
	}
	contractID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid contract id")
		return
	}
	invoices, err := h.Repo.ListInvoices(r.Context(), &contractID)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("list invoices for contract")
		writeError(w, http.StatusInternalServerError, "failed to list invoices")
		return
	}
	writeJSON(w, http.StatusOK, invoices)
}

func (h *InvoicesHandler) List(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasProcurementAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "procurement access required")
		return
	}
	invoices, err := h.Repo.ListInvoices(r.Context(), nil)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("list invoices")
		writeError(w, http.StatusInternalServerError, "failed to list invoices")
		return
	}
	writeJSON(w, http.StatusOK, invoices)
}

func (h *InvoicesHandler) Get(w http.ResponseWriter, r *http.Request) {
	inv, ok := h.load(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, inv)
}

// Update edits an invoice's fields/items. Only allowed while the invoice is in
// the received state — an approved or paid invoice is locked.
func (h *InvoicesHandler) Update(w http.ResponseWriter, r *http.Request) {
	inv, ok := h.load(w, r)
	if !ok {
		return
	}
	if inv.Status != model.InvoiceReceived {
		writeError(w, http.StatusConflict, "only a received (not yet approved) invoice can be edited")
		return
	}
	var in invoiceInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	repoIn := in.toRepo()
	if msg, ok := validateInvoice(repoIn); !ok {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if err := h.Repo.UpdateInvoice(r.Context(), inv.ID, repoIn); err != nil {
		reqLog(r).Error().Err(err).Msg("update invoice")
		writeError(w, http.StatusInternalServerError, "failed to update invoice")
		return
	}
	recordProcessEvent(r, h.Repo, inv.PurchaseRequestID, model.ProcessUpdateInvoice, "")
	updated, err := h.Repo.GetInvoice(r.Context(), inv.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("reload invoice after mutation")
		writeError(w, http.StatusInternalServerError, "failed to reload invoice")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// SetStatus advances or reverts an invoice's status (received/approved/paid).
func (h *InvoicesHandler) SetStatus(w http.ResponseWriter, r *http.Request) {
	inv, ok := h.load(w, r)
	if !ok {
		return
	}
	var in struct {
		Status string `json:"status"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	switch in.Status {
	case model.InvoiceReceived, model.InvoiceApproved, model.InvoicePaid:
	default:
		writeError(w, http.StatusBadRequest, "status must be one of received, approved, paid")
		return
	}
	user := middleware.UserFromCtx(r.Context())
	if err := h.Repo.SetInvoiceStatus(r.Context(), inv.ID, in.Status, user.ID); err != nil {
		if errors.Is(err, repository.ErrInvalidState) {
			writeError(w, http.StatusConflict, "that status change is not allowed from the invoice's current state")
			return
		}
		reqLog(r).Error().Err(err).Msg("set invoice status")
		writeError(w, http.StatusInternalServerError, "failed to update invoice status")
		return
	}
	recordProcessEvent(r, h.Repo, inv.PurchaseRequestID, model.ProcessInvoiceStatus, in.Status) // qualifier: received|approved|paid
	updated, err := h.Repo.GetInvoice(r.Context(), inv.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("reload invoice after mutation")
		writeError(w, http.StatusInternalServerError, "failed to reload invoice")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// Delete removes an invoice. Only allowed while in the received state.
func (h *InvoicesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	inv, ok := h.load(w, r)
	if !ok {
		return
	}
	if inv.Status != model.InvoiceReceived {
		writeError(w, http.StatusConflict, "only a received (not yet approved) invoice can be deleted")
		return
	}
	paths, err := h.Repo.DeleteInvoice(r.Context(), inv.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("delete invoice")
		writeError(w, http.StatusInternalServerError, "failed to delete invoice")
		return
	}
	for _, p := range paths {
		_ = h.Storage.Delete(p)
	}
	recordProcessEvent(r, h.Repo, inv.PurchaseRequestID, model.ProcessDeleteInvoice, "")
	w.WriteHeader(http.StatusNoContent)
}

func (h *InvoicesHandler) UploadDocument(w http.ResponseWriter, r *http.Request) {
	inv, ok := h.load(w, r)
	if !ok {
		return
	}
	doc, ok := saveUploadedDoc(w, r, h.Repo, h.Storage, inv.PurchaseRequestID, model.OwnerInvoice, inv.ID, allowedExtensions)
	if !ok {
		return
	}
	writeJSON(w, http.StatusCreated, doc)
}

func (h *InvoicesHandler) DownloadDocument(w http.ResponseWriter, r *http.Request) {
	inv, ok := h.load(w, r)
	if !ok {
		return
	}
	docID, err := parseID(r, "docID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid document id")
		return
	}
	downloadOwnedDoc(w, r, h.Repo, h.Storage, model.OwnerInvoice, inv.ID, docID)
}

func (h *InvoicesHandler) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	inv, ok := h.load(w, r)
	if !ok {
		return
	}
	docID, err := parseID(r, "docID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid document id")
		return
	}
	deleteOwnedDoc(w, r, h.Repo, h.Storage, model.OwnerInvoice, inv.ID, docID)
}

// load fetches the invoice for procurement-level actions, enforcing procurement access.
func (h *InvoicesHandler) load(w http.ResponseWriter, r *http.Request) (*repository.Invoice, bool) {
	if !middleware.HasProcurementAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "procurement access required")
		return nil, false
	}
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid invoice id")
		return nil, false
	}
	inv, err := h.Repo.GetInvoice(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "invoice not found")
			return nil, false
		}
		reqLog(r).Error().Err(err).Msg("get invoice")
		writeError(w, http.StatusInternalServerError, "failed to load invoice")
		return nil, false
	}
	return inv, true
}
