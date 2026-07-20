package repository

import (
	"context"
	"time"

	"github.com/cs/purchasing-app/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const ownerRecommendationComment = model.OwnerRecommendationComment

// =====================================================================
// Procurement recommendations
// =====================================================================

// Recommendation is procurement's proposal to proceed with a vendor on a PR, plus
// the required approval cards (budget owner / legal / security) and their
// comments. One per purchase request.
type Recommendation struct {
	ID                int64   `json:"id"`
	PurchaseRequestID int64   `json:"purchase_request_id"`
	VendorID          int64   `json:"vendor_id"`
	Vendor            *Vendor `json:"vendor,omitempty"`
	Description       string  `json:"description"`
	// Commercial details, mirroring the PR's commercial section — procurement may
	// refine these on the recommendation independently of the PR's figures.
	EstimatedValue float64       `json:"estimated_value"`
	Currency       string        `json:"currency"`
	EngagementType string        `json:"engagement_type"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
	Approvals      []RecApproval `json:"approvals"`
	// MyActionableTypes is the subset of required approval types the calling user
	// may act on (toggle/comment). Computed in the handler layer, not here.
	MyActionableTypes []string `json:"my_actionable_types"`
	// ContractID / Contract are the contract attached to this recommendation (the
	// optional contract card), or nil. Loaded on detail reads.
	ContractID *int64    `json:"contract_id"`
	Contract   *Contract `json:"contract,omitempty"`
	// RFIDescription / RFIDocuments are the optional RFI (request for information)
	// raised with the vendor: a description plus PDF attachments. The RFI is
	// "present" when the description is non-empty or there is ≥1 attachment.
	RFIDescription string     `json:"rfi_description"`
	RFIDocuments   []Document `json:"rfi_documents"`
}

// RecApproval is one required approval card: its type, whether it is approved
// (and by whom), its assignee (legal/security only), and the comments left on it.
type RecApproval struct {
	ApprovalType string       `json:"approval_type"`
	Approved     bool         `json:"approved"`
	ApprovedBy   *int64       `json:"approved_by"`
	Approver     *UserSummary `json:"approver,omitempty"`
	ApprovedAt   *time.Time   `json:"approved_at"`
	// AssigneeID / Assignee is the team member responsible for this card
	// (legal/security). Only the assignee may approve; the budget card has none.
	AssigneeID *int64       `json:"assignee_id"`
	Assignee   *UserSummary `json:"assignee,omitempty"`
	Comments   []RecComment `json:"comments"`
	// CanComment / CanApprove / CanAssign are per-caller capability flags computed
	// in the handler layer (not persisted), so the UI shows only allowed controls.
	CanComment bool `json:"can_comment"`
	CanApprove bool `json:"can_approve"`
	CanAssign  bool `json:"can_assign"`
}

// RecComment is one comment on an approval card, with optional document attachments.
type RecComment struct {
	ID        int64        `json:"id"`
	AuthorID  int64        `json:"author_id"`
	Author    *UserSummary `json:"author,omitempty"`
	Comment   string       `json:"comment"`
	CreatedAt time.Time    `json:"created_at"`
	Documents []Document   `json:"documents"`
}

// RecommendationInput carries the writable fields of a recommendation for
// create/update (the vendor, description, commercial details and required cards).
type RecommendationInput struct {
	VendorID       int64
	Description    string
	EstimatedValue float64
	Currency       string
	EngagementType string
	RequiredTypes  []string
}

// CreateRecommendation inserts a recommendation and its required approval cards
// (each pending). Fails if the PR already has one (enforced by the UNIQUE index).
func (r *Repository) CreateRecommendation(ctx context.Context, prID int64, in RecommendationInput, createdBy int64) (*Recommendation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var id int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO pr_recommendations
			(purchase_request_id, vendor_id, description, estimated_value, currency, engagement_type, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		prID, in.VendorID, in.Description, in.EstimatedValue, in.Currency, in.EngagementType, createdBy).Scan(&id); err != nil {
		return nil, err
	}
	for _, t := range in.RequiredTypes {
		if _, err := tx.Exec(ctx, `
			INSERT INTO pr_recommendation_approvals (recommendation_id, approval_type)
			VALUES ($1, $2) ON CONFLICT DO NOTHING`, id, t); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.GetRecommendation(ctx, prID)
}

// UpdateRecommendation replaces the vendor, description and required approval set
// of a PR's recommendation, resetting every card back to pending (an edit
// re-opens the recommendation for fresh sign-off). Cards no longer required have
// their comments removed; the stored paths of those comments' documents are
// returned so the handler can unlink the files after commit.
func (r *Repository) UpdateRecommendation(ctx context.Context, prID int64, in RecommendationInput) ([]string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var recID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM pr_recommendations WHERE purchase_request_id = $1`, prID).Scan(&recID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE pr_recommendations
		SET vendor_id = $2, description = $3, estimated_value = $4, currency = $5, engagement_type = $6, updated_at = NOW()
		WHERE id = $1`,
		recID, in.VendorID, in.Description, in.EstimatedValue, in.Currency, in.EngagementType); err != nil {
		return nil, err
	}

	wanted := map[string]bool{}
	for _, t := range in.RequiredTypes {
		wanted[t] = true
	}

	// Find the cards being dropped, and collect their comments' document paths.
	rows, err := tx.Query(ctx, `SELECT approval_type FROM pr_recommendation_approvals WHERE recommendation_id = $1`, recID)
	if err != nil {
		return nil, err
	}
	var existing []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			rows.Close()
			return nil, err
		}
		existing = append(existing, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var paths []string
	for _, t := range existing {
		if wanted[t] {
			continue
		}
		// Dropped card: unlink each of its comments' documents, then the comments.
		cids, err := commentIDsForType(ctx, tx, recID, t)
		if err != nil {
			return nil, err
		}
		for _, cid := range cids {
			p, err := deleteOwnedDocsTx(ctx, tx, ownerRecommendationComment, cid)
			if err != nil {
				return nil, err
			}
			paths = append(paths, p...)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM pr_recommendation_comments WHERE recommendation_id = $1 AND approval_type = $2`, recID, t); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM pr_recommendation_approvals WHERE recommendation_id = $1 AND approval_type = $2`, recID, t); err != nil {
			return nil, err
		}
	}

	// Upsert each wanted card, resetting it to pending.
	for _, t := range in.RequiredTypes {
		if _, err := tx.Exec(ctx, `
			INSERT INTO pr_recommendation_approvals (recommendation_id, approval_type)
			VALUES ($1, $2)
			ON CONFLICT (recommendation_id, approval_type)
			DO UPDATE SET approved_by = NULL, approved_at = NULL`, recID, t); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return paths, nil
}

func commentIDsForType(ctx context.Context, tx pgx.Tx, recID int64, approvalType string) ([]int64, error) {
	rows, err := tx.Query(ctx, `SELECT id FROM pr_recommendation_comments WHERE recommendation_id = $1 AND approval_type = $2`, recID, approvalType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// DeleteRecommendation removes a PR's recommendation (cards + comments cascade),
// returning the stored paths of its comments' documents for the handler to unlink.
func (r *Repository) DeleteRecommendation(ctx context.Context, prID int64) ([]string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var recID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM pr_recommendations WHERE purchase_request_id = $1`, prID).Scan(&recID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT id FROM pr_recommendation_comments WHERE recommendation_id = $1`, recID)
	if err != nil {
		return nil, err
	}
	var cids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		cids = append(cids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var paths []string
	for _, cid := range cids {
		p, err := deleteOwnedDocsTx(ctx, tx, ownerRecommendationComment, cid)
		if err != nil {
			return nil, err
		}
		paths = append(paths, p...)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM pr_recommendations WHERE id = $1`, recID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return paths, nil
}

