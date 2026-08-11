package repository

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Quotation PDF extraction staging. An extraction never writes onto a quotation —
// it records what Claude read out of the PDF so a procurement user can review and
// apply it. See docs/plans/20-quotation-pdf-extraction.md.

// Extraction status values.
const (
	ExtractionPending   = "pending"
	ExtractionSucceeded = "succeeded"
	ExtractionFailed    = "failed"
)

// QuotationExtraction is one attempt at reading a quotation PDF.
//
// QuotationID is null for the pre-create flow (the PDF was uploaded on the PR page
// before the quotation existed); the create call adopts DocumentID into the new
// quotation's initial-PDF slot and marks the row applied.
type QuotationExtraction struct {
	ID                int64              `json:"id"`
	PurchaseRequestID int64              `json:"purchase_request_id"`
	DocumentID        int64              `json:"document_id"`
	QuotationID       *int64             `json:"quotation_id"`
	Status            string             `json:"status"`
	Model             string             `json:"model"`
	RawJSON           json.RawMessage    `json:"raw_json,omitempty"`
	ErrorMessage      string             `json:"error_message,omitempty"`
	InputTokens       int                `json:"input_tokens"`
	OutputTokens      int                `json:"output_tokens"`
	CreatedAt         pgtype.Timestamptz `json:"created_at"`
	AppliedAt         *string            `json:"applied_at"`
	// Filename of the staged document, joined in for display.
	Filename string `json:"filename,omitempty"`
}

const extractionCols = `e.id, e.purchase_request_id, e.document_id, e.quotation_id, e.status,
	e.model, e.raw_json, e.error_message, e.input_tokens, e.output_tokens, e.created_at,
	e.applied_at, COALESCE(d.filename, '')`

func scanExtraction(row pgx.Row) (*QuotationExtraction, error) {
	e := &QuotationExtraction{}
	var quotationID pgtype.Int8
	var raw []byte
	var appliedAt pgtype.Timestamptz
	if err := row.Scan(&e.ID, &e.PurchaseRequestID, &e.DocumentID, &quotationID, &e.Status,
		&e.Model, &raw, &e.ErrorMessage, &e.InputTokens, &e.OutputTokens, &e.CreatedAt,
		&appliedAt, &e.Filename); err != nil {
		return nil, err
	}
	if quotationID.Valid {
		e.QuotationID = &quotationID.Int64
	}
	if len(raw) > 0 {
		e.RawJSON = json.RawMessage(raw)
	}
	if appliedAt.Valid {
		s := appliedAt.Time.Format("2006-01-02T15:04:05Z07:00")
		e.AppliedAt = &s
	}
	return e, nil
}

// CreateExtraction stages a pending extraction for a stored document. Re-running
// against the same document replaces the previous non-failed attempt (the unique
// partial index on document_id), so extraction is idempotent per PDF.
func (r *Repository) CreateExtraction(ctx context.Context, prID, documentID int64, quotationID *int64, model string, createdBy int64) (*QuotationExtraction, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Drop any prior live attempt for this document so the retry doesn't collide
	// with it. Failed rows are left in place as a record of the failure.
	if _, err := tx.Exec(ctx,
		`DELETE FROM quotation_extractions WHERE document_id = $1 AND status <> 'failed'`,
		documentID); err != nil {
		return nil, err
	}
	var id int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO quotation_extractions
			(purchase_request_id, document_id, quotation_id, status, model, created_by)
		VALUES ($1, $2, $3, 'pending', $4, $5) RETURNING id`,
		prID, documentID, quotationID, model, createdBy).Scan(&id); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.GetExtraction(ctx, id)
}

// GetExtraction loads one extraction by id.
func (r *Repository) GetExtraction(ctx context.Context, id int64) (*QuotationExtraction, error) {
	return scanExtraction(r.pool.QueryRow(ctx, `
		SELECT `+extractionCols+`
		FROM quotation_extractions e
		LEFT JOIN documents d ON d.id = e.document_id
		WHERE e.id = $1`, id))
}

// ListSucceededExtractionsForPR returns every live (succeeded) extraction under a
// purchase request, newest first — one per document, since the unique partial index
// on document_id allows only one non-failed row per PDF.
//
// This is what lets the PR page show the extracted details *per PDF slot*: the
// initial and final quotation PDFs are separate documents with separate extractions,
// and their figures legitimately differ. Failed rows are excluded — they carry no
// suggestion to display.
func (r *Repository) ListSucceededExtractionsForPR(ctx context.Context, prID int64) ([]*QuotationExtraction, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+extractionCols+`
		FROM quotation_extractions e
		LEFT JOIN documents d ON d.id = e.document_id
		WHERE e.purchase_request_id = $1 AND e.status = 'succeeded'
		ORDER BY e.created_at DESC, e.id DESC`, prID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []*QuotationExtraction{}
	for rows.Next() {
		e, err := scanExtraction(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// FinishExtraction records a completed attempt. On success rawJSON holds the
// model's validated output; on failure errMsg explains why.
func (r *Repository) FinishExtraction(ctx context.Context, id int64, status string, rawJSON []byte, inputTokens, outputTokens int, errMsg string) error {
	var raw any
	if len(rawJSON) > 0 {
		raw = string(rawJSON)
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE quotation_extractions
		SET status = $2, raw_json = $3, input_tokens = $4, output_tokens = $5, error_message = $6
		WHERE id = $1`,
		id, status, raw, inputTokens, outputTokens, errMsg)
	return err
}

