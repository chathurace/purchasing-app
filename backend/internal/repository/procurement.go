package repository

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/cs/purchasing-app/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Document owner types, mirrored from the model package so callers within this
// package can reference them without the model import.
const (
	ownerPurchaseRequest = model.OwnerPurchaseRequest
	ownerQuotation       = model.OwnerQuotation
	ownerContract        = model.OwnerContract
)

// ErrHasChildren is returned when an entity cannot be deleted because dependent
// records exist (e.g. a quotation with a contract). Handlers map it to 409.
var ErrHasChildren = errors.New("entity has dependent records")

// ErrInvalidState is returned when an action is not valid for an entity's
// current status (e.g. signing a contract that is not approved). Handlers map
// it to 409.
var ErrInvalidState = errors.New("action not valid for current state")

// =====================================================================
// Vendors
// =====================================================================

type Vendor struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	ContactName string    `json:"contact_name"`
	Email       string    `json:"email"`
	Phone       string    `json:"phone"`
	Notes       string    `json:"notes"`
	IsActive    bool      `json:"is_active"`
	TaxID       string    `json:"tax_id"`
	AddressLine string    `json:"address_line"`
	City        string    `json:"city"`
	PostalCode  string    `json:"postal_code"`
	Country     string    `json:"country"`
	Website     string    `json:"website"`
	Registered  bool      `json:"registered"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type VendorInput struct {
	Name        string
	ContactName string
	Email       string
	Phone       string
	Notes       string
	IsActive    bool
	TaxID       string
	AddressLine string
	City        string
	PostalCode  string
	Country     string
	Website     string
	Registered  bool
}

// VendorLookup is a minimal vendor summary used by the purchase-request
// "proposed supplier" dropdown. Unlike the full vendor list it is open to any
// authenticated user (a requester is usually staff, not procurement).
type VendorLookup struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Website     string `json:"website"`
	ContactName string `json:"contact_name"`
	Email       string `json:"email"`
	Registered  bool   `json:"registered"`
}

// VendorUsage counts the records that reference a vendor, used to show a
// "where used" summary and to explain why a vendor cannot be deleted.
type VendorUsage struct {
	Quotations int `json:"quotations"`
	Contracts  int `json:"contracts"`
	GRNs       int `json:"grns"`
	Invoices   int `json:"invoices"`
}

const vendorCols = `id, name, contact_name, email, phone, notes,
	is_active, tax_id, address_line, city, postal_code, country, website, registered, created_at, updated_at`

func scanVendor(row pgx.Row, v *Vendor) error {
	return row.Scan(&v.ID, &v.Name, &v.ContactName, &v.Email, &v.Phone, &v.Notes,
		&v.IsActive, &v.TaxID, &v.AddressLine, &v.City, &v.PostalCode, &v.Country,
		&v.Website, &v.Registered, &v.CreatedAt, &v.UpdatedAt)
}

func (r *Repository) CreateVendor(ctx context.Context, in VendorInput, createdBy int64) (*Vendor, error) {
	v := &Vendor{}
	err := scanVendor(r.pool.QueryRow(ctx, `
		INSERT INTO vendors (name, contact_name, email, phone, notes, tax_id, address_line, city, postal_code, country, website, registered, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING `+vendorCols,
		in.Name, in.ContactName, in.Email, in.Phone, in.Notes,
		in.TaxID, in.AddressLine, in.City, in.PostalCode, in.Country, in.Website, in.Registered, createdBy), v)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (r *Repository) UpdateVendor(ctx context.Context, id int64, in VendorInput) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE vendors
		SET name = $2, contact_name = $3, email = $4, phone = $5, notes = $6,
			is_active = $7, tax_id = $8, address_line = $9, city = $10, postal_code = $11, country = $12,
			website = $13, registered = $14, updated_at = NOW()
		WHERE id = $1`,
		id, in.Name, in.ContactName, in.Email, in.Phone, in.Notes,
		in.IsActive, in.TaxID, in.AddressLine, in.City, in.PostalCode, in.Country, in.Website, in.Registered)
	return err
}

func (r *Repository) ListVendors(ctx context.Context) ([]*Vendor, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+vendorCols+` FROM vendors ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Vendor{}
	for rows.Next() {
		v := &Vendor{}
		if err := scanVendor(rows, v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repository) GetVendor(ctx context.Context, id int64) (*Vendor, error) {
	v := &Vendor{}
	if err := scanVendor(r.pool.QueryRow(ctx, `SELECT `+vendorCols+` FROM vendors WHERE id = $1`, id), v); err != nil {
		return nil, err
	}
	return v, nil
}

// ListVendorsLookup returns active vendors as minimal summaries for the
// purchase-request "proposed supplier" dropdown.
func (r *Repository) ListVendorsLookup(ctx context.Context) ([]VendorLookup, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, website, contact_name, email, registered
		FROM vendors WHERE is_active ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []VendorLookup{}
	for rows.Next() {
		var v VendorLookup
		if err := rows.Scan(&v.ID, &v.Name, &v.Website, &v.ContactName, &v.Email, &v.Registered); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetVendorUsage returns the number of quotations, contracts, GRNs and invoices
// that reference the vendor.
func (r *Repository) GetVendorUsage(ctx context.Context, id int64) (VendorUsage, error) {
	var u VendorUsage
	err := r.pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM quotations WHERE vendor_id = $1),
			(SELECT COUNT(*) FROM contracts  WHERE vendor_id = $1),
			(SELECT COUNT(*) FROM grns        WHERE vendor_id = $1),
			(SELECT COUNT(*) FROM invoices    WHERE vendor_id = $1)`, id).
		Scan(&u.Quotations, &u.Contracts, &u.GRNs, &u.Invoices)
	return u, err
}

// =====================================================================
// Budget units (formerly cost centers)
// =====================================================================

// BudgetUnitBracket is one ordered value range of a budget unit and the
// approver(s) qualified to sign off a budget card that resolves to it. MinValue
// is inclusive; MaxValue is inclusive and nil means unbounded (any value). All
// Approvers of the resolved bracket are qualified — there is no "first" pick.
type BudgetUnitBracket struct {
	ID        int64          `json:"id"`
	Position  int            `json:"position"`
	Currency  string         `json:"currency"`
	MinValue  float64        `json:"min_value"`
	MaxValue  *float64       `json:"max_value"`
	Approvers []*UserSummary `json:"approvers"`
}

