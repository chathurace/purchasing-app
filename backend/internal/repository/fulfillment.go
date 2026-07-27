package repository

import (
	"context"
	"time"

	"github.com/cs/purchasing-app/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Document owner types for the fulfillment entities, mirrored from the model
// package (see procurement.go for the procurement-side equivalents).
const (
	ownerGRN     = model.OwnerGRN
	ownerInvoice = model.OwnerInvoice
)

// resolveContractForFulfillment loads the bits a GRN/invoice denormalizes from
// its contract and enforces that the contract is signed. Returns ErrInvalidState
// when the contract is not yet signed.
func resolveContractForFulfillment(ctx context.Context, tx pgx.Tx, contractID int64) (prID, vendorID int64, currency string, err error) {
	var status string
	err = tx.QueryRow(ctx, `
		SELECT purchase_request_id, vendor_id, currency, status
		FROM contracts WHERE id = $1`, contractID).Scan(&prID, &vendorID, &currency, &status)
	if err != nil {
		return 0, 0, "", err
	}
	if status != model.ContractSigned {
		return 0, 0, "", ErrInvalidState
	}
	return prID, vendorID, currency, nil
}

// contractInvoicedTotal sums the invoice totals for a contract whose currency
// matches the contract's. Amounts in other currencies are excluded rather than
// summed blindly (see formatMoney's contract: never add across currencies).
func (r *Repository) contractInvoicedTotal(ctx context.Context, contractID int64, currency string) (float64, error) {
	var total pgtype.Numeric
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(total_amount), 0) FROM invoices
		WHERE contract_id = $1 AND currency = $2`, contractID, currency).Scan(&total)
	if err != nil {
		return 0, err
	}
	f, err := total.Float64Value()
	if err != nil {
		return 0, err
	}
	return f.Float64, nil
}

// =====================================================================
// Goods Received Notes (GRNs)
// =====================================================================

type GRNItem struct {
	ID          int64   `json:"id"`
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
	Position    int     `json:"position"`
}

type GRN struct {
	ID                int64      `json:"id"`
	ContractID        int64      `json:"contract_id"`
	PurchaseRequestID int64      `json:"purchase_request_id"`
	VendorID          int64      `json:"vendor_id"`
	ReceivedDate      string     `json:"received_date"` // YYYY-MM-DD
	ReceivedBy        string     `json:"received_by"`
	Note              string     `json:"note"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	Items             []GRNItem  `json:"items"`
	Vendor            *Vendor    `json:"vendor,omitempty"`
	Documents         []Document `json:"documents,omitempty"`
}

type GRNInput struct {
	ReceivedDate string // YYYY-MM-DD; defaulted to today by the handler when empty
	ReceivedBy   string
	Note         string
	Items        []GRNItem
}

