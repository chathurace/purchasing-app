package repository_test

import (
	"context"
	"testing"

	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
)

// TestTeamsAndAssignee covers the teams membership model (a member is a user who
// holds the team's role) and the recommendation approval-card assignee.
func TestTeamsAndAssignee(t *testing.T) {
	repo, ctx := newTestRepo(t)

	teams, err := repo.ListTeams(ctx)
	if err != nil {
		t.Fatalf("list teams: %v", err)
	}
	byKey := map[string]*repository.Team{}
	for _, tm := range teams {
		byKey[tm.Key] = tm
	}
	for _, key := range []string{"legal", "security", "compliance", "procurement"} {
		if byKey[key] == nil {
			t.Fatalf("team %q missing from ListTeams", key)
		}
	}
	if byKey["legal"].MemberRole != model.RoleLegal {
		t.Fatalf("legal team member_role = %q, want legal", byKey["legal"].MemberRole)
	}

	// A user becomes a member by holding the team's role.
	u, err := repo.UpsertUser(ctx, "legal-sub-"+t.Name(), "lawyer-"+t.Name()+"@example.com", "Lawyer")
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}
	if err := repo.EnsureUserHasRole(ctx, u.ID, model.RoleLegal); err != nil {
		t.Fatalf("grant legal role: %v", err)
	}
	members, err := repo.ListUsersByRole(ctx, model.RoleLegal)
	if err != nil {
		t.Fatalf("list users by role: %v", err)
	}
	if !containsUser(members, u.ID) {
		t.Fatalf("user %d not in legal members after grant", u.ID)
	}
	team, err := repo.GetTeamByKey(ctx, "legal")
	if err != nil {
		t.Fatalf("get team: %v", err)
	}
	if !containsUser(team.Members, u.ID) {
		t.Fatalf("user %d not in GetTeamByKey members", u.ID)
	}
	if ok, err := repo.UserHasRole(ctx, u.ID, model.RoleLegal); err != nil || !ok {
		t.Fatalf("UserHasRole(legal) = %v, %v; want true, nil", ok, err)
	}

	// Team email round-trips and is discoverable by role (for the CC).
	if err := repo.SetTeamEmail(ctx, "legal", "legal-team@example.com"); err != nil {
		t.Fatalf("set team email: %v", err)
	}
	if got, _ := repo.TeamEmailForRole(ctx, model.RoleLegal); got != "legal-team@example.com" {
		t.Fatalf("TeamEmailForRole = %q, want legal-team@example.com", got)
	}

	// Assignee on a recommendation's legal card round-trips.
	pr, err := repo.CreatePurchaseRequest(ctx, u.ID, repository.PurchaseRequestInput{Title: "Assignee test"})
	if err != nil {
		t.Fatalf("create PR: %v", err)
	}
	vendor, err := repo.CreateVendor(ctx, repository.VendorInput{Name: "Acme " + t.Name()}, u.ID)
	if err != nil {
		t.Fatalf("create vendor: %v", err)
	}
	if _, err := repo.CreateQuotation(ctx, pr.ID, repository.QuotationInput{VendorID: vendor.ID, TotalAmount: 10, Currency: "USD"}, u.ID); err != nil {
		t.Fatalf("create quotation: %v", err)
	}
	if _, err := repo.CreateRecommendation(ctx, pr.ID, repository.RecommendationInput{VendorID: vendor.ID, Description: "go", RequiredTypes: []string{model.RecApprovalLegal}}, u.ID); err != nil {
		t.Fatalf("create recommendation: %v", err)
	}
	if err := repo.SetRecAssignee(ctx, pr.ID, model.RecApprovalLegal, &u.ID); err != nil {
		t.Fatalf("set assignee: %v", err)
	}
	rec, err := repo.GetRecommendation(ctx, pr.ID)
	if err != nil || rec == nil {
		t.Fatalf("get recommendation: %v", err)
	}
	legal := findApprovalTest(rec, model.RecApprovalLegal)
	if legal == nil || legal.AssigneeID == nil || *legal.AssigneeID != u.ID {
		t.Fatalf("legal assignee not persisted: %+v", legal)
	}
	if legal.Assignee == nil || legal.Assignee.ID != u.ID {
		t.Fatalf("legal assignee identity not loaded: %+v", legal)
	}

	// Clearing the assignee.
	if err := repo.SetRecAssignee(ctx, pr.ID, model.RecApprovalLegal, nil); err != nil {
		t.Fatalf("clear assignee: %v", err)
	}
	rec, _ = repo.GetRecommendation(ctx, pr.ID)
	if legal := findApprovalTest(rec, model.RecApprovalLegal); legal == nil || legal.AssigneeID != nil {
		t.Fatalf("assignee not cleared: %+v", legal)
	}
}