type BudgetUnit struct {
	ID          int64   `json:"id"`
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Budget      float64 `json:"budget"`
	Currency    string  `json:"currency"`
	IsActive    bool    `json:"is_active"`
	// DefaultApproverID is the catch-all budget approver, used when a PR matches
	// no bracket (no value, no currency match, or out of every range).
	DefaultApproverID *int64              `json:"default_approver_id"`
	DefaultApprover   *UserSummary        `json:"default_approver,omitempty"`
	CreatedAt         time.Time           `json:"created_at"`
	UpdatedAt         time.Time           `json:"updated_at"`
	Brackets          []BudgetUnitBracket `json:"brackets"`
}

// BudgetUnitBracketInput is one bracket on a create/update; brackets are ordered
// by their position in the slice. A bracket matches on its own Currency + range.
type BudgetUnitBracketInput struct {
	Currency    string
	MinValue    float64
	MaxValue    *float64
	ApproverIDs []int64
}

type BudgetUnitInput struct {
	Code              string
	Name              string
	Description       string
	Budget            float64
	Currency          string
	IsActive          bool
	DefaultApproverID *int64
	Brackets          []BudgetUnitBracketInput
}

// BudgetUnitSummary is the lightweight shape returned by the lookup endpoint
// (used to populate the budget-unit dropdown on a purchase request). Currency is
// included so the client can label the estimated-value input.
type BudgetUnitSummary struct {
	ID       int64  `json:"id"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	Currency string `json:"currency"`
}

// BudgetUnitUsage counts the records that reference a budget unit, used to show
// a "where used" summary before deactivating.
type BudgetUnitUsage struct {
	PurchaseRequests int `json:"purchase_requests"`
}

const budgetUnitCols = `id, code, name, description,
	budget, currency, is_active, default_approver_id, created_at, updated_at`

func scanBudgetUnit(row pgx.Row, c *BudgetUnit) error {
	return row.Scan(&c.ID, &c.Code, &c.Name, &c.Description,
		&c.Budget, &c.Currency, &c.IsActive, &c.DefaultApproverID, &c.CreatedAt, &c.UpdatedAt)
}

// populateDefaultApprover fills the default-approver UserSummary from its id.
func (r *Repository) populateDefaultApprover(ctx context.Context, c *BudgetUnit) error {
	if c.DefaultApproverID == nil {
		return nil
	}
	var email, name pgtype.Text
	var id int64
	err := r.pool.QueryRow(ctx, `SELECT id, email, name FROM users WHERE id = $1`, *c.DefaultApproverID).
		Scan(&id, &email, &name)
	if err == nil {
		c.DefaultApprover = &UserSummary{ID: id, Email: email.String, Name: name.String}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	return nil
}

// listBracketApprovers returns the approvers of one bracket, ordered.
func (r *Repository) listBracketApprovers(ctx context.Context, bracketID int64) ([]*UserSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT u.id, u.email, u.name
		FROM budget_unit_bracket_approvers ba
		JOIN users u ON u.id = ba.user_id
		WHERE ba.bracket_id = $1
		ORDER BY ba.position, u.id`, bracketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*UserSummary{}
	for rows.Next() {
		u := &UserSummary{}
		var email, name pgtype.Text
		if err := rows.Scan(&u.ID, &email, &name); err != nil {
			return nil, err
		}
		u.Email = email.String
		u.Name = name.String
		out = append(out, u)
	}
	return out, rows.Err()
}