// CreateGRN records a goods-received note against a signed contract. Returns
// ErrInvalidState when the contract is not signed.
func (r *Repository) CreateGRN(ctx context.Context, contractID int64, in GRNInput, createdBy int64) (*GRN, error) {
	receivedDate, err := parseDate(&in.ReceivedDate)
	if err != nil {
		return nil, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	prID, vendorID, _, err := resolveContractForFulfillment(ctx, tx, contractID)
	if err != nil {
		return nil, err
	}
	var id int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO grns (contract_id, purchase_request_id, vendor_id, received_date, received_by, note, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		contractID, prID, vendorID, receivedDate, in.ReceivedBy, in.Note, createdBy).Scan(&id); err != nil {
		return nil, err
	}
	if err := replaceGRNItems(ctx, tx, id, in.Items); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.GetGRN(ctx, id)
}

func (r *Repository) UpdateGRN(ctx context.Context, id int64, in GRNInput) error {
	receivedDate, err := parseDate(&in.ReceivedDate)
	if err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE grns SET received_date = $2, received_by = $3, note = $4, updated_at = NOW()
		WHERE id = $1`, id, receivedDate, in.ReceivedBy, in.Note); err != nil {
		return err
	}
	if err := replaceGRNItems(ctx, tx, id, in.Items); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func replaceGRNItems(ctx context.Context, tx pgx.Tx, grnID int64, items []GRNItem) error {
	if _, err := tx.Exec(ctx, `DELETE FROM grn_items WHERE grn_id = $1`, grnID); err != nil {
		return err
	}
	for i, it := range items {
		if _, err := tx.Exec(ctx, `
			INSERT INTO grn_items (grn_id, description, quantity, position)
			VALUES ($1, $2, $3, $4)`, grnID, it.Description, it.Quantity, i); err != nil {
			return err
		}
	}
	return nil
}

// ListGRNs returns GRN summaries (with vendor name). If contractID is non-nil,
// results are scoped to that contract.
func (r *Repository) ListGRNs(ctx context.Context, contractID *int64) ([]*GRN, error) {
	query := `
		SELECT g.id, g.contract_id, g.purchase_request_id, g.vendor_id, g.received_date,
		       g.received_by, g.note, g.created_at, g.updated_at, v.name
		FROM grns g
		JOIN vendors v ON v.id = g.vendor_id`
	args := []any{}
	if contractID != nil {
		query += ` WHERE g.contract_id = $1`
		args = append(args, *contractID)
	}
	query += ` ORDER BY g.received_date DESC, g.id DESC`

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*GRN{}
	for rows.Next() {
		g := &GRN{}
		var receivedDate pgtype.Date
		var vendorName string
		if err := rows.Scan(&g.ID, &g.ContractID, &g.PurchaseRequestID, &g.VendorID, &receivedDate,
			&g.ReceivedBy, &g.Note, &g.CreatedAt, &g.UpdatedAt, &vendorName); err != nil {
			return nil, err
		}
		if d := formatDate(receivedDate); d != nil {
			g.ReceivedDate = *d
		}
		g.Vendor = &Vendor{ID: g.VendorID, Name: vendorName}
		out = append(out, g)
	}
	return out, rows.Err()
}

// ListGRNsForPR returns GRN summaries (with vendor name) for every contract in a
// purchase request's case, using the denormalized purchase_request_id.
func (r *Repository) ListGRNsForPR(ctx context.Context, prID int64) ([]*GRN, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT g.id, g.contract_id, g.purchase_request_id, g.vendor_id, g.received_date,
		       g.received_by, g.note, g.created_at, g.updated_at, v.name
		FROM grns g
		JOIN vendors v ON v.id = g.vendor_id
		WHERE g.purchase_request_id = $1
		ORDER BY g.received_date DESC, g.id DESC`, prID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*GRN{}
	for rows.Next() {
		g := &GRN{}
		var receivedDate pgtype.Date
		var vendorName string
		if err := rows.Scan(&g.ID, &g.ContractID, &g.PurchaseRequestID, &g.VendorID, &receivedDate,
			&g.ReceivedBy, &g.Note, &g.CreatedAt, &g.UpdatedAt, &vendorName); err != nil {
			return nil, err
		}
		if d := formatDate(receivedDate); d != nil {
			g.ReceivedDate = *d
		}
		g.Vendor = &Vendor{ID: g.VendorID, Name: vendorName}
		out = append(out, g)
	}
	return out, rows.Err()
}

// GetGRN loads a GRN with its items, vendor and documents.
func (r *Repository) GetGRN(ctx context.Context, id int64) (*GRN, error) {
	g := &GRN{}
	var receivedDate pgtype.Date
	err := r.pool.QueryRow(ctx, `
		SELECT id, contract_id, purchase_request_id, vendor_id, received_date, received_by, note, created_at, updated_at
		FROM grns WHERE id = $1`, id).
		Scan(&g.ID, &g.ContractID, &g.PurchaseRequestID, &g.VendorID, &receivedDate, &g.ReceivedBy, &g.Note, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if d := formatDate(receivedDate); d != nil {
		g.ReceivedDate = *d
	}
	if g.Vendor, err = r.GetVendor(ctx, g.VendorID); err != nil {
		return nil, err
	}
	if g.Items, err = r.listGRNItems(ctx, id); err != nil {
		return nil, err
	}
	if g.Documents, err = r.ListOwnedDocuments(ctx, ownerGRN, id); err != nil {
		return nil, err
	}
	return g, nil
}

func (r *Repository) listGRNItems(ctx context.Context, grnID int64) ([]GRNItem, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, description, quantity, position FROM grn_items
		WHERE grn_id = $1 ORDER BY position, id`, grnID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []GRNItem{}
	for rows.Next() {
		var it GRNItem
		if err := rows.Scan(&it.ID, &it.Description, &it.Quantity, &it.Position); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// DeleteGRN removes a GRN, returning the stored paths of its documents so the
// handler can unlink the files after commit.
func (r *Repository) DeleteGRN(ctx context.Context, id int64) ([]string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	paths, err := deleteOwnedDocsTx(ctx, tx, ownerGRN, id)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM grns WHERE id = $1`, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return paths, nil
}

// =====================================================================
// Invoices
// =====================================================================

type InvoiceItem struct {
	ID          int64   `json:"id"`
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
	Position    int     `json:"position"`
}

// CostAllocation splits an invoice's cost to a business unit. Value is a
// percentage (0-100) or an absolute amount, per the invoice's AllocationMode.
// Amount is the resolved amount in the invoice currency, computed on read.
type CostAllocation struct {
	ID             int64                `json:"id"`
	BusinessUnitID int64                `json:"business_unit_id"`
	Value          float64              `json:"value"`
	Position       int                  `json:"position"`
	Amount         float64              `json:"amount"`
	BusinessUnit   *BusinessUnitSummary `json:"business_unit,omitempty"`
}

type Invoice struct {
	ID                int64            `json:"id"`
	ContractID        int64            `json:"contract_id"`
	PurchaseRequestID int64            `json:"purchase_request_id"`
	VendorID          int64            `json:"vendor_id"`
	VendorInvoiceNo   string           `json:"vendor_invoice_no"`
	InvoiceDate       string           `json:"invoice_date"`  // YYYY-MM-DD
	DueDate           *string          `json:"due_date"`      // YYYY-MM-DD or null
	TotalAmount       float64          `json:"total_amount"`  // effective total: entered_total if set, else line-items sum
	EnteredTotal      *float64         `json:"entered_total"` // directly-entered total, or null to derive from items
	Currency          string           `json:"currency"`
	Status            string           `json:"status"`
	Note              string           `json:"note"`
	PaidDate          *string          `json:"paid_date"` // YYYY-MM-DD or null
	ApprovedAt        *time.Time       `json:"approved_at"`
	CreatedAt         time.Time        `json:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at"`
	AllocationMode    string           `json:"allocation_mode"` // percentage | amount
	Items             []InvoiceItem    `json:"items"`
	CostAllocations   []CostAllocation `json:"cost_allocations"`
	Vendor            *Vendor          `json:"vendor,omitempty"`
	Approver          *UserSummary     `json:"approver,omitempty"`
	Documents         []Document       `json:"documents,omitempty"`
}

type InvoiceInput struct {
	VendorInvoiceNo string
	InvoiceDate     string // YYYY-MM-DD; defaulted to today by the handler when empty
	DueDate         *string
	Currency        string
	Note            string
	AllocationMode  string   // percentage | amount
	EnteredTotal    *float64 // directly-entered total; nil derives the total from line items
	Items           []InvoiceItem
	CostAllocations []CostAllocation
}

// itemsTotal sums quantity * unit_price across the invoice's line items.
func itemsTotal(items []InvoiceItem) float64 {
	var total float64
	for _, it := range items {
		total += it.Quantity * it.UnitPrice
	}
	return total
}

// effectiveTotal is the invoice's stored value: the directly-entered total when
// supplied (it takes priority), otherwise the line-items sum.
func effectiveTotal(in InvoiceInput) float64 {
	if in.EnteredTotal != nil {
		return *in.EnteredTotal
	}
	return itemsTotal(in.Items)
}

// CreateInvoice records a vendor invoice against a signed contract. The currency
// defaults to the contract's when not supplied. Returns ErrInvalidState when the
// contract is not signed.
func (r *Repository) CreateInvoice(ctx context.Context, contractID int64, in InvoiceInput, createdBy int64) (*Invoice, error) {
	invoiceDate, err := parseDate(&in.InvoiceDate)
	if err != nil {
		return nil, err
	}
	dueDate, err := parseDate(in.DueDate)
	if err != nil {
		return nil, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	prID, vendorID, contractCurrency, err := resolveContractForFulfillment(ctx, tx, contractID)
	if err != nil {
		return nil, err
	}
	currency := in.Currency
	if currency == "" {
		currency = contractCurrency
	}
	var id int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO invoices (contract_id, purchase_request_id, vendor_id, vendor_invoice_no,
		                      invoice_date, due_date, total_amount, entered_total, currency, note, allocation_mode, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING id`,
		contractID, prID, vendorID, in.VendorInvoiceNo, invoiceDate, dueDate,
		effectiveTotal(in), in.EnteredTotal, currency, in.Note, in.AllocationMode, createdBy).Scan(&id); err != nil {
		return nil, err
	}
	if err := replaceInvoiceItems(ctx, tx, id, in.Items); err != nil {
		return nil, err
	}
	if err := replaceInvoiceAllocations(ctx, tx, id, in.CostAllocations); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.GetInvoice(ctx, id)
}

// UpdateInvoice updates an invoice's fields and replaces its line items,
// recomputing the stored total (the entered total when given, else the
// line-items sum). The currency is left unchanged.
func (r *Repository) UpdateInvoice(ctx context.Context, id int64, in InvoiceInput) error {
	invoiceDate, err := parseDate(&in.InvoiceDate)
	if err != nil {
		return err
	}
	dueDate, err := parseDate(in.DueDate)
	if err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE invoices
		SET vendor_invoice_no = $2, invoice_date = $3, due_date = $4, total_amount = $5, entered_total = $6, note = $7,
			allocation_mode = $8, updated_at = NOW()
		WHERE id = $1`, id, in.VendorInvoiceNo, invoiceDate, dueDate, effectiveTotal(in), in.EnteredTotal, in.Note, in.AllocationMode); err != nil {
		return err
	}
	if err := replaceInvoiceItems(ctx, tx, id, in.Items); err != nil {
		return err
	}
	if err := replaceInvoiceAllocations(ctx, tx, id, in.CostAllocations); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func replaceInvoiceItems(ctx context.Context, tx pgx.Tx, invoiceID int64, items []InvoiceItem) error {
	if _, err := tx.Exec(ctx, `DELETE FROM invoice_items WHERE invoice_id = $1`, invoiceID); err != nil {
		return err
	}
	for i, it := range items {
		if _, err := tx.Exec(ctx, `
			INSERT INTO invoice_items (invoice_id, description, quantity, unit_price, position)
			VALUES ($1, $2, $3, $4, $5)`, invoiceID, it.Description, it.Quantity, it.UnitPrice, i); err != nil {
			return err
		}
	}
	return nil
}

func replaceInvoiceAllocations(ctx context.Context, tx pgx.Tx, invoiceID int64, allocs []CostAllocation) error {
	if _, err := tx.Exec(ctx, `DELETE FROM invoice_cost_allocations WHERE invoice_id = $1`, invoiceID); err != nil {
		return err
	}
	for i, a := range allocs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO invoice_cost_allocations (invoice_id, business_unit_id, value, position)
			VALUES ($1, $2, $3, $4)`, invoiceID, a.BusinessUnitID, a.Value, i); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) listInvoiceAllocations(ctx context.Context, invoiceID int64) ([]CostAllocation, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT a.id, a.business_unit_id, a.value, a.position, bu.name
		FROM invoice_cost_allocations a
		JOIN business_units bu ON bu.id = a.business_unit_id
		WHERE a.invoice_id = $1 ORDER BY a.position, a.id`, invoiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CostAllocation{}
	for rows.Next() {
		var a CostAllocation
		var name string
		if err := rows.Scan(&a.ID, &a.BusinessUnitID, &a.Value, &a.Position, &name); err != nil {
			return nil, err
		}
		a.BusinessUnit = &BusinessUnitSummary{ID: a.BusinessUnitID, Name: name}
		out = append(out, a)
	}
	return out, rows.Err()
}

// resolveAllocationAmounts fills each allocation's resolved Amount in the
// invoice currency from the invoice's mode and total.
func resolveAllocationAmounts(mode string, total float64, allocs []CostAllocation) {
	for i := range allocs {
		if mode == model.AllocByAmount {
			allocs[i].Amount = allocs[i].Value
		} else {
			allocs[i].Amount = total * allocs[i].Value / 100
		}
	}
}

// SetInvoiceStatus moves an invoice to a new status, enforcing the allowed
// transitions. Approving stamps approved_by/approved_at; reverting from approved
// clears them. Marking paid stamps paid_date; reverting from paid clears it.
// Returns ErrInvalidState for a disallowed transition.
func (r *Repository) SetInvoiceStatus(ctx context.Context, id int64, to string, actorID int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var from string
	if err := tx.QueryRow(ctx, `SELECT status FROM invoices WHERE id = $1`, id).Scan(&from); err != nil {
		return err
	}
	if from == to || !model.ValidInvoiceTransition(from, to) {
		return ErrInvalidState
	}
	switch to {
	case model.InvoiceApproved:
		// Entering approved (from received, or reverting from paid): record the
		// approver and clear any paid_date.
		if _, err := tx.Exec(ctx, `
			UPDATE invoices SET status = $2, approved_by = $3, approved_at = NOW(), paid_date = NULL, updated_at = NOW()
			WHERE id = $1`, id, to, actorID); err != nil {
			return err
		}
	case model.InvoiceReceived:
		// Reverting to received clears the approval.
		if _, err := tx.Exec(ctx, `
			UPDATE invoices SET status = $2, approved_by = NULL, approved_at = NULL, updated_at = NOW()
			WHERE id = $1`, id, to); err != nil {
			return err
		}
	case model.InvoicePaid:
		if _, err := tx.Exec(ctx, `
			UPDATE invoices SET status = $2, paid_date = CURRENT_DATE, updated_at = NOW()
			WHERE id = $1`, id, to); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ListInvoices returns invoice summaries (with vendor name). If contractID is
// non-nil, results are scoped to that contract.
func (r *Repository) ListInvoices(ctx context.Context, contractID *int64) ([]*Invoice, error) {
	query := `
		SELECT i.id, i.contract_id, i.purchase_request_id, i.vendor_id, i.vendor_invoice_no,
		       i.invoice_date, i.due_date, i.total_amount, i.entered_total, i.currency, i.status, i.note,
		       i.allocation_mode, i.paid_date, i.approved_at, i.created_at, i.updated_at, v.name
		FROM invoices i
		JOIN vendors v ON v.id = i.vendor_id`
	args := []any{}
	if contractID != nil {
		query += ` WHERE i.contract_id = $1`
		args = append(args, *contractID)
	}
	query += ` ORDER BY i.invoice_date DESC, i.id DESC`

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Invoice{}
	for rows.Next() {
		inv := &Invoice{}
		if err := scanInvoiceSummary(rows, inv); err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

// ListInvoicesForPR returns invoice summaries (with vendor name) for every
// contract in a purchase request's case, using the denormalized
// purchase_request_id.
func (r *Repository) ListInvoicesForPR(ctx context.Context, prID int64) ([]*Invoice, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT i.id, i.contract_id, i.purchase_request_id, i.vendor_id, i.vendor_invoice_no,
		       i.invoice_date, i.due_date, i.total_amount, i.entered_total, i.currency, i.status, i.note,
		       i.allocation_mode, i.paid_date, i.approved_at, i.created_at, i.updated_at, v.name
		FROM invoices i
		JOIN vendors v ON v.id = i.vendor_id
		WHERE i.purchase_request_id = $1
		ORDER BY i.invoice_date DESC, i.id DESC`, prID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Invoice{}
	for rows.Next() {
		inv := &Invoice{}
		if err := scanInvoiceSummary(rows, inv); err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

// scanInvoiceSummary scans the shared invoice-summary column list (with the
// trailing vendor name) into inv.
func scanInvoiceSummary(rows pgx.Rows, inv *Invoice) error {
	var invoiceDate pgtype.Date
	var dueDate, paidDate pgtype.Date
	var approvedAt pgtype.Timestamptz
	var vendorName string
	if err := rows.Scan(&inv.ID, &inv.ContractID, &inv.PurchaseRequestID, &inv.VendorID, &inv.VendorInvoiceNo,
		&invoiceDate, &dueDate, &inv.TotalAmount, &inv.EnteredTotal, &inv.Currency, &inv.Status, &inv.Note,
		&inv.AllocationMode, &paidDate, &approvedAt, &inv.CreatedAt, &inv.UpdatedAt, &vendorName); err != nil {
		return err
	}
	if d := formatDate(invoiceDate); d != nil {
		inv.InvoiceDate = *d
	}
	inv.DueDate = formatDate(dueDate)
	inv.PaidDate = formatDate(paidDate)
	if approvedAt.Valid {
		t := approvedAt.Time
		inv.ApprovedAt = &t
	}
	inv.Vendor = &Vendor{ID: inv.VendorID, Name: vendorName}
	return nil
}

// GetInvoice loads an invoice with its items, vendor, approver and documents.
func (r *Repository) GetInvoice(ctx context.Context, id int64) (*Invoice, error) {
	inv := &Invoice{}
	var invoiceDate, dueDate, paidDate pgtype.Date
	var approvedBy pgtype.Int8
	var approvedAt pgtype.Timestamptz
	err := r.pool.QueryRow(ctx, `
		SELECT id, contract_id, purchase_request_id, vendor_id, vendor_invoice_no, invoice_date,
		       due_date, total_amount, entered_total, currency, status, note, allocation_mode, paid_date, approved_by, approved_at, created_at, updated_at
		FROM invoices WHERE id = $1`, id).
		Scan(&inv.ID, &inv.ContractID, &inv.PurchaseRequestID, &inv.VendorID, &inv.VendorInvoiceNo, &invoiceDate,
			&dueDate, &inv.TotalAmount, &inv.EnteredTotal, &inv.Currency, &inv.Status, &inv.Note, &inv.AllocationMode, &paidDate, &approvedBy, &approvedAt, &inv.CreatedAt, &inv.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if d := formatDate(invoiceDate); d != nil {
		inv.InvoiceDate = *d
	}
	inv.DueDate = formatDate(dueDate)
	inv.PaidDate = formatDate(paidDate)
	if approvedAt.Valid {
		t := approvedAt.Time
		inv.ApprovedAt = &t
	}
	if inv.Vendor, err = r.GetVendor(ctx, inv.VendorID); err != nil {
		return nil, err
	}
	if approvedBy.Valid {
		if u, err := r.GetUserByID(ctx, approvedBy.Int64); err == nil {
			inv.Approver = &UserSummary{ID: u.ID, Email: u.Email, Name: u.Name}
		}
	}
	if inv.Items, err = r.listInvoiceItems(ctx, id); err != nil {
		return nil, err
	}
	if inv.CostAllocations, err = r.listInvoiceAllocations(ctx, id); err != nil {
		return nil, err
	}
	resolveAllocationAmounts(inv.AllocationMode, inv.TotalAmount, inv.CostAllocations)
	if inv.Documents, err = r.ListOwnedDocuments(ctx, ownerInvoice, id); err != nil {
		return nil, err
	}
	return inv, nil
}

func (r *Repository) listInvoiceItems(ctx context.Context, invoiceID int64) ([]InvoiceItem, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, description, quantity, unit_price, position FROM invoice_items
		WHERE invoice_id = $1 ORDER BY position, id`, invoiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []InvoiceItem{}
	for rows.Next() {
		var it InvoiceItem
		if err := rows.Scan(&it.ID, &it.Description, &it.Quantity, &it.UnitPrice, &it.Position); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// DeleteInvoice removes an invoice, returning the stored paths of its documents.
func (r *Repository) DeleteInvoice(ctx context.Context, id int64) ([]string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	paths, err := deleteOwnedDocsTx(ctx, tx, ownerInvoice, id)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM invoices WHERE id = $1`, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return paths, nil
}
