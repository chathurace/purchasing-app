package repository_test

import (
	"context"
	"os"
	"testing"

	"github.com/cs/purchasing-app/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

// testDSN is the local dev database. Tests skip if it is unreachable so they
// don't fail in environments without Postgres.
const testDSN = "postgres://chathura@localhost:5432/purchasing?sslmode=disable"

func newTestRepo(t *testing.T) (*repository.Repository, context.Context) {
	t.Helper()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, testDSN)
	if err != nil {
		t.Skipf("no test DB: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("test DB unreachable: %v", err)
	}
	t.Cleanup(pool.Close)
	return repository.New(pool), ctx
}

func TestPurchaseRequestLifecycle(t *testing.T) {
	repo, ctx := newTestRepo(t)

	user, err := repo.UpsertUser(ctx, "test-sub-"+t.Name(), "tester@example.com", "Tester")
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}

	// Default-role provisioning: first call grants staff; it must not duplicate.
	if err := repo.GrantDefaultRoleIfNone(ctx, user.ID, "staff"); err != nil {
		t.Fatalf("grant default role: %v", err)
	}
	roles, err := repo.GetUserRoles(ctx, user.ID)
	if err != nil {
		t.Fatalf("get roles: %v", err)
	}
	if len(roles) != 1 || roles[0] != "staff" {
		t.Fatalf("expected [staff], got %v", roles)
	}

	// Create with items + links.
	pr, err := repo.CreatePurchaseRequest(ctx, user.ID, repository.PurchaseRequestInput{
		Title:    "Monitors",
		Comments: "Need by Q3",
		Items: []repository.Item{
			{Description: "Dell 27\" monitor", Quantity: 5},
			{Description: "HDMI cable", Quantity: 5},
		},
		Links: []repository.Link{{URL: "https://dell.com/quote", Label: "Quote"}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() {
		_, _ = repo.Pool().Exec(context.Background(), `DELETE FROM purchase_requests WHERE id=$1`, pr.ID)
	})

	if pr.Status != "submitted" {
		t.Errorf("expected status submitted, got %q", pr.Status)
	}
	if len(pr.Items) != 2 || len(pr.Links) != 1 {
		t.Fatalf("expected 2 items + 1 link, got %d items %d links", len(pr.Items), len(pr.Links))
	}
	if pr.Requester == nil || pr.Requester.Email != "tester@example.com" {
		t.Errorf("requester not populated: %+v", pr.Requester)
	}

	// Update: replace items/links.
	if err := repo.UpdatePurchaseRequest(ctx, pr.ID, repository.PurchaseRequestInput{
		Title:    "Monitors (revised)",
		Comments: "Updated",
		Items:    []repository.Item{{Description: "Dell 32\" monitor", Quantity: 3}},
		Links:    nil,
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := repo.GetPurchaseRequest(ctx, pr.ID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got.Title != "Monitors (revised)" || len(got.Items) != 1 || len(got.Links) != 0 {
		t.Errorf("update not applied: title=%q items=%d links=%d", got.Title, len(got.Items), len(got.Links))
	}

	// Documents.
	doc, err := repo.AddDocument(ctx, pr.ID, "quote.pdf", "purchase-requests/1/abc/quote.pdf", "application/pdf", 1234, user.ID)
	if err != nil {
		t.Fatalf("add document: %v", err)
	}
	docs, err := repo.ListDocuments(ctx, pr.ID)
	if err != nil || len(docs) != 1 {
		t.Fatalf("list documents: err=%v count=%d", err, len(docs))
	}
	if docs[0].Filename != "quote.pdf" {
		t.Errorf("filename not preserved: %q", docs[0].Filename)
	}
	if err := repo.DeleteDocument(ctx, pr.ID, doc.ID); err != nil {
		t.Fatalf("delete document: %v", err)
	}

	// Scoped listing returns this requester's PR.
	mine, err := repo.ListPurchaseRequests(ctx, user.ID, user.Email, false, false, false, false, repository.PRScopeDefault, repository.PRListFilter{})
	if err != nil || len(mine) == 0 {
		t.Fatalf("list scoped: err=%v count=%d", err, len(mine))
	}
}

func TestBusinessUnitLifecycle(t *testing.T) {
	repo, ctx := newTestRepo(t)

	a1, err := repo.UpsertUser(ctx, "bu-a1-"+t.Name(), "a1@example.com", "Approver One")
	if err != nil {
		t.Fatalf("upsert approver one: %v", err)
	}
	a2, err := repo.UpsertUser(ctx, "bu-a2-"+t.Name(), "a2@example.com", "Approver Two")
	if err != nil {
		t.Fatalf("upsert approver two: %v", err)
	}

	// A business unit with a flat list of two approvers.
	bu, err := repo.CreateBusinessUnit(ctx, repository.BusinessUnitInput{
		Name:        "Engineering",
		Description: "R&D spend",
		IsActive:    true,
		ApproverIDs: []int64{a1.ID, a2.ID},
	}, a1.ID)
	if err != nil {
		t.Fatalf("create business unit: %v", err)
	}
	t.Cleanup(func() {
		_, _ = repo.Pool().Exec(context.Background(), `DELETE FROM business_units WHERE id=$1`, bu.ID)
	})

	if len(bu.Approvers) != 2 {
		t.Fatalf("expected 2 approvers, got %d", len(bu.Approvers))
	}
	if bu.Approvers[0].ID != a1.ID || bu.Approvers[1].ID != a2.ID {
		t.Errorf("approvers not populated in order: %+v", bu.Approvers)
	}

	// Lookup returns only active business units as summaries.
	active, err := repo.ListActiveBusinessUnits(ctx)
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if !containsBusinessUnit(active, bu.ID) {
		t.Errorf("active business unit missing from lookup")
	}

	// The approver list drives the requisition-form budget-approver dropdown.
	approvers, err := repo.BusinessUnitApprovers(ctx, bu.ID)
	if err != nil {
		t.Fatalf("business unit approvers: %v", err)
	}
	if len(approvers) != 2 {
		t.Errorf("expected 2 approvers from lookup, got %d", len(approvers))
	}

	// A PR links to the business unit.
	pr, err := repo.CreatePurchaseRequest(ctx, a1.ID, repository.PurchaseRequestInput{
		Title:          "Laptops",
		BusinessUnitID: &bu.ID,
	})
	if err != nil {
		t.Fatalf("create PR: %v", err)
	}
	t.Cleanup(func() {
		_, _ = repo.Pool().Exec(context.Background(), `DELETE FROM purchase_requests WHERE id=$1`, pr.ID)
	})
	if pr.BusinessUnitID == nil || *pr.BusinessUnitID != bu.ID {
		t.Errorf("PR business_unit_id not linked: %+v", pr.BusinessUnitID)
	}

	// Usage reflects the linked PR.
	usage, err := repo.GetBusinessUnitUsage(ctx, bu.ID)
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	if usage.PurchaseRequests != 1 {
		t.Errorf("expected 1 referencing PR, got %d", usage.PurchaseRequests)
	}

	// Deactivate (soft delete) and confirm it drops from the active lookup and
	// that approvers can be replaced (down to one).
	if err := repo.UpdateBusinessUnit(ctx, bu.ID, repository.BusinessUnitInput{
		Name:        bu.Name,
		IsActive:    false,
		ApproverIDs: []int64{a2.ID},
	}); err != nil {
		t.Fatalf("update business unit: %v", err)
	}
	active, err = repo.ListActiveBusinessUnits(ctx)
	if err != nil {
		t.Fatalf("list active after deactivate: %v", err)
	}
	if containsBusinessUnit(active, bu.ID) {
		t.Errorf("deactivated business unit still in active lookup")
	}
	got, err := repo.GetBusinessUnit(ctx, bu.ID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got.IsActive {
		t.Errorf("expected inactive after deactivate")
	}
	if len(got.Approvers) != 1 || got.Approvers[0].ID != a2.ID {
		t.Errorf("expected approvers replaced down to [a2], got %+v", got.Approvers)
	}
}

func containsBusinessUnit(list []*repository.BusinessUnitSummary, id int64) bool {
	for _, c := range list {
		if c.ID == id {
			return true
		}
	}
	return false
}

func TestInvoiceCostAllocations(t *testing.T) {
	repo, ctx := newTestRepo(t)

	user, err := repo.UpsertUser(ctx, "inv-alloc-"+t.Name(), "invalloc@example.com", "Inv Alloc")
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}

	// Two cost centers; the first is the PR's, used as the contract default.
	cc1, err := repo.CreateBusinessUnit(ctx, repository.BusinessUnitInput{Name: "Engineering", IsActive: true}, user.ID)
	if err != nil {
		t.Fatalf("create cc1: %v", err)
	}
	cc2, err := repo.CreateBusinessUnit(ctx, repository.BusinessUnitInput{Name: "Marketing", IsActive: true}, user.ID)
	if err != nil {
		t.Fatalf("create cc2: %v", err)
	}
	t.Cleanup(func() {
		_, _ = repo.Pool().Exec(context.Background(), `DELETE FROM business_units WHERE id IN ($1,$2)`, cc1.ID, cc2.ID)
	})

	vendor, err := repo.CreateVendor(ctx, repository.VendorInput{Name: "Acme", IsActive: true}, user.ID)
	if err != nil {
		t.Fatalf("create vendor: %v", err)
	}
	t.Cleanup(func() {
		_, _ = repo.Pool().Exec(context.Background(), `DELETE FROM vendors WHERE id=$1`, vendor.ID)
	})

	pr, err := repo.CreatePurchaseRequest(ctx, user.ID, repository.PurchaseRequestInput{
		Title:          "Servers",
		BusinessUnitID: &cc1.ID,
	})
	if err != nil {
		t.Fatalf("create PR: %v", err)
	}
	t.Cleanup(func() {
		_, _ = repo.Pool().Exec(context.Background(), `DELETE FROM purchase_requests WHERE id=$1`, pr.ID)
	})

	// A signed contract is required to record an invoice. Insert one directly.
	var contractID int64
	if err := repo.Pool().QueryRow(ctx, `
		INSERT INTO contracts (purchase_request_id, vendor_id, title, total_amount, currency, status)
		VALUES ($1, $2, 'Server supply', 1000, 'USD', 'signed') RETURNING id`, pr.ID, vendor.ID).Scan(&contractID); err != nil {
		t.Fatalf("insert contract: %v", err)
	}
	t.Cleanup(func() {
		_, _ = repo.Pool().Exec(context.Background(), `DELETE FROM contracts WHERE id=$1`, contractID)
	})

	// The contract surfaces the PR's cost center (used to default the invoice).
	con, err := repo.GetContract(ctx, contractID)
	if err != nil {
		t.Fatalf("get contract: %v", err)
	}
	if con.BusinessUnit == nil || con.BusinessUnit.ID != cc1.ID {
		t.Errorf("contract budget unit not resolved from PR: %+v", con.BusinessUnit)
	}

	// Create an invoice split 60/40 by percentage across both cost centers.
	inv, err := repo.CreateInvoice(ctx, contractID, repository.InvoiceInput{
		InvoiceDate:    "2026-01-15",
		AllocationMode: "percentage",
		Items:          []repository.InvoiceItem{{Description: "Server", Quantity: 1, UnitPrice: 1000}},
		CostAllocations: []repository.CostAllocation{
			{BusinessUnitID: cc1.ID, Value: 60},
			{BusinessUnitID: cc2.ID, Value: 40},
		},
	}, user.ID)
	if err != nil {
		t.Fatalf("create invoice: %v", err)
	}
	if inv.AllocationMode != "percentage" || len(inv.CostAllocations) != 2 {
		t.Fatalf("unexpected allocations: mode=%q n=%d", inv.AllocationMode, len(inv.CostAllocations))
	}
	// Resolved amounts: 60% and 40% of 1000.
	if got := inv.CostAllocations[0].Amount; got != 600 {
		t.Errorf("expected 600 for cc1, got %v", got)
	}
	if got := inv.CostAllocations[1].Amount; got != 400 {
		t.Errorf("expected 400 for cc2, got %v", got)
	}

	// Switch to amount mode; values are the resolved amounts directly.
	if err := repo.UpdateInvoice(ctx, inv.ID, repository.InvoiceInput{
		InvoiceDate:    "2026-01-15",
		AllocationMode: "amount",
		Items:          []repository.InvoiceItem{{Description: "Server", Quantity: 1, UnitPrice: 1000}},
		CostAllocations: []repository.CostAllocation{
			{BusinessUnitID: cc1.ID, Value: 700},
			{BusinessUnitID: cc2.ID, Value: 300},
		},
	}); err != nil {
		t.Fatalf("update invoice: %v", err)
	}
	got, err := repo.GetInvoice(ctx, inv.ID)
	if err != nil {
		t.Fatalf("get invoice: %v", err)
	}
	if got.AllocationMode != "amount" {
		t.Errorf("expected amount mode, got %q", got.AllocationMode)
	}
	if got.CostAllocations[0].Amount != 700 || got.CostAllocations[1].Amount != 300 {
		t.Errorf("amount-mode resolved amounts wrong: %+v", got.CostAllocations)
	}

	// The cost-center invoice summary buckets the (received) invoice under
	// pending, with each cost center's allocated share totalled per currency.
	sum, err := repo.GetBusinessUnitInvoiceSummary(ctx, cc1.ID)
	if err != nil {
		t.Fatalf("cost center invoice summary: %v", err)
	}
	if sum.Pending.Count != 1 {
		t.Errorf("expected 1 pending invoice for cc1, got %d", sum.Pending.Count)
	}
	if len(sum.Pending.Totals) != 1 || sum.Pending.Totals[0].Currency != "USD" || sum.Pending.Totals[0].Amount != 700 {
		t.Errorf("cc1 pending total wrong: %+v", sum.Pending.Totals)
	}
	if sum.Approved.Count != 0 || sum.Paid.Count != 0 {
		t.Errorf("expected no approved/paid for cc1, got approved=%d paid=%d", sum.Approved.Count, sum.Paid.Count)
	}
	sum2, err := repo.GetBusinessUnitInvoiceSummary(ctx, cc2.ID)
	if err != nil {
		t.Fatalf("cost center invoice summary cc2: %v", err)
	}
	if sum2.Pending.Count != 1 || len(sum2.Pending.Totals) != 1 || sum2.Pending.Totals[0].Amount != 300 {
		t.Errorf("cc2 pending total wrong: count=%d totals=%+v", sum2.Pending.Count, sum2.Pending.Totals)
	}
}

// TestInvoiceEnteredTotal verifies the directly-entered total takes priority
// over the line-items sum, and that clearing it reverts to the derived total.
func TestInvoiceEnteredTotal(t *testing.T) {
	repo, ctx := newTestRepo(t)

	user, err := repo.UpsertUser(ctx, "inv-total-"+t.Name(), "invtotal@example.com", "Inv Total")
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}
	cc, err := repo.CreateBusinessUnit(ctx, repository.BusinessUnitInput{Name: "Ops", IsActive: true}, user.ID)
	if err != nil {
		t.Fatalf("create cc: %v", err)
	}
	t.Cleanup(func() { _, _ = repo.Pool().Exec(context.Background(), `DELETE FROM business_units WHERE id=$1`, cc.ID) })

	vendor, err := repo.CreateVendor(ctx, repository.VendorInput{Name: "Globex", IsActive: true}, user.ID)
	if err != nil {
		t.Fatalf("create vendor: %v", err)
	}
	t.Cleanup(func() { _, _ = repo.Pool().Exec(context.Background(), `DELETE FROM vendors WHERE id=$1`, vendor.ID) })

	pr, err := repo.CreatePurchaseRequest(ctx, user.ID, repository.PurchaseRequestInput{Title: "Misc", BusinessUnitID: &cc.ID})
	if err != nil {
		t.Fatalf("create PR: %v", err)
	}
	t.Cleanup(func() {
		_, _ = repo.Pool().Exec(context.Background(), `DELETE FROM purchase_requests WHERE id=$1`, pr.ID)
	})

	var contractID int64
	if err := repo.Pool().QueryRow(ctx, `
		INSERT INTO contracts (purchase_request_id, vendor_id, title, total_amount, currency, status)
		VALUES ($1, $2, 'Supply', 5000, 'USD', 'signed') RETURNING id`, pr.ID, vendor.ID).Scan(&contractID); err != nil {
		t.Fatalf("insert contract: %v", err)
	}
	t.Cleanup(func() { _, _ = repo.Pool().Exec(context.Background(), `DELETE FROM contracts WHERE id=$1`, contractID) })

	// Line items sum to 1000, but the entered total of 1500 takes priority.
	entered := 1500.0
	inv, err := repo.CreateInvoice(ctx, contractID, repository.InvoiceInput{
		InvoiceDate:    "2026-02-01",
		AllocationMode: "amount",
		EnteredTotal:   &entered,
		Items:          []repository.InvoiceItem{{Description: "Server", Quantity: 1, UnitPrice: 1000}},
		// Amount allocations validate (in the handler) against the entered total.
		CostAllocations: []repository.CostAllocation{{BusinessUnitID: cc.ID, Value: 1500}},
	}, user.ID)
	if err != nil {
		t.Fatalf("create invoice: %v", err)
	}
	if inv.TotalAmount != 1500 {
		t.Errorf("expected entered total 1500 to win, got total_amount=%v", inv.TotalAmount)
	}
	if inv.EnteredTotal == nil || *inv.EnteredTotal != 1500 {
		t.Errorf("expected entered_total=1500 returned, got %v", inv.EnteredTotal)
	}
	// The entered total drives the resolved allocation amount, not the items sum.
	if inv.CostAllocations[0].Amount != 1500 {
		t.Errorf("expected allocation amount 1500, got %v", inv.CostAllocations[0].Amount)
	}

	// Clearing the entered total reverts to the line-items sum (1000).
	if err := repo.UpdateInvoice(ctx, inv.ID, repository.InvoiceInput{
		InvoiceDate:     "2026-02-01",
		AllocationMode:  "amount",
		EnteredTotal:    nil,
		Items:           []repository.InvoiceItem{{Description: "Server", Quantity: 1, UnitPrice: 1000}},
		CostAllocations: []repository.CostAllocation{{BusinessUnitID: cc.ID, Value: 1000}},
	}); err != nil {
		t.Fatalf("update invoice: %v", err)
	}
	got, err := repo.GetInvoice(ctx, inv.ID)
	if err != nil {
		t.Fatalf("get invoice: %v", err)
	}
	if got.EnteredTotal != nil {
		t.Errorf("expected entered_total cleared to nil, got %v", *got.EnteredTotal)
	}
	if got.TotalAmount != 1000 {
		t.Errorf("expected derived total 1000 after clearing, got %v", got.TotalAmount)
	}
}

// TestTeamLeadVisibilityGate verifies that a PR pending team-lead approval is
// hidden from procurement but visible to the team lead and admins, and becomes
// visible to procurement once approved.
func TestTeamLeadVisibilityGate(t *testing.T) {
	repo, ctx := newTestRepo(t)

	requester, err := repo.UpsertUser(ctx, "tl-req-"+t.Name(), "tl-req-"+t.Name()+"@example.com", "Requester")
	if err != nil {
		t.Fatalf("upsert requester: %v", err)
	}
	lead, err := repo.UpsertUser(ctx, "tl-lead-"+t.Name(), "tl-lead-"+t.Name()+"@example.com", "Lead")
	if err != nil {
		t.Fatalf("upsert lead: %v", err)
	}
	fin, err := repo.UpsertUser(ctx, "tl-fin-"+t.Name(), "tl-fin-"+t.Name()+"@example.com", "Procurement")
	if err != nil {
		t.Fatalf("upsert procurement: %v", err)
	}

	pr, err := repo.CreatePurchaseRequest(ctx, requester.ID, repository.PurchaseRequestInput{
		Title:         "Gated PR",
		TeamLeadEmail: lead.Email,
	})
	if err != nil {
		t.Fatalf("create PR: %v", err)
	}
	t.Cleanup(func() {
		_, _ = repo.Pool().Exec(context.Background(), `DELETE FROM purchase_requests WHERE id=$1`, pr.ID)
	})

	has := func(callerID int64, callerEmail string, seesAll, isAdmin bool, scope repository.PRListScope) bool {
		prs, err := repo.ListPurchaseRequests(ctx, callerID, callerEmail, seesAll, isAdmin, false, false, scope, repository.PRListFilter{})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		for _, p := range prs {
			if p.ID == pr.ID {
				return true
			}
		}
		return false
	}

	// Pending: hidden from procurement, visible to team lead (Approvals) and admin.
	if has(fin.ID, fin.Email, true, false, repository.PRScopeDefault) {
		t.Error("pending PR should be hidden from procurement")
	}
	if !has(lead.ID, lead.Email, false, false, repository.PRScopeApprovals) {
		t.Error("pending PR should be in the team lead's Approvals queue")
	}
	if !has(requester.ID, requester.Email, false, false, repository.PRScopeMine) {
		t.Error("requester should always see their own PR")
	}
	if !has(fin.ID, fin.Email, true, true, repository.PRScopeDefault) {
		t.Error("admin should see the pending PR")
	}

	// Approve, then procurement can see it.
	if err := repo.RecordTeamLeadDecision(ctx, pr.ID, lead.ID, "approved", "ok"); err != nil {
		t.Fatalf("record decision: %v", err)
	}
	if !has(fin.ID, fin.Email, true, false, repository.PRScopeDefault) {
		t.Error("approved PR should be visible to procurement")
	}
}

// TestOwnPRVisibleInBothScopes verifies that a PR submitted by a procurement
// user appears in BOTH the "My requests" list (PRScopeMine) and the full
// "Purchase requests" queue (PRScopeDefault with seesAll) — even while it is
// still pending team-lead approval — because the default scope always includes
// the caller's own submissions.
func TestOwnPRVisibleInBothScopes(t *testing.T) {
	repo, ctx := newTestRepo(t)

	proc, err := repo.UpsertUser(ctx, "own-proc-"+t.Name(), "own-proc-"+t.Name()+"@example.com", "Procurement")
	if err != nil {
		t.Fatalf("upsert procurement: %v", err)
	}
	lead, err := repo.UpsertUser(ctx, "own-lead-"+t.Name(), "own-lead-"+t.Name()+"@example.com", "Lead")
	if err != nil {
		t.Fatalf("upsert lead: %v", err)
	}

	// The procurement user submits their own PR (pending team-lead approval).
	pr, err := repo.CreatePurchaseRequest(ctx, proc.ID, repository.PurchaseRequestInput{
		Title:         "Procurement's own PR",
		TeamLeadEmail: lead.Email,
	})
	if err != nil {
		t.Fatalf("create PR: %v", err)
	}
	t.Cleanup(func() {
		_, _ = repo.Pool().Exec(context.Background(), `DELETE FROM purchase_requests WHERE id=$1`, pr.ID)
	})

	has := func(seesAll bool, scope repository.PRListScope) bool {
		prs, err := repo.ListPurchaseRequests(ctx, proc.ID, proc.Email, seesAll, false, false, false, scope, repository.PRListFilter{})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		for _, p := range prs {
			if p.ID == pr.ID {
				return true
			}
		}
		return false
	}

	if !has(false, repository.PRScopeMine) {
		t.Error("own PR should appear in My requests (scope=mine)")
	}
	if !has(true, repository.PRScopeDefault) {
		t.Error("own PR should appear in the Purchase requests queue (default scope, procurement)")
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