// populateBrackets fills the ordered brackets (with their approvers) on a BU.
func (r *Repository) populateBrackets(ctx context.Context, c *BudgetUnit) error {
	rows, err := r.pool.Query(ctx, `
		SELECT id, position, currency, min_value, max_value
		FROM budget_unit_brackets
		WHERE budget_unit_id = $1
		ORDER BY position, id`, c.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	brackets := []BudgetUnitBracket{}
	for rows.Next() {
		b := BudgetUnitBracket{Approvers: []*UserSummary{}}
		if err := rows.Scan(&b.ID, &b.Position, &b.Currency, &b.MinValue, &b.MaxValue); err != nil {
			return err
		}
		brackets = append(brackets, b)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range brackets {
		approvers, err := r.listBracketApprovers(ctx, brackets[i].ID)
		if err != nil {
			return err
		}
		brackets[i].Approvers = approvers
	}
	c.Brackets = brackets
	return nil
}

func (r *Repository) ListBudgetUnits(ctx context.Context) ([]*BudgetUnit, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+budgetUnitCols+` FROM budget_units ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*BudgetUnit{}
	for rows.Next() {
		c := &BudgetUnit{}
		if err := scanBudgetUnit(rows, c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, c := range out {
		if err := r.populateBrackets(ctx, c); err != nil {
			return nil, err
		}
		if err := r.populateDefaultApprover(ctx, c); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// ListActiveBudgetUnits returns active budget units as summaries, for the PR
// dropdown. Readable by any authenticated user.
func (r *Repository) ListActiveBudgetUnits(ctx context.Context) ([]*BudgetUnitSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, code, name, currency
		FROM budget_units
		WHERE is_active ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*BudgetUnitSummary{}
	for rows.Next() {
		c := &BudgetUnitSummary{}
		if err := rows.Scan(&c.ID, &c.Code, &c.Name, &c.Currency); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repository) GetBudgetUnit(ctx context.Context, id int64) (*BudgetUnit, error) {
	c := &BudgetUnit{}
	if err := scanBudgetUnit(r.pool.QueryRow(ctx, `SELECT `+budgetUnitCols+` FROM budget_units WHERE id = $1`, id), c); err != nil {
		return nil, err
	}
	if err := r.populateBrackets(ctx, c); err != nil {
		return nil, err
	}
	if err := r.populateDefaultApprover(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// replaceBrackets deletes and re-inserts the brackets (and their approvers) for
// a budget unit, preserving slice order as the bracket position.
func replaceBrackets(ctx context.Context, tx pgx.Tx, budgetUnitID int64, brackets []BudgetUnitBracketInput) error {
	if _, err := tx.Exec(ctx, `DELETE FROM budget_unit_brackets WHERE budget_unit_id = $1`, budgetUnitID); err != nil {
		return err
	}
	for pos, b := range brackets {
		var bracketID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO budget_unit_brackets (budget_unit_id, position, currency, min_value, max_value)
			VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			budgetUnitID, pos, b.Currency, b.MinValue, b.MaxValue).Scan(&bracketID); err != nil {
			return err
		}
		for apos, uid := range b.ApproverIDs {
			if _, err := tx.Exec(ctx, `
				INSERT INTO budget_unit_bracket_approvers (bracket_id, user_id, position)
				VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, bracketID, uid, apos); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Repository) CreateBudgetUnit(ctx context.Context, in BudgetUnitInput, createdBy int64) (*BudgetUnit, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var id int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO budget_units (code, name, description, budget, currency, is_active, default_approver_id, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`,
		in.Code, in.Name, in.Description, in.Budget, in.Currency, in.IsActive, in.DefaultApproverID, createdBy).Scan(&id); err != nil {
		return nil, err
	}
	if err := replaceBrackets(ctx, tx, id, in.Brackets); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.GetBudgetUnit(ctx, id)
}

func (r *Repository) UpdateBudgetUnit(ctx context.Context, id int64, in BudgetUnitInput) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE budget_units
		SET code = $2, name = $3, description = $4,
			budget = $5, currency = $6, is_active = $7, default_approver_id = $8, updated_at = NOW()
		WHERE id = $1`,
		id, in.Code, in.Name, in.Description, in.Budget, in.Currency, in.IsActive, in.DefaultApproverID); err != nil {
		return err
	}
	if err := replaceBrackets(ctx, tx, id, in.Brackets); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// GetBudgetUnitUsage returns the number of purchase requests that reference the
// budget unit.
func (r *Repository) GetBudgetUnitUsage(ctx context.Context, id int64) (BudgetUnitUsage, error) {
	var u BudgetUnitUsage
	err := r.pool.QueryRow(ctx, `
		SELECT (SELECT COUNT(*) FROM purchase_requests WHERE budget_unit_id = $1)`, id).
		Scan(&u.PurchaseRequests)
	return u, err
}

// CurrencyTotal is an amount paired with its currency. Amounts of differing
// currencies are kept separate — never summed (see formatMoney's contract).
type CurrencyTotal struct {
	Currency string  `json:"currency"`
	Amount   float64 `json:"amount"`
}

// BudgetUnitInvoiceCategory summarises one invoice status bucket for a budget
// unit: how many invoices allocate to it and the allocated totals per currency.
type BudgetUnitInvoiceCategory struct {
	Count  int             `json:"count"`
	Totals []CurrencyTotal `json:"totals"`
}

// BudgetUnitInvoiceSummary buckets the invoices allocated to a budget unit by
// status (pending = received), each with the budget unit's allocated share.
type BudgetUnitInvoiceSummary struct {
	Pending  BudgetUnitInvoiceCategory `json:"pending"`
	Approved BudgetUnitInvoiceCategory `json:"approved"`
	Paid     BudgetUnitInvoiceCategory `json:"paid"`
}

// GetBudgetUnitInvoiceSummary returns the invoices allocated to a budget unit,
// bucketed by status, with this budget unit's allocated share totalled per
// currency. The share is the resolved allocation amount in the invoice's
// currency (percentage of the invoice total, or the absolute amount).
func (r *Repository) GetBudgetUnitInvoiceSummary(ctx context.Context, id int64) (BudgetUnitInvoiceSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT i.status, i.currency,
		       CASE WHEN i.allocation_mode = 'amount' THEN a.value
		            ELSE i.total_amount * a.value / 100 END AS allocated
		FROM invoice_cost_allocations a
		JOIN invoices i ON i.id = a.invoice_id
		WHERE a.budget_unit_id = $1`, id)
	if err != nil {
		return BudgetUnitInvoiceSummary{}, err
	}
	defer rows.Close()

	// Accumulate per status, then per currency within that status.
	counts := map[string]int{}
	totals := map[string]map[string]float64{
		model.InvoiceReceived: {}, model.InvoiceApproved: {}, model.InvoicePaid: {},
	}
	for rows.Next() {
		var status, currency string
		var allocated float64
		if err := rows.Scan(&status, &currency, &allocated); err != nil {
			return BudgetUnitInvoiceSummary{}, err
		}
		if _, ok := totals[status]; !ok {
			continue // unknown status; ignore defensively
		}
		counts[status]++
		totals[status][currency] += allocated
	}
	if err := rows.Err(); err != nil {
		return BudgetUnitInvoiceSummary{}, err
	}
	return BudgetUnitInvoiceSummary{
		Pending:  buildInvoiceCategory(counts[model.InvoiceReceived], totals[model.InvoiceReceived]),
		Approved: buildInvoiceCategory(counts[model.InvoiceApproved], totals[model.InvoiceApproved]),
		Paid:     buildInvoiceCategory(counts[model.InvoicePaid], totals[model.InvoicePaid]),
	}, nil
}

func buildInvoiceCategory(count int, byCurrency map[string]float64) BudgetUnitInvoiceCategory {
	totals := make([]CurrencyTotal, 0, len(byCurrency))
	for currency, amount := range byCurrency {
		totals = append(totals, CurrencyTotal{Currency: currency, Amount: amount})
	}
	// Stable order by currency so the response is deterministic.
	sort.Slice(totals, func(i, j int) bool { return totals[i].Currency < totals[j].Currency })
	return BudgetUnitInvoiceCategory{Count: count, Totals: totals}
}

// =====================================================================
// Quotations
// =====================================================================

type QuotationItem struct {
	ID          int64   `json:"id"`
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
	Position    int     `json:"position"`
}

type Quotation struct {
	ID                int64   `json:"id"`
	PurchaseRequestID int64   `json:"purchase_request_id"`
	VendorID          int64   `json:"vendor_id"`
	TotalAmount       float64 `json:"total_amount"`
	Currency          string  `json:"currency"`
	ValidUntil        *string `json:"valid_until"` // YYYY-MM-DD or null
	Notes             string  `json:"notes"`
	Status            string  `json:"status"`
	// QuotationDocumentID references the single primary quotation PDF (if any),
	// pulled out of Documents on detail reads and exposed as QuotationDocument.
	QuotationDocumentID *int64          `json:"quotation_document_id"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
	Items               []QuotationItem `json:"items"`
	Vendor              *Vendor         `json:"vendor,omitempty"`
	// Documents are the other supporting documents; the primary quotation PDF is
	// exposed separately as QuotationDocument.
	Documents         []Document `json:"documents,omitempty"`
	QuotationDocument *Document  `json:"quotation_document,omitempty"`
}

type QuotationInput struct {
	VendorID    int64
	TotalAmount float64
	Currency    string
	ValidUntil  *string
	Notes       string
	Items       []QuotationItem
}

// CreateQuotation inserts a quotation (with its line items) directly against a
// purchase request and advances the PR into review.
func (r *Repository) CreateQuotation(ctx context.Context, prID int64, in QuotationInput, createdBy int64) (*Quotation, error) {
	validUntil, err := parseDate(in.ValidUntil)
	if err != nil {
		return nil, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var id int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO quotations (purchase_request_id, vendor_id, total_amount, currency, valid_until, notes, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		prID, in.VendorID, in.TotalAmount, in.Currency, validUntil, in.Notes, createdBy).Scan(&id); err != nil {
		return nil, err
	}
	if err := replaceQuotationItems(ctx, tx, id, in.Items); err != nil {
		return nil, err
	}
	if err := advancePR(ctx, tx, prID, model.NextPRStatusForQuotationCreated); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.GetQuotation(ctx, id)
}

// UpdateQuotation updates the quotation fields and replaces its line items.
func (r *Repository) UpdateQuotation(ctx context.Context, id int64, in QuotationInput) error {
	validUntil, err := parseDate(in.ValidUntil)
	if err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE quotations
		SET vendor_id = $2, total_amount = $3, currency = $4, valid_until = $5, notes = $6, updated_at = NOW()
		WHERE id = $1`, id, in.VendorID, in.TotalAmount, in.Currency, validUntil, in.Notes); err != nil {
		return err
	}
	if err := replaceQuotationItems(ctx, tx, id, in.Items); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func replaceQuotationItems(ctx context.Context, tx pgx.Tx, quotationID int64, items []QuotationItem) error {
	if _, err := tx.Exec(ctx, `DELETE FROM quotation_items WHERE quotation_id = $1`, quotationID); err != nil {
		return err
	}
	for i, it := range items {
		if _, err := tx.Exec(ctx, `
			INSERT INTO quotation_items (quotation_id, description, quantity, unit_price, position)
			VALUES ($1, $2, $3, $4, $5)`, quotationID, it.Description, it.Quantity, it.UnitPrice, i); err != nil {
			return err
		}
	}
	return nil
}

// ListQuotations returns quotation summaries with vendor info. If prID is
// non-nil, results are scoped to that purchase request.
const quotationSummarySelect = `
	SELECT q.id, q.purchase_request_id, q.vendor_id, q.total_amount, q.currency, q.valid_until, q.notes,
	       q.status, q.created_at, q.updated_at, v.name
	FROM quotations q
	JOIN vendors v ON v.id = q.vendor_id`

func scanQuotationSummaries(rows pgx.Rows) ([]*Quotation, error) {
	defer rows.Close()
	out := []*Quotation{}
	for rows.Next() {
		q := &Quotation{}
		var validUntil pgtype.Date
		var vendorName string
		if err := rows.Scan(&q.ID, &q.PurchaseRequestID, &q.VendorID, &q.TotalAmount, &q.Currency, &validUntil,
			&q.Notes, &q.Status, &q.CreatedAt, &q.UpdatedAt, &vendorName); err != nil {
			return nil, err
		}
		q.ValidUntil = formatDate(validUntil)
		q.Vendor = &Vendor{ID: q.VendorID, Name: vendorName}
		out = append(out, q)
	}
	return out, rows.Err()
}

func (r *Repository) ListQuotations(ctx context.Context, prID *int64) ([]*Quotation, error) {
	query := quotationSummarySelect
	args := []any{}
	if prID != nil {
		query += ` WHERE q.purchase_request_id = $1`
		args = append(args, *prID)
	}
	query += ` ORDER BY q.created_at DESC`

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return scanQuotationSummaries(rows)
}

// ListQuotationsForApprover returns quotation summaries limited to PRs the caller
// approves (see approvablePredicate) — the read-only Quotations tab for approvers.
func (r *Repository) ListQuotationsForApprover(ctx context.Context, callerID int64, hasLegal, hasSecurity bool) ([]*Quotation, error) {
	rows, err := r.pool.Query(ctx, quotationSummarySelect+`
		WHERE q.purchase_request_id IN (
			SELECT pr.id FROM purchase_requests pr WHERE `+approvablePredicate+`)
		ORDER BY q.created_at DESC`, callerID, hasLegal, hasSecurity)
	if err != nil {
		return nil, err
	}
	return scanQuotationSummaries(rows)
}

// ListQuotationsForPR returns a purchase request's quotation summaries (with
// vendor info), newest first. Used to surface a case's related records.
func (r *Repository) ListQuotationsForPR(ctx context.Context, prID int64) ([]*Quotation, error) {
	return r.ListQuotations(ctx, &prID)
}

// GetQuotation loads a quotation with its items, vendor, documents and owning PR.
func (r *Repository) GetQuotation(ctx context.Context, id int64) (*Quotation, error) {
	q := &Quotation{}
	var validUntil pgtype.Date
	var quotationDocID pgtype.Int8
	err := r.pool.QueryRow(ctx, `
		SELECT q.id, q.purchase_request_id, q.vendor_id, q.total_amount, q.currency, q.valid_until, q.notes,
		       q.status, q.quotation_document_id, q.created_at, q.updated_at
		FROM quotations q
		WHERE q.id = $1`, id).
		Scan(&q.ID, &q.PurchaseRequestID, &q.VendorID, &q.TotalAmount, &q.Currency, &validUntil, &q.Notes,
			&q.Status, &quotationDocID, &q.CreatedAt, &q.UpdatedAt)
	if err != nil {
		return nil, err
	}
	q.ValidUntil = formatDate(validUntil)
	if quotationDocID.Valid {
		q.QuotationDocumentID = &quotationDocID.Int64
	}
	if q.Vendor, err = r.GetVendor(ctx, q.VendorID); err != nil {
		return nil, err
	}
	if q.Items, err = r.listQuotationItems(ctx, id); err != nil {
		return nil, err
	}
	// All quotation-owned documents; the primary PDF (if any) is pulled out
	// separately so Documents holds only the other supporting documents.
	allDocs, err := r.ListOwnedDocuments(ctx, ownerQuotation, id)
	if err != nil {
		return nil, err
	}
	q.Documents = make([]Document, 0, len(allDocs))
	for i := range allDocs {
		if q.QuotationDocumentID != nil && allDocs[i].ID == *q.QuotationDocumentID {
			doc := allDocs[i]
			q.QuotationDocument = &doc
			continue
		}
		q.Documents = append(q.Documents, allDocs[i])
	}
	return q, nil
}

func (r *Repository) listQuotationItems(ctx context.Context, quotationID int64) ([]QuotationItem, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, description, quantity, unit_price, position FROM quotation_items
		WHERE quotation_id = $1 ORDER BY position, id`, quotationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []QuotationItem{}
	for rows.Next() {
		var it QuotationItem
		if err := rows.Scan(&it.ID, &it.Description, &it.Quantity, &it.UnitPrice, &it.Position); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// SetQuotationDocument attaches (or replaces) the quotation's single primary
// PDF. Returns the id of the previously attached document (if any, and
// different) so the caller can delete the old file.
func (r *Repository) SetQuotationDocument(ctx context.Context, quotationID, docID int64) (*int64, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var prev pgtype.Int8
	if err := tx.QueryRow(ctx, `
		SELECT quotation_document_id FROM quotations WHERE id = $1`, quotationID).Scan(&prev); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE quotations SET quotation_document_id = $2, updated_at = NOW() WHERE id = $1`,
		quotationID, docID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	if prev.Valid && prev.Int64 != docID {
		return &prev.Int64, nil
	}
	return nil, nil
}

// ClearQuotationDocument removes the primary quotation PDF (clearing the FK and
// deleting the document row) and returns the stored path of the removed file for
// the caller to unlink. Returns ErrInvalidState if no primary PDF is set.
func (r *Repository) ClearQuotationDocument(ctx context.Context, quotationID int64) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var docID pgtype.Int8
	if err := tx.QueryRow(ctx, `
		SELECT quotation_document_id FROM quotations WHERE id = $1`, quotationID).Scan(&docID); err != nil {
		return "", err
	}
	if !docID.Valid {
		return "", ErrInvalidState
	}
	var storedPath string
	if err := tx.QueryRow(ctx, `SELECT stored_path FROM documents WHERE id = $1`, docID.Int64).Scan(&storedPath); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE quotations SET quotation_document_id = NULL, updated_at = NOW() WHERE id = $1`, quotationID); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM documents WHERE id = $1`, docID.Int64); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return storedPath, nil
}

// SelectQuotation marks a quotation as selected and advances the PR to
// vendor_selected.
func (r *Repository) SelectQuotation(ctx context.Context, id int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var prID int64
	if err := tx.QueryRow(ctx, `
		SELECT purchase_request_id FROM quotations WHERE id = $1`, id).Scan(&prID); err != nil {
		return err
	}
	// Gate: the PR's procurement recommendation must be fully approved first.
	if ok, err := recommendationFullyApprovedTx(ctx, tx, prID); err != nil {
		return err
	} else if !ok {
		return ErrInvalidState
	}
	if _, err := tx.Exec(ctx, `
		UPDATE quotations SET status = $2, updated_at = NOW() WHERE id = $1`,
		id, model.QuoSelected); err != nil {
		return err
	}
	if err := advancePR(ctx, tx, prID, model.NextPRStatusForQuotationSelected); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// DeleteQuotation removes a quotation that no contract references, returning the
// stored paths of its documents.
func (r *Repository) DeleteQuotation(ctx context.Context, id int64) ([]string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM contracts WHERE quotation_id = $1`, id).Scan(&n); err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, ErrHasChildren
	}
	paths, err := deleteOwnedDocsTx(ctx, tx, ownerQuotation, id)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM quotations WHERE id = $1`, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return paths, nil
}

// =====================================================================
// Contracts
// =====================================================================

type Contract struct {
	ID                int64     `json:"id"`
	PurchaseRequestID int64     `json:"purchase_request_id"`
	QuotationID       *int64    `json:"quotation_id"`
	VendorID          int64     `json:"vendor_id"`
	Title             string    `json:"title"`
	TotalAmount       float64   `json:"total_amount"`
	Currency          string    `json:"currency"`
	Terms             string    `json:"terms"`
	Status            string    `json:"status"`
	SignedDocumentID  *int64    `json:"signed_document_id"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	Vendor            *Vendor   `json:"vendor,omitempty"`
	// Documents are the draft-contract PDFs (each with its own notes); the signed
	// PDF is exposed separately as SignedDocument.
	Documents      []Document `json:"documents,omitempty"`
	SignedDocument *Document  `json:"signed_document,omitempty"`
	// BudgetUnit is the budget unit of the owning purchase request (if any),
	// used to default an invoice's cost allocation. Populated on detail reads.
	BudgetUnit *BudgetUnitSummary `json:"budget_unit,omitempty"`
	// InvoicedTotal is the sum of this contract's invoice totals (any status), in
	// the contract's own currency. Populated on detail reads to drive the
	// over-billing warning. Not summed across differing currencies.
	InvoicedTotal float64 `json:"invoiced_total"`
}

type ContractInput struct {
	Title       string
	TotalAmount float64
	Currency    string
	Terms       string
}

func (r *Repository) UpdateContract(ctx context.Context, id int64, in ContractInput) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE contracts
		SET title = $2, total_amount = $3, currency = $4, terms = $5, updated_at = NOW()
		WHERE id = $1`, id, in.Title, in.TotalAmount, in.Currency, in.Terms)
	return err
}

const contractSummarySelect = `
	SELECT c.id, c.purchase_request_id, c.quotation_id, c.vendor_id, c.title, c.total_amount,
	       c.currency, c.terms, c.status, c.signed_document_id, c.created_at, c.updated_at, v.name
	FROM contracts c
	JOIN vendors v ON v.id = c.vendor_id`

func scanContractSummaries(rows pgx.Rows) ([]*Contract, error) {
	defer rows.Close()
	out := []*Contract{}
	for rows.Next() {
		c := &Contract{}
		var quotationID, signedDocID pgtype.Int8
		var vendorName string
		if err := rows.Scan(&c.ID, &c.PurchaseRequestID, &quotationID, &c.VendorID, &c.Title,
			&c.TotalAmount, &c.Currency, &c.Terms, &c.Status, &signedDocID, &c.CreatedAt, &c.UpdatedAt, &vendorName); err != nil {
			return nil, err
		}
		if quotationID.Valid {
			c.QuotationID = &quotationID.Int64
		}
		if signedDocID.Valid {
			c.SignedDocumentID = &signedDocID.Int64
		}
		c.Vendor = &Vendor{ID: c.VendorID, Name: vendorName}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListContracts returns contract summaries with vendor info. If prID is
// non-nil, results are scoped to that purchase request.
func (r *Repository) ListContracts(ctx context.Context, prID *int64) ([]*Contract, error) {
	query := contractSummarySelect
	args := []any{}
	if prID != nil {
		query += ` WHERE c.purchase_request_id = $1`
		args = append(args, *prID)
	}
	query += ` ORDER BY c.created_at DESC`

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return scanContractSummaries(rows)
}

// ListContractsForApprover returns contract summaries limited to PRs the caller
// approves (see approvablePredicate) — the read-only Contracts tab for approvers.
func (r *Repository) ListContractsForApprover(ctx context.Context, callerID int64, hasLegal, hasSecurity bool) ([]*Contract, error) {
	rows, err := r.pool.Query(ctx, contractSummarySelect+`
		WHERE c.purchase_request_id IN (
			SELECT pr.id FROM purchase_requests pr WHERE `+approvablePredicate+`)
		ORDER BY c.created_at DESC`, callerID, hasLegal, hasSecurity)
	if err != nil {
		return nil, err
	}
	return scanContractSummaries(rows)
}

// GetContract loads a contract with its vendor, approvals, documents and signed
// document.
func (r *Repository) GetContract(ctx context.Context, id int64) (*Contract, error) {
	c := &Contract{}
	var quotationID, signedDocID pgtype.Int8
	err := r.pool.QueryRow(ctx, `
		SELECT id, purchase_request_id, quotation_id, vendor_id, title, total_amount, currency,
		       terms, status, signed_document_id, created_at, updated_at
		FROM contracts WHERE id = $1`, id).
		Scan(&c.ID, &c.PurchaseRequestID, &quotationID, &c.VendorID, &c.Title, &c.TotalAmount,
			&c.Currency, &c.Terms, &c.Status, &signedDocID, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if quotationID.Valid {
		c.QuotationID = &quotationID.Int64
	}
	if signedDocID.Valid {
		c.SignedDocumentID = &signedDocID.Int64
	}
	if c.Vendor, err = r.GetVendor(ctx, c.VendorID); err != nil {
		return nil, err
	}
	// All contract-owned PDFs; the signed one (if any) is pulled out separately so
	// Documents holds only the draft contracts.
	allDocs, err := r.ListOwnedDocuments(ctx, ownerContract, id)
	if err != nil {
		return nil, err
	}
	c.Documents = make([]Document, 0, len(allDocs))
	for i := range allDocs {
		if c.SignedDocumentID != nil && allDocs[i].ID == *c.SignedDocumentID {
			doc := allDocs[i]
			c.SignedDocument = &doc
			continue
		}
		c.Documents = append(c.Documents, allDocs[i])
	}
	if c.InvoicedTotal, err = r.contractInvoicedTotal(ctx, id, c.Currency); err != nil {
		return nil, err
	}
	// Resolve the owning PR's budget unit (if set), used to default an invoice's
	// cost allocation. A missing budget unit is not an error.
	var buID pgtype.Int8
	var buCode, buName, buCurrency pgtype.Text
	if err := r.pool.QueryRow(ctx, `
		SELECT bu.id, bu.code, bu.name, bu.currency
		FROM purchase_requests pr
		JOIN budget_units bu ON bu.id = pr.budget_unit_id
		WHERE pr.id = $1`, c.PurchaseRequestID).Scan(&buID, &buCode, &buName, &buCurrency); err == nil {
		c.BudgetUnit = &BudgetUnitSummary{ID: buID.Int64, Code: buCode.String, Name: buName.String, Currency: buCurrency.String}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	return c, nil
}

// SetContractSigned attaches the signed document and advances the contract to
// signed (and the PR to order_signed). Allowed from draft (first signing) or
// signed (replacing the signed PDF). Returns the id of the previously signed
// document (if any) so the caller can delete the old file.
func (r *Repository) SetContractSigned(ctx context.Context, contractID, signedDocID int64) (*int64, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var status string
	var prID int64
	var prevSigned pgtype.Int8
	if err := tx.QueryRow(ctx, `
		SELECT status, purchase_request_id, signed_document_id FROM contracts WHERE id = $1`, contractID).
		Scan(&status, &prID, &prevSigned); err != nil {
		return nil, err
	}
	if status != model.ContractDraft && status != model.ContractSigned {
		return nil, ErrInvalidState
	}
	if _, err := tx.Exec(ctx, `
		UPDATE contracts SET signed_document_id = $2, status = $3, updated_at = NOW() WHERE id = $1`,
		contractID, signedDocID, model.ContractSigned); err != nil {
		return nil, err
	}
	// Advance the PR only on first signing; the conditional transition makes a
	// re-sign (already order_signed) a no-op.
	if err := advancePR(ctx, tx, prID, model.NextPRStatusForContractSigned); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	if prevSigned.Valid && prevSigned.Int64 != signedDocID {
		return &prevSigned.Int64, nil
	}
	return nil, nil
}

// ClearContractSigned removes the signed PDF and reverts the contract to draft
// (and the PR from order_signed back to contract_prepared). It refuses if the
// contract already has GRNs or invoices recorded against it, and returns the
// stored path of the removed signed document for the caller to unlink.
func (r *Repository) ClearContractSigned(ctx context.Context, contractID int64) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var prID int64
	var signed pgtype.Int8
	if err := tx.QueryRow(ctx, `
		SELECT purchase_request_id, signed_document_id FROM contracts WHERE id = $1`, contractID).
		Scan(&prID, &signed); err != nil {
		return "", err
	}
	if !signed.Valid {
		return "", ErrInvalidState
	}
	var fulfillment int
	if err := tx.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM grns WHERE contract_id = $1)
		     + (SELECT count(*) FROM invoices WHERE contract_id = $1)`, contractID).Scan(&fulfillment); err != nil {
		return "", err
	}
	if fulfillment > 0 {
		return "", ErrInvalidState
	}
	var storedPath string
	if err := tx.QueryRow(ctx, `SELECT stored_path FROM documents WHERE id = $1`, signed.Int64).Scan(&storedPath); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE contracts SET signed_document_id = NULL, status = $2, updated_at = NOW() WHERE id = $1`,
		contractID, model.ContractDraft); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM documents WHERE id = $1`, signed.Int64); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE purchase_requests SET status = $2, updated_at = NOW()
		WHERE id = $1 AND status = $3`, prID, model.StatusContractPrepared, model.StatusOrderSigned); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return storedPath, nil
}

// =====================================================================
// Purchase-request rejection
// =====================================================================

// RejectPurchaseRequest marks a PR rejected with a reason and cancels its open
// children (non-selected quotations, unsigned contracts). Signed contracts are
// left untouched.
func (r *Repository) RejectPurchaseRequest(ctx context.Context, id int64, reason string, _ int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE purchase_requests SET status = $2, rejection_reason = $3, updated_at = NOW() WHERE id = $1`,
		id, model.StatusRejected, reason); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE quotations SET status = $2, updated_at = NOW()
		WHERE purchase_request_id = $1 AND status <> $3`,
		id, model.QuoRejected, model.QuoSelected); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE contracts SET status = $2, updated_at = NOW()
		WHERE purchase_request_id = $1 AND status IN ($3, $4, $5)`,
		id, model.ContractRejected, model.ContractDraft, model.ContractPendingApproval, model.ContractApproved); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// =====================================================================