// GetRecommendation loads a PR's recommendation with its vendor, approval cards
// and per-card comments (with documents). Returns (nil, nil) when the PR has none.
func (r *Repository) GetRecommendation(ctx context.Context, prID int64) (*Recommendation, error) {
	rec := &Recommendation{RFIDocuments: []Document{}}
	var contractID pgtype.Int8
	err := r.pool.QueryRow(ctx, `
		SELECT id, purchase_request_id, vendor_id, description, estimated_value, currency, engagement_type,
		       contract_id, rfi_description, created_at, updated_at
		FROM pr_recommendations WHERE purchase_request_id = $1`, prID).
		Scan(&rec.ID, &rec.PurchaseRequestID, &rec.VendorID, &rec.Description,
			&rec.EstimatedValue, &rec.Currency, &rec.EngagementType,
			&contractID, &rec.RFIDescription, &rec.CreatedAt, &rec.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if rec.Vendor, err = r.GetVendor(ctx, rec.VendorID); err != nil {
		return nil, err
	}
	if contractID.Valid {
		rec.ContractID = &contractID.Int64
		if rec.Contract, err = r.GetContract(ctx, contractID.Int64); err != nil {
			return nil, err
		}
	}
	if rec.RFIDocuments, err = r.ListOwnedDocuments(ctx, model.OwnerRecommendationRFI, rec.ID); err != nil {
		return nil, err
	}

	// Approval cards, with the approver's identity when approved and the assignee's
	// identity when set.
	rows, err := r.pool.Query(ctx, `
		SELECT a.approval_type, a.approved_by, a.approved_at, u.email, u.name,
		       a.assignee_id, au.email, au.name
		FROM pr_recommendation_approvals a
		LEFT JOIN users u ON u.id = a.approved_by
		LEFT JOIN users au ON au.id = a.assignee_id
		WHERE a.recommendation_id = $1
		ORDER BY a.approval_type`, rec.ID)
	if err != nil {
		return nil, err
	}
	idxByType := map[string]int{}
	for rows.Next() {
		var a RecApproval
		var approvedBy pgtype.Int8
		var approvedAt pgtype.Timestamptz
		var email, name pgtype.Text
		var assigneeID pgtype.Int8
		var assigneeEmail, assigneeName pgtype.Text
		if err := rows.Scan(&a.ApprovalType, &approvedBy, &approvedAt, &email, &name,
			&assigneeID, &assigneeEmail, &assigneeName); err != nil {
			rows.Close()
			return nil, err
		}
		if approvedBy.Valid {
			a.Approved = true
			a.ApprovedBy = &approvedBy.Int64
			a.Approver = &UserSummary{ID: approvedBy.Int64, Email: email.String, Name: name.String}
		}
		if approvedAt.Valid {
			t := approvedAt.Time
			a.ApprovedAt = &t
		}
		if assigneeID.Valid {
			a.AssigneeID = &assigneeID.Int64
			a.Assignee = &UserSummary{ID: assigneeID.Int64, Email: assigneeEmail.String, Name: assigneeName.String}
		}
		a.Comments = []RecComment{}
		rec.Approvals = append(rec.Approvals, a)
		idxByType[a.ApprovalType] = len(rec.Approvals) - 1
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Comments, grouped onto their card.
	crows, err := r.pool.Query(ctx, `
		SELECT c.id, c.approval_type, c.author_id, c.comment, c.created_at, u.email, u.name
		FROM pr_recommendation_comments c
		JOIN users u ON u.id = c.author_id
		WHERE c.recommendation_id = $1
		ORDER BY c.created_at, c.id`, rec.ID)
	if err != nil {
		return nil, err
	}
	type pendingComment struct {
		approvalType string
		c            RecComment
	}
	var pending []pendingComment
	for crows.Next() {
		var approvalType string
		var c RecComment
		var email, name pgtype.Text
		if err := crows.Scan(&c.ID, &approvalType, &c.AuthorID, &c.Comment, &c.CreatedAt, &email, &name); err != nil {
			crows.Close()
			return nil, err
		}
		c.Author = &UserSummary{ID: c.AuthorID, Email: email.String, Name: name.String}
		c.Documents = []Document{}
		pending = append(pending, pendingComment{approvalType, c})
	}
	crows.Close()
	if err := crows.Err(); err != nil {
		return nil, err
	}
	for _, pc := range pending {
		docs, err := r.ListOwnedDocuments(ctx, ownerRecommendationComment, pc.c.ID)
		if err != nil {
			return nil, err
		}
		pc.c.Documents = docs
		// Write back by index — the cards slice is fully built now, so indexing is
		// stable (capturing &rec.Approvals[i] during the append loop above would
		// dangle once the slice reallocated, dropping earlier cards' comments).
		if i, ok := idxByType[pc.approvalType]; ok {
			rec.Approvals[i].Comments = append(rec.Approvals[i].Comments, pc.c)
		}
	}
	return rec, nil
}

// CreateRecommendationContract drafts the contract attached to a PR's
// recommendation card. It uses the recommended vendor and that vendor's
// quotation on the PR for the amount/currency, takes the given text as the
// contract terms, links the new contract back onto the recommendation, marks the
// quotation selected and advances the PR to contract_prepared. The PDF is
// uploaded separately against the returned contract. No approval gate — the card
// may be filled in at any time. Returns ErrInvalidState if the recommendation
// already has a contract.
func (r *Repository) CreateRecommendationContract(ctx context.Context, prID int64, terms string, createdBy int64) (*Contract, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var recID, vendorID int64
	var existing pgtype.Int8
	if err := tx.QueryRow(ctx, `
		SELECT id, vendor_id, contract_id FROM pr_recommendations WHERE purchase_request_id = $1`, prID).
		Scan(&recID, &vendorID, &existing); err != nil {
		return nil, err
	}
	if existing.Valid {
		return nil, ErrInvalidState
	}

	// The recommended vendor's quotation on this PR sources the amount/currency
	// (a recommendation may only name a vendor that has quoted).
	var quotationID int64
	var total float64
	var currency string
	if err := tx.QueryRow(ctx, `
		SELECT id, total_amount, currency FROM quotations
		WHERE purchase_request_id = $1 AND vendor_id = $2
		ORDER BY created_at DESC, id DESC LIMIT 1`, prID, vendorID).
		Scan(&quotationID, &total, &currency); err != nil {
		return nil, err
	}

	var title string
	if err := tx.QueryRow(ctx, `SELECT title FROM purchase_requests WHERE id = $1`, prID).Scan(&title); err != nil {
		return nil, err
	}

	var contractID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO contracts (purchase_request_id, quotation_id, vendor_id, title, total_amount, currency, terms, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
		prID, quotationID, vendorID, title, total, currency, terms, createdBy).Scan(&contractID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE pr_recommendations SET contract_id = $2, updated_at = NOW() WHERE id = $1`, recID, contractID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE quotations SET status = $2, updated_at = NOW()
		WHERE id = $1 AND status <> $3`, quotationID, model.QuoSelected, model.QuoRejected); err != nil {
		return nil, err
	}
	if err := advancePR(ctx, tx, prID, model.NextPRStatusForContractCreated); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.GetContract(ctx, contractID)
}

// DeleteRecommendationContract removes the contract attached to a PR's
// recommendation and unlinks it. Only a still-draft contract may be removed
// (once it has entered review/signing it is managed on the contract page). It
// deletes the contract's documents (returning their stored paths for the handler
// to unlink), deletes the contract row — which clears pr_recommendations.
// contract_id via ON DELETE SET NULL — and rewinds the PR from contract_prepared
// back to vendor_selected (the selected quotation remains). Returns
// ErrInvalidState if the contract is past draft, pgx.ErrNoRows if there is none.
func (r *Repository) DeleteRecommendationContract(ctx context.Context, prID int64) ([]string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var contractID pgtype.Int8
	if err := tx.QueryRow(ctx, `SELECT contract_id FROM pr_recommendations WHERE purchase_request_id = $1`, prID).Scan(&contractID); err != nil {
		return nil, err
	}
	if !contractID.Valid {
		return nil, pgx.ErrNoRows
	}
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM contracts WHERE id = $1`, contractID.Int64).Scan(&status); err != nil {
		return nil, err
	}
	if status != model.ContractDraft {
		return nil, ErrInvalidState
	}
	paths, err := deleteOwnedDocsTx(ctx, tx, model.OwnerContract, contractID.Int64)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM contracts WHERE id = $1`, contractID.Int64); err != nil {
		return nil, err
	}
	// Rewind the PR only if it is still sitting at the state contract creation put
	// it in; never stomp a status that has since moved on.
	if _, err := tx.Exec(ctx, `
		UPDATE purchase_requests SET status = $2, updated_at = NOW()
		WHERE id = $1 AND status = $3`, prID, model.StatusVendorSelected, model.StatusContractPrepared); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return paths, nil
}

// SetRecommendationRFIDescription sets the RFI description on a PR's
// recommendation. ErrNoRows if the PR has no recommendation.
func (r *Repository) SetRecommendationRFIDescription(ctx context.Context, prID int64, description string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE pr_recommendations SET rfi_description = $2, updated_at = NOW()
		WHERE purchase_request_id = $1`, prID, description)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// ClearRecommendationRFI removes a PR's RFI entirely: it blanks the description
// and deletes every RFI attachment, returning the stored paths of those
// documents so the handler can unlink the files after commit.
func (r *Repository) ClearRecommendationRFI(ctx context.Context, prID int64) ([]string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var recID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM pr_recommendations WHERE purchase_request_id = $1`, prID).Scan(&recID); err != nil {
		return nil, err
	}
	paths, err := deleteOwnedDocsTx(ctx, tx, model.OwnerRecommendationRFI, recID)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE pr_recommendations SET rfi_description = '', updated_at = NOW() WHERE id = $1`, recID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return paths, nil
}

// recIDForPR resolves a PR's recommendation id (pgx.ErrNoRows if it has none).
func (r *Repository) recIDForPR(ctx context.Context, prID int64) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx, `SELECT id FROM pr_recommendations WHERE purchase_request_id = $1`, prID).Scan(&id)
	return id, err
}

// SetRecApproval marks an approval card approved by the given user (the approve
// toggle, on). Returns ErrInvalidState if the card is not a required one.
func (r *Repository) SetRecApproval(ctx context.Context, prID int64, approvalType string, approverID int64) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE pr_recommendation_approvals
		SET approved_by = $3, approved_at = NOW()
		WHERE recommendation_id = (SELECT id FROM pr_recommendations WHERE purchase_request_id = $1)
		  AND approval_type = $2`, prID, approvalType, approverID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrInvalidState
	}
	return nil
}