// MarkExtractionApplied stamps an extraction as applied to a quotation — set when
// the user accepts the suggestion (on create, or via the card's Apply).
//
// At most one extraction per quotation is applied at a time: applying one clears the
// stamp on the quotation's others. A quotation holds a single total, so "applied"
// answers "whose numbers is this carrying?" — and two PDFs both claiming it could
// only mislead. Applying the final quotation's read therefore un-applies the
// initial's, which is what actually happened.
func (r *Repository) MarkExtractionApplied(ctx context.Context, id, quotationID, userID int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE quotation_extractions
		SET applied_at = NULL, applied_by = NULL
		WHERE quotation_id = $1 AND id <> $2 AND applied_at IS NOT NULL`,
		quotationID, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE quotation_extractions
		SET quotation_id = $2, applied_at = now(), applied_by = $3
		WHERE id = $1`, id, quotationID, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// TakePendingExtractionDocument returns the staged document id for a succeeded,
// not-yet-applied extraction belonging to prID, or ErrInvalidState if the
// extraction can't be adopted (wrong PR, already applied, or not succeeded).
//
// Used by quotation-create to adopt the already-stored PDF instead of having the
// client re-upload the same bytes.
func (r *Repository) TakePendingExtractionDocument(ctx context.Context, id, prID int64) (int64, error) {
	var docID int64
	var status string
	var applied pgtype.Timestamptz
	var rowPR int64
	err := r.pool.QueryRow(ctx, `
		SELECT document_id, status, applied_at, purchase_request_id
		FROM quotation_extractions WHERE id = $1`, id).
		Scan(&docID, &status, &applied, &rowPR)
	if err != nil {
		return 0, err
	}
	if rowPR != prID || status != ExtractionSucceeded || applied.Valid {
		return 0, ErrInvalidState
	}
	return docID, nil
}

// SetDocumentOwner re-points a document at a different logical owner. Used to
// promote a staging extraction document (owner_type quotation_extraction) onto the
// quotation created from it; the document's purchase_request_id — and so its
// storage location — is unchanged.
func (r *Repository) SetDocumentOwner(ctx context.Context, docID int64, ownerType string, ownerID int64) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE documents SET owner_type = $2, owner_id = $3 WHERE id = $1`,
		docID, ownerType, ownerID)
	return err
}

// VendorMatch is a candidate vendor for an extracted vendor name, with a coarse
// score so the UI can preselect an obvious hit and still show alternatives.
type VendorMatch struct {
	Vendor *Vendor `json:"vendor"`
	// Score: 100 exact (case-insensitive), 80 prefix, 60 substring,
	// 40 shared significant word.
	Score int `json:"score"`
}

// MatchVendors ranks existing vendors against a name read out of a PDF. The model
// only ever returns a name — the user picks the actual vendor, so this is a
// convenience ranking and not an authoritative resolution.
//
// Returns at most limit matches, best first; an empty slice when nothing plausibly
// matches (the UI then offers to create a vendor).
func (r *Repository) MatchVendors(ctx context.Context, name string, limit int) ([]VendorMatch, error) {
	needle := normalizeVendorName(name)
	if needle == "" {
		return []VendorMatch{}, nil
	}
	if limit <= 0 {
		limit = 5
	}
	vendors, err := r.ListVendors(ctx)
	if err != nil {
		return nil, err
	}
	needleWords := significantWords(needle)

	matches := []VendorMatch{}
	for _, v := range vendors {
		hay := normalizeVendorName(v.Name)
		if hay == "" {
			continue
		}
		score := 0
		switch {
		case hay == needle:
			score = 100
		case strings.HasPrefix(hay, needle) || strings.HasPrefix(needle, hay):
			score = 80
		case strings.Contains(hay, needle) || strings.Contains(needle, hay):
			score = 60
		default:
			// Fall back to a shared significant word ("SUSE Software Solutions"
			// vs. "SUSE"), which is the common real-world case.
			for _, w := range significantWords(hay) {
				for _, nw := range needleWords {
					if w == nw {
						score = 40
						break
					}
				}
				if score > 0 {
					break
				}
			}
		}
		if score == 0 {
			continue
		}
		// An inactive vendor is still worth offering, just never above an active one.
		if !v.IsActive {
			score -= 5
		}
		matches = append(matches, VendorMatch{Vendor: v, Score: score})
	}

	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return strings.ToLower(matches[i].Vendor.Name) < strings.ToLower(matches[j].Vendor.Name)
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}
	return matches, nil
}

// vendorNoise are legal-form and punctuation tokens that carry no identifying
// signal when comparing company names.
var vendorNoise = map[string]bool{
	"inc": true, "inc.": true, "llc": true, "ltd": true, "ltd.": true, "limited": true,
	"gmbh": true, "bv": true, "b.v.": true, "nv": true, "sa": true, "s.a.": true,
	"pvt": true, "private": true, "plc": true, "co": true, "co.": true, "corp": true,
	"corporation": true, "company": true, "the": true, "and": true, "&": true,
	"lda": true, "ltda": true, "me": true, "eireli": true, // pt-BR legal forms
}

// normalizeVendorName lowercases, strips punctuation, and collapses whitespace so
// "SUSE Software Solutions, Ltda." and "suse software solutions ltda" compare equal.
//
// Letters and digits are classified by Unicode category, not by ASCII range, so
// accented names ("Patrocínio") keep their letters while non-ASCII punctuation
// (em dashes, typographic quotes) is treated as a separator like any other.
func normalizeVendorName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevSpace := false
	for _, ch := range s {
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) {
			b.WriteRune(ch)
			prevSpace = false
			continue
		}
		if !prevSpace && b.Len() > 0 {
			b.WriteByte(' ')
			prevSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

// significantWords splits a normalized name into tokens worth matching on,
// dropping legal-form noise and very short fragments.
func significantWords(normalized string) []string {
	out := []string{}
	for _, w := range strings.Fields(normalized) {
		if len(w) < 3 || vendorNoise[w] {
			continue
		}
		out = append(out, w)
	}
	return out
}