// Owner-scoped documents (shared by all procurement entities)
// =====================================================================

func (r *Repository) AddOwnedDocument(ctx context.Context, prID int64, ownerType string, ownerID int64, filename, storedPath, contentType string, size, uploadedBy int64, notes string) (*Document, error) {
	d := &Document{}
	var up pgtype.Int8
	err := r.pool.QueryRow(ctx, `
		INSERT INTO documents (purchase_request_id, owner_type, owner_id, filename, stored_path, content_type, size_bytes, uploaded_by, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, filename, stored_path, content_type, size_bytes, notes, uploaded_by, created_at`,
		prID, ownerType, ownerID, filename, storedPath, contentType, size, uploadedBy, notes).
		Scan(&d.ID, &d.Filename, &d.StoredPath, &d.ContentType, &d.SizeBytes, &d.Notes, &up, &d.CreatedAt)
	if err != nil {
		return nil, err
	}
	if up.Valid {
		d.UploadedBy = &up.Int64
	}
	return d, nil
}

func (r *Repository) ListOwnedDocuments(ctx context.Context, ownerType string, ownerID int64) ([]Document, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, filename, stored_path, content_type, size_bytes, notes, uploaded_by, created_at
		FROM documents WHERE owner_type = $1 AND owner_id = $2 ORDER BY created_at, id`, ownerType, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	docs := []Document{}
	for rows.Next() {
		var d Document
		var up pgtype.Int8
		if err := rows.Scan(&d.ID, &d.Filename, &d.StoredPath, &d.ContentType, &d.SizeBytes, &d.Notes, &up, &d.CreatedAt); err != nil {
			return nil, err
		}
		if up.Valid {
			d.UploadedBy = &up.Int64
		}
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

func (r *Repository) GetOwnedDocument(ctx context.Context, ownerType string, ownerID, docID int64) (*Document, error) {
	d := &Document{}
	var up pgtype.Int8
	err := r.pool.QueryRow(ctx, `
		SELECT id, filename, stored_path, content_type, size_bytes, notes, uploaded_by, created_at
		FROM documents WHERE id = $1 AND owner_type = $2 AND owner_id = $3`, docID, ownerType, ownerID).
		Scan(&d.ID, &d.Filename, &d.StoredPath, &d.ContentType, &d.SizeBytes, &d.Notes, &up, &d.CreatedAt)
	if err != nil {
		return nil, err
	}
	if up.Valid {
		d.UploadedBy = &up.Int64
	}
	return d, nil
}

// UpdateOwnedDocumentNotes sets the notes on a document scoped to its owner.
func (r *Repository) UpdateOwnedDocumentNotes(ctx context.Context, ownerType string, ownerID, docID int64, notes string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE documents SET notes = $4 WHERE id = $1 AND owner_type = $2 AND owner_id = $3`,
		docID, ownerType, ownerID, notes)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *Repository) DeleteOwnedDocument(ctx context.Context, ownerType string, ownerID, docID int64) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM documents WHERE id = $1 AND owner_type = $2 AND owner_id = $3`, docID, ownerType, ownerID)
	return err
}

// deleteOwnedDocsTx removes all document rows owned by an entity within a tx and
// returns their stored paths for the caller to unlink after commit.
func deleteOwnedDocsTx(ctx context.Context, tx pgx.Tx, ownerType string, ownerID int64) ([]string, error) {
	rows, err := tx.Query(ctx, `SELECT stored_path FROM documents WHERE owner_type = $1 AND owner_id = $2`, ownerType, ownerID)
	if err != nil {
		return nil, err
	}
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return nil, err
		}
		paths = append(paths, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM documents WHERE owner_type = $1 AND owner_id = $2`, ownerType, ownerID); err != nil {
		return nil, err
	}
	return paths, nil
}