// ClearRecApproval reverts an approval card to pending (the approve toggle, off).
func (r *Repository) ClearRecApproval(ctx context.Context, prID int64, approvalType string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE pr_recommendation_approvals
		SET approved_by = NULL, approved_at = NULL
		WHERE recommendation_id = (SELECT id FROM pr_recommendations WHERE purchase_request_id = $1)
		  AND approval_type = $2`, prID, approvalType)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrInvalidState
	}
	return nil
}

// SetRecAssignee sets (or clears, when assigneeID is nil) the assignee of a
// legal/security approval card. Returns ErrInvalidState if the card is not a
// required one on this PR's recommendation.
func (r *Repository) SetRecAssignee(ctx context.Context, prID int64, approvalType string, assigneeID *int64) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE pr_recommendation_approvals
		SET assignee_id = $3
		WHERE recommendation_id = (SELECT id FROM pr_recommendations WHERE purchase_request_id = $1)
		  AND approval_type = $2`, prID, approvalType, assigneeID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrInvalidState
	}
	return nil
}

// AddRecComment appends a comment to an approval card. Returns ErrInvalidState if
// the card is not a required one.
func (r *Repository) AddRecComment(ctx context.Context, prID int64, approvalType string, authorID int64, comment string) (*RecComment, error) {
	recID, err := r.recIDForPR(ctx, prID)
	if err != nil {
		return nil, err
	}
	var required bool
	if err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM pr_recommendation_approvals WHERE recommendation_id = $1 AND approval_type = $2)`,
		recID, approvalType).Scan(&required); err != nil {
		return nil, err
	}
	if !required {
		return nil, ErrInvalidState
	}
	c := &RecComment{Documents: []Document{}}
	if err := r.pool.QueryRow(ctx, `
		INSERT INTO pr_recommendation_comments (recommendation_id, approval_type, author_id, comment)
		VALUES ($1, $2, $3, $4)
		RETURNING id, author_id, comment, created_at`,
		recID, approvalType, authorID, comment).
		Scan(&c.ID, &c.AuthorID, &c.Comment, &c.CreatedAt); err != nil {
		return nil, err
	}
	return c, nil
}

// GetRecComment fetches one comment of a PR's recommendation (used to scope its
// document operations). pgx.ErrNoRows if it does not belong to this PR.
func (r *Repository) GetRecComment(ctx context.Context, prID, commentID int64) (*RecComment, error) {
	c := &RecComment{Documents: []Document{}}
	err := r.pool.QueryRow(ctx, `
		SELECT c.id, c.author_id, c.comment, c.created_at
		FROM pr_recommendation_comments c
		JOIN pr_recommendations r ON r.id = c.recommendation_id
		WHERE c.id = $1 AND r.purchase_request_id = $2`, commentID, prID).
		Scan(&c.ID, &c.AuthorID, &c.Comment, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// RecommendationFullyApproved reports whether the PR has a recommendation with at
// least one required card and no card still pending — the gate for selecting a
// quotation / drafting a contract.
func (r *Repository) RecommendationFullyApproved(ctx context.Context, prID int64) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, recApprovedSQL, prID).Scan(&ok)
	return ok, err
}

const recApprovedSQL = `
	SELECT
		EXISTS(SELECT 1 FROM pr_recommendation_approvals ra
		       JOIN pr_recommendations r ON r.id = ra.recommendation_id
		       WHERE r.purchase_request_id = $1)
		AND NOT EXISTS(SELECT 1 FROM pr_recommendation_approvals ra
		       JOIN pr_recommendations r ON r.id = ra.recommendation_id
		       WHERE r.purchase_request_id = $1 AND ra.approved_by IS NULL)`

// recommendationFullyApprovedTx is RecommendationFullyApproved within a tx, for
// gating actions that mutate in the same transaction.
func recommendationFullyApprovedTx(ctx context.Context, tx pgx.Tx, prID int64) (bool, error) {
	var ok bool
	err := tx.QueryRow(ctx, recApprovedSQL, prID).Scan(&ok)
	return ok, err
}

// IsBudgetApproverForPR reports whether the user is a qualified budget approver
// for the PR's recommendation: a member of the approver set that the
// recommendation's estimated value + currency resolve to within the PR's budget
// unit (a matching bracket's approvers, else the unit's default approver).
func (r *Repository) IsBudgetApproverForPR(ctx context.Context, prID, userID int64) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM purchase_requests pr
			JOIN pr_recommendations rec ON rec.purchase_request_id = pr.id
			JOIN resolve_budget_approvers(pr.budget_unit_id, rec.estimated_value, rec.currency) rba
			  ON rba.user_id = $2
			WHERE pr.id = $1)`, prID, userID).Scan(&ok)
	return ok, err
}

