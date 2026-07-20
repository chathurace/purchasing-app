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
	for _, key := range []string{"legal", "security", "procurement"} {
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

// TestProcurementTeamAdminMembers verifies that the Procurement team surfaces
// procurement_admin holders in AdminMembers (they count as members for display)
// while base Members stays scoped to plain `procurement` holders, and that teams
// without an admin variant (legal/security) have no admin members.
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

	// Legal/security have no admin variant.
	legal, err := repo.GetTeamByKey(ctx, "legal")
	if err != nil {
		t.Fatalf("get legal team: %v", err)
	}
	if len(legal.AdminMembers) != 0 {
		t.Errorf("legal team should have no admin members, got %d", len(legal.AdminMembers))
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