// =====================================================================
// Shared helpers
// =====================================================================

// advancePR reads the PR's current status and applies a transition function,
// updating only if the status is unchanged (guards against races / regressions).
func advancePR(ctx context.Context, tx pgx.Tx, prID int64, next func(string) (string, bool)) error {
	var cur string
	if err := tx.QueryRow(ctx, `SELECT status FROM purchase_requests WHERE id = $1`, prID).Scan(&cur); err != nil {
		return err
	}
	target, ok := next(cur)
	if !ok {
		return nil
	}
	_, err := tx.Exec(ctx, `
		UPDATE purchase_requests SET status = $2, updated_at = NOW() WHERE id = $1 AND status = $3`,
		prID, target, cur)
	return err
}

// parseDate converts an optional YYYY-MM-DD string into a value suitable for a
// nullable DATE column (nil for empty/absent).
func parseDate(s *string) (any, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", *s)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// formatDate renders a nullable DATE as a YYYY-MM-DD string (nil when not set).
func formatDate(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	s := d.Time.Format("2006-01-02")
	return &s
}

// =====================================================================
// Purchase-request approvals
// =====================================================================

type PRApproval struct {
	ID                int64        `json:"id"`
	PurchaseRequestID int64        `json:"purchase_request_id"`
	ApproverID        int64        `json:"approver_id"`
	Status            string       `json:"status"` // pending | approved | rejected
	Comment           string       `json:"comment"`
	DecidedAt         *time.Time   `json:"decided_at"`
	CreatedAt         time.Time    `json:"created_at"`
	UpdatedAt         time.Time    `json:"updated_at"`
	Approver          *UserSummary `json:"approver,omitempty"`
}

const prApprovalCols = `pa.id, pa.purchase_request_id, pa.approver_id, pa.status,
	pa.comment, pa.decided_at, pa.created_at, pa.updated_at, u.email, u.name`

func scanApproval(row pgx.Row) (*PRApproval, error) {
	a := &PRApproval{}
	var decided pgtype.Timestamptz
	var email, name pgtype.Text
	if err := row.Scan(&a.ID, &a.PurchaseRequestID, &a.ApproverID, &a.Status,
		&a.Comment, &decided, &a.CreatedAt, &a.UpdatedAt, &email, &name); err != nil {
		return nil, err
	}
	if decided.Valid {
		a.DecidedAt = &decided.Time
	}
	a.Approver = &UserSummary{ID: a.ApproverID, Email: email.String, Name: name.String}
	return a, nil
}

// ListApprovals returns a PR's approvers and their decisions, oldest first.
func (r *Repository) ListApprovals(ctx context.Context, prID int64) ([]PRApproval, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+prApprovalCols+`
		FROM pr_approvals pa JOIN users u ON u.id = pa.approver_id
		WHERE pa.purchase_request_id = $1
		ORDER BY pa.created_at, pa.id`, prID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PRApproval{}
	for rows.Next() {
		a, err := scanApproval(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// GetApproval fetches one approver's row for a PR (pgx.ErrNoRows if absent).
func (r *Repository) GetApproval(ctx context.Context, prID, approverID int64) (*PRApproval, error) {
	return scanApproval(r.pool.QueryRow(ctx, `
		SELECT `+prApprovalCols+`
		FROM pr_approvals pa JOIN users u ON u.id = pa.approver_id
		WHERE pa.purchase_request_id = $1 AND pa.approver_id = $2`, prID, approverID))
}

// AddApprover adds an approver to a PR (idempotent — re-adding an existing
// approver is a no-op that returns the current row).
func (r *Repository) AddApprover(ctx context.Context, prID, approverID int64) (*PRApproval, error) {
	if _, err := r.pool.Exec(ctx, `
		INSERT INTO pr_approvals (purchase_request_id, approver_id)
		VALUES ($1, $2) ON CONFLICT DO NOTHING`, prID, approverID); err != nil {
		return nil, err
	}
	return r.GetApproval(ctx, prID, approverID)
}

// RemoveApprover deletes an approver from a PR. Returns ErrInvalidState when it
// would remove the last approver (every PR must keep at least one).
func (r *Repository) RemoveApprover(ctx context.Context, prID, approverID int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM pr_approvals WHERE purchase_request_id = $1`, prID).Scan(&n); err != nil {
		return err
	}
	if n <= 1 {
		return ErrInvalidState
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM pr_approvals WHERE purchase_request_id = $1 AND approver_id = $2`, prID, approverID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RecordApprovalDecision sets an approver's decision (approved | rejected) with a
// comment. Returns ErrInvalidState when the caller is not an approver on the PR.
func (r *Repository) RecordApprovalDecision(ctx context.Context, prID, approverID int64, decision, comment string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE pr_approvals
		SET status = $3, comment = $4, decided_at = NOW(), updated_at = NOW()
		WHERE purchase_request_id = $1 AND approver_id = $2`, prID, approverID, decision, comment)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrInvalidState
	}
	return nil
}

// RequestApprovalAgain resets a rejected approval back to pending so the approver
// can reconsider. Returns ErrInvalidState if the row is not currently rejected.
func (r *Repository) RequestApprovalAgain(ctx context.Context, prID, approverID int64) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE pr_approvals
		SET status = $3, comment = '', decided_at = NULL, updated_at = NOW()
		WHERE purchase_request_id = $1 AND approver_id = $2 AND status = $4`,
		prID, approverID, model.PRApprovalPending, model.PRApprovalRejected)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrInvalidState
	}
	return nil
}

// ApprovalTally returns the number of approvers on a PR and how many have
// approved, used to surface the approval progress on the request.
func (r *Repository) ApprovalTally(ctx context.Context, prID int64) (total, approved int, err error) {
	err = r.pool.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE status = $2)
		FROM pr_approvals WHERE purchase_request_id = $1`, prID, model.PRApprovalApproved).
		Scan(&total, &approved)
	return total, approved, err
}

// ListActiveUsers returns active users for the approver picker. Caller-agnostic;
// the handler excludes the caller themselves.
func (r *Repository) ListActiveUsers(ctx context.Context) ([]*UserSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, email, name FROM users WHERE is_active ORDER BY lower(email), id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*UserSummary{}
	for rows.Next() {
		u := &UserSummary{}
		var email, name pgtype.Text
		if err := rows.Scan(&u.ID, &email, &name); err != nil {
			return nil, err
		}
		u.Email = email.String
		u.Name = name.String
		out = append(out, u)
	}
	return out, rows.Err()
}

// IsApprover reports whether userID is an approver on the PR.
func (r *Repository) IsApprover(ctx context.Context, prID, userID int64) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM pr_approvals WHERE purchase_request_id = $1 AND approver_id = $2)`,
		prID, userID).Scan(&exists)
	return exists, err
}