// TestComplianceApprovalCard covers the compliance card end-to-end: a compliance
// team member is an approver of the PR (the recCardTypes array bind of
// approvablePredicate), the PR shows up in their Approvals scope as pending, the
// assignee round-trips, and the card gates RecommendationFullyApproved exactly
// like legal/security.
func TestComplianceApprovalCard(t *testing.T) {
	repo, ctx := newTestRepo(t)

	requester, err := repo.UpsertUser(ctx, "comp-req-"+t.Name(), "comp-req-"+t.Name()+"@example.com", "Requester")
	if err != nil {
		t.Fatalf("upsert requester: %v", err)
	}
	officer, err := repo.UpsertUser(ctx, "comp-off-"+t.Name(), "comp-off-"+t.Name()+"@example.com", "Officer")
	if err != nil {
		t.Fatalf("upsert officer: %v", err)
	}
	if err := repo.EnsureUserHasRole(ctx, officer.ID, model.RoleCompliance); err != nil {
		t.Fatalf("grant compliance role: %v", err)
	}
	pr, err := repo.CreatePurchaseRequest(ctx, requester.ID, repository.PurchaseRequestInput{Title: "Compliance test"})
	if err != nil {
		t.Fatalf("create PR: %v", err)
	}
	vendor, err := repo.CreateVendor(ctx, repository.VendorInput{Name: "CompVendor " + t.Name()}, requester.ID)
	if err != nil {
		t.Fatalf("create vendor: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = repo.Pool().Exec(bg, `DELETE FROM purchase_requests WHERE id = $1`, pr.ID)
		_, _ = repo.Pool().Exec(bg, `DELETE FROM vendors WHERE id = $1`, vendor.ID)
		_, _ = repo.Pool().Exec(bg, `DELETE FROM users WHERE id = ANY($1)`, []int64{requester.ID, officer.ID})
	})
	if _, err := repo.CreateQuotation(ctx, pr.ID, repository.QuotationInput{VendorID: vendor.ID, TotalAmount: 10, Currency: "USD"}, requester.ID); err != nil {
		t.Fatalf("create quotation: %v", err)
	}
	// Team-lead approval is what makes the PR visible to card actors.
	if err := repo.RecordTeamLeadDecision(ctx, pr.ID, requester.ID, "approved", ""); err != nil {
		t.Fatalf("team lead approve: %v", err)
	}
	if _, err := repo.CreateRecommendation(ctx, pr.ID, repository.RecommendationInput{
		VendorID: vendor.ID, Description: "compliance please",
		RequiredTypes: []string{model.RecApprovalCompliance},
	}, requester.ID); err != nil {
		t.Fatalf("create recommendation: %v", err)
	}

	compTypes := []string{model.RecApprovalCompliance}
	if ok, err := repo.IsApproverForPR(ctx, pr.ID, officer.ID, officer.Email, compTypes); err != nil || !ok {
		t.Fatalf("compliance member IsApproverForPR = %v (err %v), want true", ok, err)
	}
	// Holding a different team's role must not qualify.
	if ok, _ := repo.IsApproverForPR(ctx, pr.ID, officer.ID, officer.Email, []string{model.RecApprovalLegal}); ok {
		t.Fatal("legal role should not approve a compliance-only recommendation")
	}
	if ok, err := repo.HasApprovableWork(ctx, officer.ID, officer.Email, compTypes); err != nil || !ok {
		t.Fatalf("HasApprovableWork = %v (err %v), want true", ok, err)
	}

	// The PR appears in the officer's Approvals scope, pending their card.
	prs, err := repo.ListPurchaseRequests(ctx, officer.ID, officer.Email, false, false, compTypes,
		repository.PRScopeApprovals, repository.PRListFilter{})
	if err != nil {
		t.Fatalf("list approvals: %v", err)
	}
	var found *repository.PurchaseRequest
	for _, p := range prs {
		if p.ID == pr.ID {
			found = p
		}
	}
	if found == nil {
		t.Fatalf("PR %d missing from the compliance member's approvals list", pr.ID)
	}
	if found.MyApprovalState == nil || *found.MyApprovalState != "pending" {
		t.Fatalf("my_approval_state = %v, want pending", found.MyApprovalState)
	}

	// Assignee round-trips, and the pending card blocks quotation selection.
	if err := repo.SetRecAssignee(ctx, pr.ID, model.RecApprovalCompliance, &officer.ID); err != nil {
		t.Fatalf("set compliance assignee: %v", err)
	}
	rec, err := repo.GetRecommendation(ctx, pr.ID)
	if err != nil || rec == nil {
		t.Fatalf("get recommendation: %v", err)
	}
	card := findApprovalTest(rec, model.RecApprovalCompliance)
	if card == nil || card.AssigneeID == nil || *card.AssigneeID != officer.ID {
		t.Fatalf("compliance assignee not persisted: %+v", card)
	}
	if ok, _ := repo.RecommendationFullyApproved(ctx, pr.ID); ok {
		t.Fatal("fully approved with a pending compliance card")
	}
	if err := repo.SetRecApproval(ctx, pr.ID, model.RecApprovalCompliance, officer.ID); err != nil {
		t.Fatalf("approve compliance card: %v", err)
	}
	if ok, err := repo.RecommendationFullyApproved(ctx, pr.ID); err != nil || !ok {
		t.Fatalf("RecommendationFullyApproved = %v (err %v), want true", ok, err)
	}
	prs, err = repo.ListPurchaseRequests(ctx, officer.ID, officer.Email, false, false, compTypes,
		repository.PRScopeApprovals, repository.PRListFilter{})
	if err != nil {
		t.Fatalf("list approvals after approve: %v", err)
	}
	for _, p := range prs {
		if p.ID == pr.ID && (p.MyApprovalState == nil || *p.MyApprovalState != "approved") {
			t.Fatalf("my_approval_state after approve = %v, want approved", p.MyApprovalState)
		}
	}
}

