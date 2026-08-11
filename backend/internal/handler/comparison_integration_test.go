package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
)

// TestQuotationComparisonFlow drives the comparison endpoints end to end against the
// real schema: the missing-final-quote gate, generating, the derived metrics, an
// approver's read access, editing the budget, and removal — plus the process events
// each of those writes.
func TestQuotationComparisonFlow(t *testing.T) {
	repo, ctx := newHandlerTestRepo(t)
	suffix := "cmp-" + t.Name()

	requester, err := repo.UpsertUser(ctx, "h-"+suffix+"-req", "h-"+suffix+"-req@example.com", "Requester")
	if err != nil {
		t.Fatalf("upsert requester: %v", err)
	}
	lead, err := repo.UpsertUser(ctx, "h-"+suffix+"-lead", "h-"+suffix+"-lead@example.com", "Lead")
	if err != nil {
		t.Fatalf("upsert lead: %v", err)
	}
	proc, err := repo.UpsertUser(ctx, "h-"+suffix+"-proc", "h-"+suffix+"-proc@example.com", "Procurement")
	if err != nil {
		t.Fatalf("upsert procurement: %v", err)
	}
	// Self-assignment validates the role in the DB, not just the request context.
	if err := repo.EnsureUserHasRole(ctx, proc.ID, model.RoleProcurement); err != nil {
		t.Fatalf("grant procurement role: %v", err)
	}

	h := &PurchaseRequestsHandler{Repo: repo}
	qh := &QuotationsHandler{Repo: repo}

	// A PR that has cleared the team-lead gate and is assigned to procurement —
	// everything the comparison sits behind.
	rec := httptest.NewRecorder()
	h.Create(rec, authed(requester, []string{model.RoleStaff},
		`{"title":"Comparison PR","team_lead_email":"`+lead.Email+`"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create PR = %d, want 201 (body=%s)", rec.Code, rec.Body.String())
	}
	var pr repository.PurchaseRequest
	if err := json.Unmarshal(rec.Body.Bytes(), &pr); err != nil {
		t.Fatalf("decode created PR: %v", err)
	}

	alpha, err := repo.CreateVendor(ctx, repository.VendorInput{Name: "Alpha " + t.Name(), IsActive: true}, proc.ID)
	if err != nil {
		t.Fatalf("create vendor alpha: %v", err)
	}
	beta, err := repo.CreateVendor(ctx, repository.VendorInput{Name: "Beta " + t.Name(), IsActive: true}, proc.ID)
	if err != nil {
		t.Fatalf("create vendor beta: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		// The PR cascades to its quotations, documents, extractions and comparison;
		// the vendors are only removable once those quotations are gone.
		if _, err := repo.Pool().Exec(bg, `DELETE FROM purchase_requests WHERE id=$1`, pr.ID); err != nil {
			t.Errorf("cleanup purchase request: %v", err)
		}
		if _, err := repo.Pool().Exec(bg, `DELETE FROM vendors WHERE id = ANY($1)`, []int64{alpha.ID, beta.ID}); err != nil {
			t.Errorf("cleanup vendors: %v", err)
		}
		if _, err := repo.Pool().Exec(bg, `DELETE FROM users WHERE id = ANY($1)`,
			[]int64{requester.ID, lead.ID, proc.ID}); err != nil {
			t.Errorf("cleanup users: %v", err)
		}
	})

	rec = httptest.NewRecorder()
	h.TeamLeadDecision(rec, withID(authed(lead, []string{model.RoleStaff}, `{"decision":"approve"}`), pr.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("team lead approve = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.SetAssignee(rec, withID(authed(proc, []string{model.RoleProcurement},
		`{"assignee_id":`+strconv.FormatInt(proc.ID, 10)+`}`), pr.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("self-assign = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}

	// One quotation per vendor. Alpha gets both PDFs read; Beta only an initial one,
	// which is the case the confirmation modal exists for.
	quoAlpha := createQuotationVia(t, qh, proc, pr.ID, alpha.ID)
	quoBeta := createQuotationVia(t, qh, proc, pr.ID, beta.ID)

	attachRead(t, repo, ctx, pr.ID, quoAlpha, proc.ID, repository.QuotationDocInitial,
		`{"vendor_name":"Alpha","currency":"USD","subtotal_amount":1000,"total_amount":1000,
		  "total_includes_tax":false,"taxes":[{"label":"VAT","rate":18,"amount":180}],
		  "items":[{"description":"Camera","quantity":10,"unit_price":100}]}`, false)
	attachRead(t, repo, ctx, pr.ID, quoAlpha, proc.ID, repository.QuotationDocFinal,
		`{"vendor_name":"Alpha","currency":"USD","subtotal_amount":900,"total_amount":1062,
		  "total_includes_tax":true,"taxes":[{"label":"VAT","rate":18,"amount":162}],
		  "items":[{"description":"Camera","quantity":10,"unit_price":90}]}`, true)
	attachRead(t, repo, ctx, pr.ID, quoBeta, proc.ID, repository.QuotationDocInitial,
		`{"vendor_name":"Beta","currency":"USD","total_amount":1400,
		  "items":[{"description":"Camera","quantity":10,"unit_price":140}]}`, true)

	// Not generated yet: procurement gets the figures (it needs missing_final for the
	// confirmation), the requester gets nothing to read.
	view := getComparison(t, h, proc, []string{model.RoleProcurement}, pr.ID)
	if view.Exists || !view.CanManage {
		t.Fatalf("before generating: exists=%v can_manage=%v, want false/true", view.Exists, view.CanManage)
	}
	if len(view.MissingFinal) != 1 || view.MissingFinal[0] != beta.Name {
		t.Fatalf("missing_final = %v, want [%s]", view.MissingFinal, beta.Name)
	}
	staffView := getComparison(t, h, requester, []string{model.RoleStaff}, pr.ID)
	if staffView.Exists || len(staffView.Vendors) != 0 {
		t.Fatalf("requester should see no comparison before one is generated, got %+v", staffView)
	}

	// Generating without the consent is refused, naming the vendor.
	rec = httptest.NewRecorder()
	h.GenerateQuotationComparison(rec, withID(authed(proc, []string{model.RoleProcurement},
		`{"approved_budget":null,"currency":"USD","use_initial_for_final":false}`), pr.ID))
	if rec.Code != http.StatusConflict {
		t.Fatalf("generate without consent = %d, want 409 (body=%s)", rec.Code, rec.Body.String())
	}

	// With the consent it is generated, and Beta's initial figures stand in.
	rec = httptest.NewRecorder()
	h.GenerateQuotationComparison(rec, withID(authed(proc, []string{model.RoleProcurement},
		`{"approved_budget":1100,"currency":"USD","use_initial_for_final":true}`), pr.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("generate = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode comparison: %v", err)
	}
	if !view.Exists || len(view.Vendors) != 2 {
		t.Fatalf("generated view: exists=%v vendors=%d", view.Exists, len(view.Vendors))
	}
	a, b := view.Vendors[0], view.Vendors[1]
	if a.VendorName != alpha.Name {
		a, b = b, a
	}
	// Alpha's initial total was printed tax-exclusive, so tax is added: 1000 + 180.
	if a.Initial.GrandTotal == nil || *a.Initial.GrandTotal != 1180 || !a.Initial.AddedTax {
		t.Errorf("Alpha initial = %+v, want 1180 with tax added", a.Initial)
	}
	if a.Final.GrandTotal == nil || *a.Final.GrandTotal != 1062 {
		t.Errorf("Alpha final = %+v, want 1062", a.Final)
	}
	if a.NegotiatedSaving == nil || *a.NegotiatedSaving != 118 {
		t.Errorf("Alpha negotiated saving = %v, want 118", a.NegotiatedSaving)
	}
	if a.VarianceVsBudget == nil || *a.VarianceVsBudget != -38 {
		t.Errorf("Alpha variance vs budget = %v, want -38", a.VarianceVsBudget)
	}
	if !a.Lowest {
		t.Error("Alpha's 1062 should be the lowest final quote")
	}
	if !b.FinalIsInitial || b.Final.Source != sourceInitial {
		t.Errorf("Beta should be comparing on its initial figures, got %+v", b.Final)
	}
	if b.Final.GrandTotal == nil || *b.Final.GrandTotal != 1400 {
		t.Errorf("Beta stand-in final = %v, want 1400", b.Final.GrandTotal)
	}
	if a.Items[0].InitialUnitPrice == nil || *a.Items[0].InitialUnitPrice != 100 ||
		a.Items[0].FinalUnitPrice == nil || *a.Items[0].FinalUnitPrice != 90 {
		t.Errorf("Alpha's item row should pair 100 → 90, got %+v", a.Items[0])
	}

	// The requester (an ordinary viewer, no procurement access) can now read it.
	staffView = getComparison(t, h, requester, []string{model.RoleStaff}, pr.ID)
	if !staffView.Exists || len(staffView.Vendors) != 2 || staffView.CanManage {
		t.Fatalf("requester view = exists %v, vendors %d, can_manage %v; want true/2/false",
			staffView.Exists, len(staffView.Vendors), staffView.CanManage)
	}

	// Editing the budget re-derives the variances.
	rec = httptest.NewRecorder()
	h.UpdateQuotationComparison(rec, withID(authed(proc, []string{model.RoleProcurement},
		`{"approved_budget":1000,"currency":"USD","use_initial_for_final":true}`), pr.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("update = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode updated comparison: %v", err)
	}
	for _, v := range view.Vendors {
		if v.VendorName == alpha.Name && (v.VarianceVsBudget == nil || *v.VarianceVsBudget != 62) {
			t.Errorf("Alpha variance after budget edit = %v, want 62", v.VarianceVsBudget)
		}
	}

	// A quotation edited after the comparison was generated is reflected on the next
	// read — the whole reason nothing is snapshotted.
	if err := repo.UpdateQuotation(ctx, quoBeta, repository.QuotationInput{
		VendorID: beta.ID, TotalAmount: 900, Currency: "USD",
	}); err != nil {
		t.Fatalf("update Beta quotation: %v", err)
	}
	if _, err := repo.Pool().Exec(ctx,
		`DELETE FROM quotation_extractions WHERE quotation_id = $1`, quoBeta); err != nil {
		t.Fatalf("drop Beta's read: %v", err)
	}
	view = getComparison(t, h, proc, []string{model.RoleProcurement}, pr.ID)
	for _, v := range view.Vendors {
		if v.VendorName != beta.Name {
			continue
		}
		if v.Initial.Source != sourceRecord || v.Initial.GrandTotal == nil || *v.Initial.GrandTotal != 900 {
			t.Errorf("Beta should now read 900 from the quotation record, got %+v", v.Initial)
		}
	}

	// Process events: one create, two updates (the budget edit, then nothing else).
	assertEventCount(t, repo, ctx, pr.ID, model.ProcessCreateQuotationComparison, 1)
	assertEventCount(t, repo, ctx, pr.ID, model.ProcessUpdateQuotationComparison, 1)

	rec = httptest.NewRecorder()
	h.DeleteQuotationComparison(rec, withID(authed(proc, []string{model.RoleProcurement}, ""), pr.ID))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d, want 204 (body=%s)", rec.Code, rec.Body.String())
	}
	assertEventCount(t, repo, ctx, pr.ID, model.ProcessDeleteQuotationComparison, 1)
	if after := getComparison(t, h, proc, []string{model.RoleProcurement}, pr.ID); after.Exists {
		t.Error("comparison should be gone after delete")
	}
}

func createQuotationVia(t *testing.T, qh *QuotationsHandler, user *repository.User, prID, vendorID int64) int64 {
	t.Helper()
	rec := httptest.NewRecorder()
	qh.Create(rec, withID(authed(user, []string{model.RoleProcurement},
		`{"vendor_id":`+strconv.FormatInt(vendorID, 10)+`,"currency":"USD"}`), prID))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create quotation = %d, want 201 (body=%s)", rec.Code, rec.Body.String())
	}
	var q repository.Quotation
	if err := json.Unmarshal(rec.Body.Bytes(), &q); err != nil {
		t.Fatalf("decode quotation: %v", err)
	}
	return q.ID
}

// attachRead attaches a PDF to one of a quotation's slots and records a succeeded
// read of it, as an upload + extract would.
func attachRead(t *testing.T, repo *repository.Repository, ctx context.Context,
	prID, quotationID, userID int64, slot repository.QuotationDocSlot, rawJSON string, applied bool) {
	t.Helper()
	doc, err := repo.AddOwnedDocument(ctx, prID, model.OwnerQuotation, quotationID,
		string(slot)+".pdf", "test/"+string(slot)+"-"+strconv.FormatInt(quotationID, 10)+".pdf",
		"application/pdf", 1024, userID, "")
	if err != nil {
		t.Fatalf("add %s document: %v", slot, err)
	}
	if _, err := repo.SetQuotationDocument(ctx, quotationID, slot, doc.ID); err != nil {
		t.Fatalf("set %s document: %v", slot, err)
	}
	ext, err := repo.CreateExtraction(ctx, prID, doc.ID, &quotationID, "claude-opus-5", userID)
	if err != nil {
		t.Fatalf("create extraction: %v", err)
	}
	if err := repo.FinishExtraction(ctx, ext.ID, repository.ExtractionSucceeded,
		[]byte(rawJSON), 10, 10, ""); err != nil {
		t.Fatalf("finish extraction: %v", err)
	}
	if applied {
		if err := repo.MarkExtractionApplied(ctx, ext.ID, quotationID, userID); err != nil {
			t.Fatalf("mark applied: %v", err)
		}
	}
}

func getComparison(t *testing.T, h *PurchaseRequestsHandler, user *repository.User, roles []string, prID int64) comparisonView {
	t.Helper()
	rec := httptest.NewRecorder()
	h.GetQuotationComparison(rec, withID(authed(user, roles, ""), prID))
	if rec.Code != http.StatusOK {
		t.Fatalf("get comparison = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var view comparisonView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode comparison: %v", err)
	}
	return view
}

func assertEventCount(t *testing.T, repo *repository.Repository, ctx context.Context, prID int64, action string, want int) {
	t.Helper()
	var got int
	if err := repo.Pool().QueryRow(ctx,
		`SELECT count(*) FROM process_events WHERE purchase_request_id=$1 AND action=$2`,
		prID, action).Scan(&got); err != nil {
		t.Fatalf("count %s events: %v", action, err)
	}
	if got != want {
		t.Errorf("%s process events = %d, want %d", action, got, want)
	}
}