// BudgetApproversForPR returns all qualified budget approvers for the PR's
// recommendation (a matching bracket's approvers, else the default approver),
// for notification.
func (r *Repository) BudgetApproversForPR(ctx context.Context, prID int64) ([]*UserSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT u.id, u.email, u.name
		FROM purchase_requests pr
		JOIN pr_recommendations rec ON rec.purchase_request_id = pr.id
		JOIN resolve_budget_approvers(pr.budget_unit_id, rec.estimated_value, rec.currency) rba
		  ON TRUE
		JOIN users u ON u.id = rba.user_id
		WHERE pr.id = $1`, prID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUserSummaries(rows)
}

// BudgetApproversForValue resolves the qualified budget approvers for a budget
// unit at a given estimated value + currency, without a recommendation — used
// for the creation-phase preview shown to the requester. When no bracket matches
// (nil value, currency mismatch, or out of range) it yields the default approver.
func (r *Repository) BudgetApproversForValue(ctx context.Context, budgetUnitID int64, value *float64, currency string) ([]*UserSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT u.id, u.email, u.name
		FROM resolve_budget_approvers($1, $2, $3) rba
		JOIN users u ON u.id = rba.user_id`, budgetUnitID, value, currency)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUserSummaries(rows)
}

// scanUserSummaries reads (id, email, name) rows into UserSummary values.
func scanUserSummaries(rows pgx.Rows) ([]*UserSummary, error) {
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
