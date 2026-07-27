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
	// BudgetSteps is the ordered serial chain of budget approval steps — populated
	// only for the budget card. Step 1 (IsBase) is the budget-unit governed base
	// approval; any further steps name their own approver (email-matched). Nil/empty
	// for legal/security cards. The card's Approved/ApprovedBy/ApprovedAt above are a
	// projection of this chain (approved iff every step is approved).
	BudgetSteps []BudgetStep `json:"budget_steps,omitempty"`
}

// BudgetStep is one step in a recommendation's serial budget approval chain. The
// base step (position 1) is governed by the PR's budget unit (its approver fields
// are blank — any qualified budget approver acts on it); additional steps name a
// specific approver by email. Steps are decided one after the other.
type BudgetStep struct {
	ID            int64        `json:"id"`
	Position      int          `json:"position"`
	IsBase        bool         `json:"is_base"`
	ApproverName  string       `json:"approver_name"`
	ApproverEmail string       `json:"approver_email"`
	Decision      string       `json:"decision"` // pending | approved | rejected
	DecidedBy     *int64       `json:"decided_by"`
	Decider       *UserSummary `json:"decider,omitempty"`
	DecidedAt     *time.Time   `json:"decided_at"`
	Comments      []RecComment `json:"comments"`
	// CanDecide / CanManage are per-caller capability flags computed in the handler
	// layer: CanDecide = this caller may approve/reject/reverse this step right now
	// (serial + identity gated); CanManage = procurement may edit/remove this step.
	CanDecide bool `json:"can_decide"`
	CanManage bool `json:"can_manage"`
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
		if t == model.RecApprovalBudget {
			if err := ensureBaseBudgetStepTx(ctx, tx, id); err != nil {
				return nil, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.GetRecommendation(ctx, prID)
}

// ensureBaseBudgetStepTx inserts the base budget step (position 1) for a
// recommendation if it has none yet. The base step is budget-unit governed, so it
// carries no named approver.
func ensureBaseBudgetStepTx(ctx context.Context, tx pgx.Tx, recID int64) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO pr_recommendation_budget_steps (recommendation_id, position)
		SELECT $1, 1
		WHERE NOT EXISTS (SELECT 1 FROM pr_recommendation_budget_steps WHERE recommendation_id = $1)`, recID)
	return err
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
		// Budget steps don't cascade off the approval row, so drop them explicitly.
		if t == model.RecApprovalBudget {
			if _, err := tx.Exec(ctx, `DELETE FROM pr_recommendation_budget_steps WHERE recommendation_id = $1`, recID); err != nil {
				return nil, err
			}
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
		// The budget chain re-opens on edit: reset every step to pending (keeping the
		// steps and their named approvers) and make sure a base step exists.
		if t == model.RecApprovalBudget {
			if err := ensureBaseBudgetStepTx(ctx, tx, recID); err != nil {
				return nil, err
			}
			if _, err := tx.Exec(ctx, `
				UPDATE pr_recommendation_budget_steps
				SET decision = 'pending', decided_by = NULL, decided_at = NULL, updated_at = NOW()
				WHERE recommendation_id = $1`, recID); err != nil {
				return nil, err
			}
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

// AddRecApprovalCard adds (requires) a single approval card to a PR's
// recommendation without disturbing the others. For the budget card it also seeds
// the base budget step. ErrInvalidState if the card is already required,
// pgx.ErrNoRows if the PR has no recommendation.
func (r *Repository) AddRecApprovalCard(ctx context.Context, prID int64, approvalType string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	recID, err := recIDForPRTx(ctx, tx, prID)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO pr_recommendation_approvals (recommendation_id, approval_type)
		VALUES ($1, $2) ON CONFLICT DO NOTHING`, recID, approvalType)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrInvalidState // already required
	}
	if approvalType == model.RecApprovalBudget {
		if err := ensureBaseBudgetStepTx(ctx, tx, recID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// RemoveRecApprovalCard removes a single approval card from a PR's recommendation
// (its comments' documents' stored paths are returned for the handler to unlink);
// for the budget card its steps are removed too. A recommendation must keep at
// least one card, so removing the last one is refused. ErrInvalidState if the card
// is the last remaining one, pgx.ErrNoRows if the card is not required / the PR has
// no recommendation.
func (r *Repository) RemoveRecApprovalCard(ctx context.Context, prID int64, approvalType string) ([]string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	recID, err := recIDForPRTx(ctx, tx, prID)
	if err != nil {
		return nil, err
	}
	var present bool
	var total int
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM pr_recommendation_approvals WHERE recommendation_id = $1 AND approval_type = $2),
		       (SELECT count(*) FROM pr_recommendation_approvals WHERE recommendation_id = $1)`,
		recID, approvalType).Scan(&present, &total); err != nil {
		return nil, err
	}
	if !present {
		return nil, pgx.ErrNoRows
	}
	if total <= 1 {
		return nil, ErrInvalidState // a recommendation must keep at least one card
	}

	cids, err := commentIDsForType(ctx, tx, recID, approvalType)
	if err != nil {
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
	if _, err := tx.Exec(ctx, `DELETE FROM pr_recommendation_comments WHERE recommendation_id = $1 AND approval_type = $2`, recID, approvalType); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM pr_recommendation_approvals WHERE recommendation_id = $1 AND approval_type = $2`, recID, approvalType); err != nil {
		return nil, err
	}
	// Budget steps don't cascade off the approval row.
	if approvalType == model.RecApprovalBudget {
		if _, err := tx.Exec(ctx, `DELETE FROM pr_recommendation_budget_steps WHERE recommendation_id = $1`, recID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return paths, nil
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

	// Comments, grouped onto their card. Budget comments hang off a step (loaded
	// below with the chain), so exclude them here.
	crows, err := r.pool.Query(ctx, `
		SELECT c.id, c.approval_type, c.author_id, c.comment, c.created_at, u.email, u.name
		FROM pr_recommendation_comments c
		JOIN users u ON u.id = c.author_id
		WHERE c.recommendation_id = $1 AND c.budget_step_id IS NULL
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

	// The budget card carries the serial approval chain.
	if i, ok := idxByType[model.RecApprovalBudget]; ok {
		steps, err := r.budgetStepsForRec(ctx, rec.ID)
		if err != nil {
			return nil, err
		}
		rec.Approvals[i].BudgetSteps = steps
	}
	return rec, nil
}

// budgetStepsForRec loads a recommendation's ordered budget approval steps, each
// with its decider identity and comment thread (with documents). CanDecide /
// CanManage are left false — the handler fills them per caller.
func (r *Repository) budgetStepsForRec(ctx context.Context, recID int64) ([]BudgetStep, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT s.id, s.position, s.approver_name, s.approver_email, s.decision,
		       s.decided_by, s.decided_at, u.email, u.name
		FROM pr_recommendation_budget_steps s
		LEFT JOIN users u ON u.id = s.decided_by
		WHERE s.recommendation_id = $1
		ORDER BY s.position`, recID)
	if err != nil {
		return nil, err
	}
	var steps []BudgetStep
	idxByID := map[int64]int{}
	for rows.Next() {
		var s BudgetStep
		var decidedBy pgtype.Int8
		var decidedAt pgtype.Timestamptz
		var email, name pgtype.Text
		if err := rows.Scan(&s.ID, &s.Position, &s.ApproverName, &s.ApproverEmail, &s.Decision,
			&decidedBy, &decidedAt, &email, &name); err != nil {
			rows.Close()
			return nil, err
		}
		s.IsBase = s.Position == 1
		if decidedBy.Valid {
			s.DecidedBy = &decidedBy.Int64
			s.Decider = &UserSummary{ID: decidedBy.Int64, Email: email.String, Name: name.String}
		}
		if decidedAt.Valid {
			t := decidedAt.Time
			s.DecidedAt = &t
		}
		s.Comments = []RecComment{}
		steps = append(steps, s)
		idxByID[s.ID] = len(steps) - 1
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Per-step comments (with documents), grouped onto their step.
	crows, err := r.pool.Query(ctx, `
		SELECT c.id, c.budget_step_id, c.author_id, c.comment, c.created_at, u.email, u.name
		FROM pr_recommendation_comments c
		JOIN users u ON u.id = c.author_id
		WHERE c.recommendation_id = $1 AND c.budget_step_id IS NOT NULL
		ORDER BY c.created_at, c.id`, recID)
	if err != nil {
		return nil, err
	}
	type pendingComment struct {
		stepID int64
		c      RecComment
	}
	var pending []pendingComment
	for crows.Next() {
		var stepID int64
		var c RecComment
		var email, name pgtype.Text
		if err := crows.Scan(&c.ID, &stepID, &c.AuthorID, &c.Comment, &c.CreatedAt, &email, &name); err != nil {
			crows.Close()
			return nil, err
		}
		c.Author = &UserSummary{ID: c.AuthorID, Email: email.String, Name: name.String}
		c.Documents = []Document{}
		pending = append(pending, pendingComment{stepID, c})
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
		if i, ok := idxByID[pc.stepID]; ok {
			steps[i].Comments = append(steps[i].Comments, pc.c)
		}
	}
	return steps, nil
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

// =====================================================================
// Budget approval chain (serial steps on the budget card)
// =====================================================================

// AddBudgetStep appends a new named budget approval step to a PR's recommendation
// (position = current max + 1). Requires the recommendation to have a budget card.
// The new step is pending, so the projected budget card falls back to unapproved.
// Returns the created step (id, position) for the handler to notify/record.
func (r *Repository) AddBudgetStep(ctx context.Context, prID int64, name, email string) (*BudgetStep, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	recID, err := recIDForPRTx(ctx, tx, prID)
	if err != nil {
		return nil, err
	}
	var hasBudget bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM pr_recommendation_approvals WHERE recommendation_id = $1 AND approval_type = 'budget')`,
		recID).Scan(&hasBudget); err != nil {
		return nil, err
	}
	if !hasBudget {
		return nil, ErrInvalidState
	}
	s := &BudgetStep{ApproverName: name, ApproverEmail: email, Decision: "pending", Comments: []RecComment{}}
	if err := tx.QueryRow(ctx, `
		INSERT INTO pr_recommendation_budget_steps (recommendation_id, position, approver_name, approver_email)
		VALUES ($1, (SELECT COALESCE(MAX(position), 0) + 1 FROM pr_recommendation_budget_steps WHERE recommendation_id = $1), $2, $3)
		RETURNING id, position`, recID, name, email).Scan(&s.ID, &s.Position); err != nil {
		return nil, err
	}
	if err := syncBudgetApprovalFromStepsTx(ctx, tx, recID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

// UpdateBudgetStep edits an additional step's named approver. Only a still-pending,
// non-base step may be edited (ErrInvalidState otherwise).
func (r *Repository) UpdateBudgetStep(ctx context.Context, prID, stepID int64, name, email string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE pr_recommendation_budget_steps
		SET approver_name = $3, approver_email = $4, updated_at = NOW()
		WHERE id = $2
		  AND recommendation_id = (SELECT id FROM pr_recommendations WHERE purchase_request_id = $1)
		  AND position > 1 AND decision = 'pending'`, prID, stepID, name, email)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrInvalidState
	}
	return nil
}

// DeleteBudgetStep removes an additional (non-base), still-pending step and
// resequences the remaining steps' positions, then re-syncs the projected budget
// card. ErrInvalidState if the step is the base step, already decided, or unknown.
func (r *Repository) DeleteBudgetStep(ctx context.Context, prID, stepID int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	recID, err := recIDForPRTx(ctx, tx, prID)
	if err != nil {
		return err
	}
	var position int
	if err := tx.QueryRow(ctx, `
		DELETE FROM pr_recommendation_budget_steps
		WHERE id = $1 AND recommendation_id = $2 AND position > 1 AND decision = 'pending'
		RETURNING position`, stepID, recID).Scan(&position); err != nil {
		if err == pgx.ErrNoRows {
			return ErrInvalidState
		}
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE pr_recommendation_budget_steps
		SET position = position - 1, updated_at = NOW()
		WHERE recommendation_id = $1 AND position > $2`, recID, position); err != nil {
		return err
	}
	if err := syncBudgetApprovalFromStepsTx(ctx, tx, recID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SetBudgetStepDecision records (or reverses) a step's decision, enforcing the
// serial chain: the previous step must be approved, and the next step must still
// be pending (a step locks once the following one has decided). decision is one of
// "approve" | "reject" | "revert". It then re-syncs the projected budget card.
// Returns the updated step. ErrInvalidState on a serial-order violation.
func (r *Repository) SetBudgetStepDecision(ctx context.Context, prID, stepID int64, decision string, deciderID int64) (*BudgetStep, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	recID, err := recIDForPRTx(ctx, tx, prID)
	if err != nil {
		return nil, err
	}

	var position int
	if err := tx.QueryRow(ctx, `
		SELECT position FROM pr_recommendation_budget_steps WHERE id = $1 AND recommendation_id = $2`,
		stepID, recID).Scan(&position); err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrInvalidState
		}
		return nil, err
	}

	// Previous step (if any) must be approved.
	if position > 1 {
		var prevApproved bool
		if err := tx.QueryRow(ctx, `
			SELECT decision = 'approved' FROM pr_recommendation_budget_steps
			WHERE recommendation_id = $1 AND position = $2`, recID, position-1).Scan(&prevApproved); err != nil {
			return nil, err
		}
		if !prevApproved {
			return nil, ErrInvalidState
		}
	}
	// Next step (if any) must still be pending — this step locks once it has decided.
	var nextDecided bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM pr_recommendation_budget_steps
		              WHERE recommendation_id = $1 AND position = $2 AND decision <> 'pending')`,
		recID, position+1).Scan(&nextDecided); err != nil {
		return nil, err
	}
	if nextDecided {
		return nil, ErrInvalidState
	}

	var newDecision string
	var decidedBy *int64
	switch decision {
	case "approve":
		newDecision, decidedBy = "approved", &deciderID
	case "reject":
		newDecision, decidedBy = "rejected", &deciderID
	case "revert":
		newDecision, decidedBy = "pending", nil
	default:
		return nil, ErrInvalidState
	}
	if _, err := tx.Exec(ctx, `
		UPDATE pr_recommendation_budget_steps
		SET decision = $2,
		    decided_by = $3,
		    decided_at = CASE WHEN $2 = 'pending' THEN NULL ELSE NOW() END,
		    updated_at = NOW()
		WHERE id = $1`, stepID, newDecision, decidedBy); err != nil {
		return nil, err
	}
	if err := syncBudgetApprovalFromStepsTx(ctx, tx, recID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &BudgetStep{ID: stepID, Position: position, IsBase: position == 1, Decision: newDecision}, nil
}

// syncBudgetApprovalFromStepsTx recomputes the budget card's approved_by/approved_at
// projection from its chain: approved (stamped with the last step's decider/time)
// iff there is ≥1 step and every step is approved; otherwise pending. Because steps
// are serial, "all approved" == "last step approved".
func syncBudgetApprovalFromStepsTx(ctx context.Context, tx pgx.Tx, recID int64) error {
	var allApproved bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM pr_recommendation_budget_steps WHERE recommendation_id = $1)
		   AND NOT EXISTS(SELECT 1 FROM pr_recommendation_budget_steps WHERE recommendation_id = $1 AND decision <> 'approved')`,
		recID).Scan(&allApproved); err != nil {
		return err
	}
	if allApproved {
		_, err := tx.Exec(ctx, `
			UPDATE pr_recommendation_approvals a
			SET approved_by = s.decided_by, approved_at = s.decided_at
			FROM (SELECT decided_by, decided_at FROM pr_recommendation_budget_steps
			      WHERE recommendation_id = $1 ORDER BY position DESC LIMIT 1) s
			WHERE a.recommendation_id = $1 AND a.approval_type = 'budget'`, recID)
		return err
	}
	_, err := tx.Exec(ctx, `
		UPDATE pr_recommendation_approvals
		SET approved_by = NULL, approved_at = NULL
		WHERE recommendation_id = $1 AND approval_type = 'budget'`, recID)
	return err
}

// recIDForPRTx resolves a PR's recommendation id within a transaction.
func recIDForPRTx(ctx context.Context, tx pgx.Tx, prID int64) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `SELECT id FROM pr_recommendations WHERE purchase_request_id = $1`, prID).Scan(&id)
	return id, err
}

// AddRecComment appends a comment to an approval card. When budgetStepID is
// non-nil the comment is attached to that budget step (approvalType must be
// "budget" and the step must belong to this PR's recommendation). Returns
// ErrInvalidState if the card is not a required one / the step does not belong.
func (r *Repository) AddRecComment(ctx context.Context, prID int64, approvalType string, budgetStepID *int64, authorID int64, comment string) (*RecComment, error) {
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
	if budgetStepID != nil {
		var belongs bool
		if err := r.pool.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM pr_recommendation_budget_steps WHERE id = $1 AND recommendation_id = $2)`,
			*budgetStepID, recID).Scan(&belongs); err != nil {
			return nil, err
		}
		if !belongs {
			return nil, ErrInvalidState
		}
	}
	c := &RecComment{Documents: []Document{}}
	if err := r.pool.QueryRow(ctx, `
		INSERT INTO pr_recommendation_comments (recommendation_id, approval_type, budget_step_id, author_id, comment)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, author_id, comment, created_at`,
		recID, approvalType, budgetStepID, authorID, comment).
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
// for the PR's recommendation: their email is a member of the PR's free-text,
// comma-separated budget_approver_email list (case-insensitive). This is the
// approver the requester picked from the business unit's approver list.
func (r *Repository) IsBudgetApproverForPR(ctx context.Context, prID, userID int64) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM purchase_requests pr
			JOIN users cu ON cu.id = $2
			WHERE pr.id = $1
			  AND cu.email <> '' AND lower(cu.email) = ANY (
				SELECT lower(trim(e)) FROM unnest(string_to_array(pr.budget_approver_email, ',')) AS e WHERE trim(e) <> '')
		)`, prID, userID).Scan(&ok)
	return ok, err
}

// BudgetApproversForPR returns all qualified budget approvers for the PR (the
// users whose email matches the PR's budget_approver_email list), for
// notification.
func (r *Repository) BudgetApproversForPR(ctx context.Context, prID int64) ([]*UserSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT u.id, u.email, u.name
		FROM purchase_requests pr
		JOIN users u ON u.email <> '' AND lower(u.email) = ANY (
			SELECT lower(trim(e)) FROM unnest(string_to_array(pr.budget_approver_email, ',')) AS e WHERE trim(e) <> '')
		WHERE pr.id = $1`, prID)
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
