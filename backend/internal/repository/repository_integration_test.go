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
		Title:      "Monitors",
		CostCenter: "Engineering",
		Comments:   "Need by Q3",
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
		Title:      "Monitors (revised)",
		CostCenter: "Engineering",
		Comments:   "Updated",
		Items:      []repository.Item{{Description: "Dell 32\" monitor", Quantity: 3}},
		Links:      nil,
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
	mine, err := repo.ListPurchaseRequests(ctx, user.ID, false, false, false, repository.PRScopeDefault)
	if err != nil || len(mine) == 0 {
		t.Fatalf("list scoped: err=%v count=%d", err, len(mine))
	}
}

func TestCostCenterLifecycle(t *testing.T) {
	repo, ctx := newTestRepo(t)

	owner, err := repo.UpsertUser(ctx, "cc-owner-"+t.Name(), "owner@example.com", "Owner")
	if err != nil {
		t.Fatalf("upsert owner: %v", err)
	}
	sec, err := repo.UpsertUser(ctx, "cc-sec-"+t.Name(), "secondary@example.com", "Secondary")
	if err != nil {
		t.Fatalf("upsert secondary: %v", err)
	}

	// Create with owners, budget and code.
	cc, err := repo.CreateCostCenter(ctx, repository.CostCenterInput{
		Code:              "CC-ENG-001",
		Name:              "Engineering",
		Description:       "R&D spend",
		PrimaryOwnerID:    &owner.ID,
		Budget:            150000,
		Currency:          "USD",
		IsActive:          true,
		SecondaryOwnerIDs: []int64{sec.ID},
	}, owner.ID)
	if err != nil {
		t.Fatalf("create cost center: %v", err)
	}
	t.Cleanup(func() {
		_, _ = repo.Pool().Exec(context.Background(), `DELETE FROM cost_centers WHERE id=$1`, cc.ID)
	})

	if cc.PrimaryOwner == nil || cc.PrimaryOwner.Email != "owner@example.com" {
		t.Errorf("primary owner not populated: %+v", cc.PrimaryOwner)
	}
	if len(cc.SecondaryOwners) != 1 || cc.SecondaryOwners[0].ID != sec.ID {
		t.Errorf("secondary owners not populated: %+v", cc.SecondaryOwners)
	}

	// Lookup returns only active cost centers as summaries.
	active, err := repo.ListActiveCostCenters(ctx)
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if !containsCostCenter(active, cc.ID) {
		t.Errorf("active cost center missing from lookup")
	}

	// A PR linked to the cost center mirrors its name into the legacy column.
	pr, err := repo.CreatePurchaseRequest(ctx, owner.ID, repository.PurchaseRequestInput{
		Title:        "Laptops",
		CostCenterID: &cc.ID,
	})
	if err != nil {
		t.Fatalf("create PR: %v", err)
	}
	t.Cleanup(func() {
		_, _ = repo.Pool().Exec(context.Background(), `DELETE FROM purchase_requests WHERE id=$1`, pr.ID)
	})
	if pr.CostCenterID == nil || *pr.CostCenterID != cc.ID {
		t.Errorf("PR cost_center_id not linked: %+v", pr.CostCenterID)
	}
	if pr.CostCenter != "Engineering" {
		t.Errorf("PR cost_center name not mirrored: %q", pr.CostCenter)
	}

	// Usage reflects the linked PR.
	usage, err := repo.GetCostCenterUsage(ctx, cc.ID)
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	if usage.PurchaseRequests != 1 {
		t.Errorf("expected 1 referencing PR, got %d", usage.PurchaseRequests)
	}

	// Deactivate (soft delete) and confirm it drops from the active lookup and
	// that secondary owners can be replaced.
	if err := repo.UpdateCostCenter(ctx, cc.ID, repository.CostCenterInput{
		Code:           cc.Code,
		Name:           cc.Name,
		PrimaryOwnerID: &owner.ID,
		IsActive:       false,
	}); err != nil {
		t.Fatalf("update cost center: %v", err)
	}
	active, err = repo.ListActiveCostCenters(ctx)
	if err != nil {
		t.Fatalf("list active after deactivate: %v", err)
	}
	if containsCostCenter(active, cc.ID) {
		t.Errorf("deactivated cost center still in active lookup")
	}
	got, err := repo.GetCostCenter(ctx, cc.ID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got.IsActive {
		t.Errorf("expected inactive after deactivate")
	}
	if len(got.SecondaryOwners) != 0 {
		t.Errorf("expected secondary owners cleared, got %+v", got.SecondaryOwners)
	}
}

func containsCostCenter(list []*repository.CostCenterSummary, id int64) bool {
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
	cc1, err := repo.CreateCostCenter(ctx, repository.CostCenterInput{Name: "Engineering", IsActive: true}, user.ID)
	if err != nil {
		t.Fatalf("create cc1: %v", err)
	}
	cc2, err := repo.CreateCostCenter(ctx, repository.CostCenterInput{Name: "Marketing", IsActive: true}, user.ID)
	if err != nil {
		t.Fatalf("create cc2: %v", err)
	}
	t.Cleanup(func() {
		_, _ = repo.Pool().Exec(context.Background(), `DELETE FROM cost_centers WHERE id IN ($1,$2)`, cc1.ID, cc2.ID)
	})

	vendor, err := repo.CreateVendor(ctx, repository.VendorInput{Name: "Acme", IsActive: true}, user.ID)
	if err != nil {
		t.Fatalf("create vendor: %v", err)
	}
	t.Cleanup(func() {
		_, _ = repo.Pool().Exec(context.Background(), `DELETE FROM vendors WHERE id=$1`, vendor.ID)
	})

	pr, err := repo.CreatePurchaseRequest(ctx, user.ID, repository.PurchaseRequestInput{
		Title:        "Servers",
		CostCenterID: &cc1.ID,
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
	if con.CostCenter == nil || con.CostCenter.ID != cc1.ID {
		t.Errorf("contract cost center not resolved from PR: %+v", con.CostCenter)
	}

	// Create an invoice split 60/40 by percentage across both cost centers.
	inv, err := repo.CreateInvoice(ctx, contractID, repository.InvoiceInput{
		InvoiceDate:    "2026-01-15",
		AllocationMode: "percentage",
		Items:          []repository.InvoiceItem{{Description: "Server", Quantity: 1, UnitPrice: 1000}},
		CostAllocations: []repository.CostAllocation{
			{CostCenterID: cc1.ID, Value: 60},
			{CostCenterID: cc2.ID, Value: 40},
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
			{CostCenterID: cc1.ID, Value: 700},
			{CostCenterID: cc2.ID, Value: 300},
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
	sum, err := repo.GetCostCenterInvoiceSummary(ctx, cc1.ID)
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
	sum2, err := repo.GetCostCenterInvoiceSummary(ctx, cc2.ID)
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
	cc, err := repo.CreateCostCenter(ctx, repository.CostCenterInput{Name: "Ops", IsActive: true}, user.ID)
	if err != nil {
		t.Fatalf("create cc: %v", err)
	}
	t.Cleanup(func() { _, _ = repo.Pool().Exec(context.Background(), `DELETE FROM cost_centers WHERE id=$1`, cc.ID) })

	vendor, err := repo.CreateVendor(ctx, repository.VendorInput{Name: "Globex", IsActive: true}, user.ID)
	if err != nil {
		t.Fatalf("create vendor: %v", err)
	}
	t.Cleanup(func() { _, _ = repo.Pool().Exec(context.Background(), `DELETE FROM vendors WHERE id=$1`, vendor.ID) })

	pr, err := repo.CreatePurchaseRequest(ctx, user.ID, repository.PurchaseRequestInput{Title: "Misc", CostCenterID: &cc.ID})
	if err != nil {
		t.Fatalf("create PR: %v", err)
	}
	t.Cleanup(func() { _, _ = repo.Pool().Exec(context.Background(), `DELETE FROM purchase_requests WHERE id=$1`, pr.ID) })

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
		CostAllocations: []repository.CostAllocation{{CostCenterID: cc.ID, Value: 1500}},
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
		CostAllocations: []repository.CostAllocation{{CostCenterID: cc.ID, Value: 1000}},
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

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