// TestProcurementTeamAdminMembers verifies that the Procurement team surfaces
// procurement_admin holders in AdminMembers (they count as members for display)
// while base Members stays scoped to plain `procurement` holders, and that teams
// without an admin variant (legal/security/compliance) have no admin members.
func TestProcurementTeamAdminMembers(t *testing.T) {
	repo, ctx := newTestRepo(t)

	base, err := repo.UpsertUser(ctx, "proc-base-"+t.Name(), "proc-base-"+t.Name()+"@example.com", "Base")
	if err != nil {
		t.Fatalf("upsert base user: %v", err)
	}
	admin, err := repo.UpsertUser(ctx, "proc-admin-"+t.Name(), "proc-admin-"+t.Name()+"@example.com", "Admin")
	if err != nil {
		t.Fatalf("upsert admin user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = repo.Pool().Exec(context.Background(), `DELETE FROM users WHERE id = ANY($1)`, []int64{base.ID, admin.ID})
	})
	if err := repo.EnsureUserHasRole(ctx, base.ID, model.RoleProcurement); err != nil {
		t.Fatalf("grant procurement: %v", err)
	}
	if err := repo.EnsureUserHasRole(ctx, admin.ID, model.RoleProcurementAdmin); err != nil {
		t.Fatalf("grant procurement_admin: %v", err)
	}

	team, err := repo.GetTeamByKey(ctx, "procurement")
	if err != nil {
		t.Fatalf("get procurement team: %v", err)
	}
	if !containsUser(team.Members, base.ID) {
		t.Errorf("base procurement user %d missing from Members", base.ID)
	}
	if containsUser(team.Members, admin.ID) {
		t.Errorf("procurement_admin user %d should NOT be in base Members", admin.ID)
	}
	if !containsUser(team.AdminMembers, admin.ID) {
		t.Errorf("procurement_admin user %d missing from AdminMembers", admin.ID)
	}
	if containsUser(team.AdminMembers, base.ID) {
		t.Errorf("base procurement user %d should NOT be in AdminMembers", base.ID)
	}

	// Legal/security/compliance have no admin variant.
	for _, key := range []string{"legal", "security", "compliance"} {
		tm, err := repo.GetTeamByKey(ctx, key)
		if err != nil {
			t.Fatalf("get %s team: %v", key, err)
		}
		if len(tm.AdminMembers) != 0 {
			t.Errorf("%s team should have no admin members, got %d", key, len(tm.AdminMembers))
		}
	}
}

func containsUser(us []repository.UserSummary, id int64) bool {
	for _, u := range us {
		if u.ID == id {
			return true
		}
	}
	return false
}

func findApprovalTest(rec *repository.Recommendation, approvalType string) *repository.RecApproval {
	for i := range rec.Approvals {
		if rec.Approvals[i].ApprovalType == approvalType {
			return &rec.Approvals[i]
		}
	}
	return nil
}
