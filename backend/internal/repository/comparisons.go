package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Quotation comparison — one per purchase request (migration 050). See
// docs/quotation-comparison.md.
//
// The row holds no figures: everything on the comparison card is derived at read
// time from the PR's quotations and their per-document extractions, so a quotation
// edit is reflected instead of leaving a stale snapshot. Stored here is only what
// can't be derived — that a comparison exists, who generated it and when, the
// approved budget to measure against, the currency it is stated in, and the user's
// consent to stand a vendor's initial figures in for a final quote it lacks.
type QuotationComparison struct {
	ID                int64 `json:"id"`
	PurchaseRequestID int64 `json:"purchase_request_id"`
	// ApprovedBudget is the sheet's "Approved budget (if any)" — nil when unknown.
	ApprovedBudget *float64 `json:"approved_budget"`
	Currency       string   `json:"currency"`
	// UseInitialForFinal records that the user confirmed comparing vendors whose
	// final quote is missing using their initial figures.
	UseInitialForFinal bool         `json:"use_initial_for_final"`
	GeneratedAt        time.Time    `json:"generated_at"`
	GeneratedBy        *int64       `json:"generated_by"`
	Generator          *UserSummary `json:"generator,omitempty"`
	UpdatedAt          time.Time    `json:"updated_at"`
}

// QuotationComparisonInput carries the writable fields.
type QuotationComparisonInput struct {
	ApprovedBudget     *float64
	Currency           string
	UseInitialForFinal bool
}

const comparisonSelect = `
	SELECT c.id, c.purchase_request_id, c.approved_budget, c.currency, c.use_initial_for_final,
	       c.generated_at, c.generated_by, c.updated_at,
	       u.id, u.email, COALESCE(u.name, '')
	FROM quotation_comparisons c
	LEFT JOIN users u ON u.id = c.generated_by`

func scanComparison(row pgx.Row) (*QuotationComparison, error) {
	c := &QuotationComparison{}
	var generatedBy, userID *int64
	var email, name *string
	if err := row.Scan(&c.ID, &c.PurchaseRequestID, &c.ApprovedBudget, &c.Currency,
		&c.UseInitialForFinal, &c.GeneratedAt, &generatedBy, &c.UpdatedAt,
		&userID, &email, &name); err != nil {
		return nil, err
	}
	c.GeneratedBy = generatedBy
	if userID != nil {
		c.Generator = &UserSummary{ID: *userID}
		if email != nil {
			c.Generator.Email = *email
		}
		if name != nil {
			c.Generator.Name = *name
		}
	}
	return c, nil
}

// GetQuotationComparison returns a PR's comparison, or (nil, nil) when none has
// been generated — "not generated yet" is the normal state, not an error.
func (r *Repository) GetQuotationComparison(ctx context.Context, prID int64) (*QuotationComparison, error) {
	c, err := scanComparison(r.pool.QueryRow(ctx, comparisonSelect+`
		WHERE c.purchase_request_id = $1`, prID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

// GenerateQuotationComparison creates the PR's comparison, or re-stamps the
// existing one (regenerate). generated_at/by always move to the caller: a
// regenerated comparison is a fresh statement that these are the quotes compared.
func (r *Repository) GenerateQuotationComparison(ctx context.Context, prID int64, in QuotationComparisonInput, userID int64) (*QuotationComparison, error) {
	if _, err := r.pool.Exec(ctx, `
		INSERT INTO quotation_comparisons
			(purchase_request_id, approved_budget, currency, use_initial_for_final, generated_by)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (purchase_request_id) DO UPDATE
		SET approved_budget = EXCLUDED.approved_budget,
		    currency = EXCLUDED.currency,
		    use_initial_for_final = EXCLUDED.use_initial_for_final,
		    generated_at = now(),
		    generated_by = EXCLUDED.generated_by,
		    updated_at = now()`,
		prID, in.ApprovedBudget, in.Currency, in.UseInitialForFinal, userID); err != nil {
		return nil, err
	}
	return r.GetQuotationComparison(ctx, prID)
}

// UpdateQuotationComparison edits an existing comparison's inputs (the approved
// budget, the currency it is stated in, the missing-final-quote consent) without
// re-stamping who generated it. Returns (nil, nil) when there is nothing to update.
func (r *Repository) UpdateQuotationComparison(ctx context.Context, prID int64, in QuotationComparisonInput) (*QuotationComparison, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE quotation_comparisons
		SET approved_budget = $2, currency = $3, use_initial_for_final = $4, updated_at = now()
		WHERE purchase_request_id = $1`,
		prID, in.ApprovedBudget, in.Currency, in.UseInitialForFinal)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, nil
	}
	return r.GetQuotationComparison(ctx, prID)
}

// DeleteQuotationComparison removes a PR's comparison. Deleting only discards the
// generated record — the quotations it compared are untouched.
func (r *Repository) DeleteQuotationComparison(ctx context.Context, prID int64) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM quotation_comparisons WHERE purchase_request_id = $1`, prID)
	return err
}

// QuotationItemsForPR returns the stored line items of every quotation on a PR,
// keyed by quotation id. One query rather than a per-quotation fetch: the
// comparison needs the items of all of them at once, and quotation *summaries*
// (what ListQuotations returns) deliberately carry no items.
func (r *Repository) QuotationItemsForPR(ctx context.Context, prID int64) (map[int64][]QuotationItem, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT qi.quotation_id, qi.id, qi.description, qi.quantity, qi.unit_price, qi.position
		FROM quotation_items qi
		JOIN quotations q ON q.id = qi.quotation_id
		WHERE q.purchase_request_id = $1
		ORDER BY qi.quotation_id, qi.position, qi.id`, prID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[int64][]QuotationItem{}
	for rows.Next() {
		var quotationID int64
		var it QuotationItem
		if err := rows.Scan(&quotationID, &it.ID, &it.Description, &it.Quantity, &it.UnitPrice, &it.Position); err != nil {
			return nil, err
		}
		out[quotationID] = append(out[quotationID], it)
	}
	return out, rows.Err()
}
