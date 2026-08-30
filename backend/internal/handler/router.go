package handler

import (
	"net/http"

	"github.com/cs/purchasing-app/internal/crypto"
	"github.com/cs/purchasing-app/internal/directory"
	"github.com/cs/purchasing-app/internal/email"
	"github.com/cs/purchasing-app/internal/extraction"
	"github.com/cs/purchasing-app/internal/middleware"
	"github.com/cs/purchasing-app/internal/repository"
	"github.com/cs/purchasing-app/internal/storage"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
)

// Deps holds everything the router needs to wire handlers.
type Deps struct {
	Repo           *repository.Repository
	Storage        storage.Store
	StorageManager *storage.StorageManager
	Secrets        *crypto.Secretbox
	GDriveClientID string
	GDriveAPIKey   string
	GDriveAppID    string
	Auth           *middleware.AuthMiddleware
	Directory      *directory.Service
	Extraction     *extraction.Service
	Mailer         email.Mailer
	AppBaseURL     string
	AllowedOrigins []string
	Log            zerolog.Logger
}

func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID)         // stamp a request_id first...
	r.Use(middleware.RequestLogger(d.Log)) // ...so the access log and every request-scoped line carry it
	r.Use(chimiddleware.Recoverer)         // inside RequestLogger: a recovered panic still gets a 500 access line
	r.Use(middleware.CORS(d.AllowedOrigins))

	users := &UsersHandler{Repo: d.Repo, Dir: d.Directory, Log: d.Log}
	home := &HomeHandler{Repo: d.Repo, Log: d.Log}
	prs := &PurchaseRequestsHandler{Repo: d.Repo, Storage: d.Storage, Mailer: d.Mailer, Directory: d.Directory, AppBaseURL: d.AppBaseURL, Log: d.Log}
	vendors := &VendorsHandler{Repo: d.Repo, Log: d.Log}
	businessUnits := &BusinessUnitsHandler{Repo: d.Repo, Log: d.Log}
	quotes := &QuotationsHandler{Repo: d.Repo, Storage: d.Storage, Mailer: d.Mailer, Extraction: d.Extraction, Directory: d.Directory, AppBaseURL: d.AppBaseURL, Log: d.Log}
	contracts := &ContractsHandler{Repo: d.Repo, Storage: d.Storage, Log: d.Log}
	grns := &GRNsHandler{Repo: d.Repo, Storage: d.Storage, Log: d.Log}
	invoices := &InvoicesHandler{Repo: d.Repo, Storage: d.Storage, Log: d.Log}
	configOptions := &ConfigOptionsHandler{Repo: d.Repo, Log: d.Log}
	teams := &TeamsHandler{Repo: d.Repo, Log: d.Log}
	events := &EventsHandler{Repo: d.Repo, Log: d.Log}
	analytics := &AnalyticsHandler{Repo: d.Repo, Log: d.Log}
	storageH := &StorageHandler{
		Repo:     d.Repo,
		Manager:  d.StorageManager,
		Secrets:  d.Secrets,
		ClientID: d.GDriveClientID,
		APIKey:   d.GDriveAPIKey,
		AppID:    d.GDriveAppID,
		Log:      d.Log,
	}

	session := &SessionHandler{Repo: d.Repo, Session: d.Auth.SessionConfig(), Log: d.Log}
	backchannelLogout := &BackchannelLogoutHandler{Repo: d.Repo, Auth: d.Auth, Log: d.Log}

	r.Route("/api/v1", func(r chi.Router) {
		// OIDC back-channel logout — the ONLY route outside the authenticated
		// group. The caller is the IdP's server, which presents no cookie and no
		// Bearer token; the signed logout token in the body is the credential
		// (verified in middleware.VerifyLogoutToken). See docs/sessions.md.
		r.Post("/auth/backchannel-logout", backchannelLogout.Post)

		r.Group(func(r chi.Router) {
			r.Use(d.Auth.Authenticate)

			r.Get("/me", users.Me)

			// Backend-issued browser session (migration 052, docs/sessions.md).
			// Create sits inside this authenticated group on purpose: the caller
			// proves identity with the IdP token (or an existing cookie) and gets
			// back the long-lived cookie every later request uses.
			r.Post("/auth/session", session.Create)
			r.Delete("/auth/session", session.Delete)     // sign out here
			r.Delete("/auth/sessions", session.DeleteAll) // sign out everywhere

			// Role-based home dashboard: count tiles + recent-activity feeds,
			// scoped to the caller's roles (read-only aggregation).
			r.Get("/home", home.Get)

			// Lightweight active-user directory for the approver picker (any
			// authenticated user; returns id/email/name only).
			r.Get("/users/lookup", users.Lookup)

			// Org user directory for name/email autocomplete, sourced from the
			// connected identity server via SCIM (falls back to DB users when SCIM
			// is disabled). Any authenticated user; returns email/name only.
			r.Get("/users/directory", users.Directory)
			// Resolve a directory person to a provisioned app user (get-or-create
			// by email) so id-based pickers can use them. procurement_admin/admin.
			r.Post("/users/ensure", users.Ensure)

			// User management (admin only — enforced in the handlers)
			r.Get("/users", users.List)
			r.Post("/users", users.Create)
			r.Put("/users/{id}", users.Update)
			// End someone else's browser sessions on every device (admin only).
			// The manual lever for an IdP-side disable/offboard, since after the
			// cookie is minted the IdP is never consulted again.
			r.Delete("/users/{id}/sessions", session.DeleteForUser)
			r.Post("/users/{id}/roles", users.AddRole)
			r.Delete("/users/{id}/roles/{role}", users.RemoveRole)
			r.Put("/users/{id}/active", users.SetActive)

			// Purchase requests
			r.Get("/purchase-requests", prs.List)
			r.Post("/purchase-requests", prs.Create)
			r.Get("/purchase-requests/{id}", prs.Get)
			r.Put("/purchase-requests/{id}", prs.Update)
			r.Post("/purchase-requests/{id}/reject", prs.Reject)
			r.Post("/purchase-requests/{id}/documents", prs.UploadDocument)
			r.Get("/purchase-requests/{id}/documents/{docID}/download", prs.DownloadDocument)
			r.Delete("/purchase-requests/{id}/documents/{docID}", prs.DeleteDocument)
			r.Get("/purchase-requests/{id}/related", prs.Related)
			r.Get("/purchase-requests/{id}/quotations", quotes.ListForPR)
			r.Post("/purchase-requests/{id}/quotations", quotes.Create)
			// Extract a quotation PDF *before* its quotation exists: the staged
			// document is adopted into the initial-PDF slot when the returned
			// extraction_id is passed to the create call above.
			r.Post("/purchase-requests/{id}/quotation-extractions", quotes.ExtractForPR)
			// What each of this PR's quotation PDFs said, one entry per document —
			// the initial and final quotation carry separate figures.
			r.Get("/purchase-requests/{id}/quotation-extractions", quotes.ListExtractionsForPR)

			// Quotation comparison (one per PR): procurement generates it once
			// there are two or more quotations; every figure on it is derived at
			// read time, so it never goes stale. Readable by anyone who may view
			// the PR — the approvers it exists for have no access to the quotation
			// endpoints. See docs/quotation-comparison.md.
			r.Get("/purchase-requests/{id}/quotation-comparison", prs.GetQuotationComparison)
			r.Post("/purchase-requests/{id}/quotation-comparison", prs.GenerateQuotationComparison)
			r.Put("/purchase-requests/{id}/quotation-comparison", prs.UpdateQuotationComparison)
			r.Delete("/purchase-requests/{id}/quotation-comparison", prs.DeleteQuotationComparison)

			// Approvals: the requester manages approvers; an approver records
			// their own decision; the requester re-requests a rejected one.
			r.Post("/purchase-requests/{id}/approvers", prs.AddApprover)
			r.Delete("/purchase-requests/{id}/approvers/{approverID}", prs.RemoveApprover)
			r.Post("/purchase-requests/{id}/approvers/{approverID}/request", prs.RequestApprovalAgain)
			r.Post("/purchase-requests/{id}/approval", prs.RecordApprovalDecision)

			// PR assignment: after team-lead approval, a procurement user must be
			// assigned before procurement work can start. Procurement self-assigns;
			// procurement_admin assigns/reassigns anyone. Assignee/admin manage collaborators.
			r.Put("/purchase-requests/{id}/assignee", prs.SetAssignee)
			r.Post("/purchase-requests/{id}/collaborators", prs.AddCollaborator)
			r.Delete("/purchase-requests/{id}/collaborators/{userID}", prs.RemoveCollaborator)
			// Priority is procurement triage on the same card — any procurement user.
			r.Put("/purchase-requests/{id}/priority", prs.SetPriority)

			// Team lead approval: the requester's named team lead (or admin) approves
			// or rejects the PR — the gate that lets procurement see and act on it.
			r.Post("/purchase-requests/{id}/team-lead-approval", prs.TeamLeadDecision)
			r.Put("/purchase-requests/{id}/team-lead-email", prs.UpdateTeamLeadEmail)
			r.Post("/purchase-requests/{id}/team-lead-reminder", prs.RemindTeamLead)

			// Procurement reconciles the PR's named budget approver against the
			// designated one from the recommendation's budget card.
			r.Put("/purchase-requests/{id}/budget-approver", prs.SetBudgetApprover)

			// Procurement recommendation (one per PR): procurement creates/edits it;
			// the named actors (legal/security role, budget-owner) toggle approval
			// and comment on their card.
			r.Post("/purchase-requests/{id}/recommendation", prs.CreateRecommendation)
			r.Put("/purchase-requests/{id}/recommendation", prs.UpdateRecommendation)
			r.Delete("/purchase-requests/{id}/recommendation", prs.DeleteRecommendation)
			r.Post("/purchase-requests/{id}/recommendation/contract", prs.CreateRecommendationContract)
			r.Delete("/purchase-requests/{id}/recommendation/contract", prs.DeleteRecommendationContract)
			r.Put("/purchase-requests/{id}/recommendation/rfi", prs.SetRecommendationRFI)
			r.Delete("/purchase-requests/{id}/recommendation/rfi", prs.DeleteRecommendationRFI)
			r.Post("/purchase-requests/{id}/recommendation/rfi/documents", prs.UploadRecRFIDocument)
			r.Get("/purchase-requests/{id}/recommendation/rfi/documents/{docID}/download", prs.DownloadRecRFIDocument)
			r.Delete("/purchase-requests/{id}/recommendation/rfi/documents/{docID}", prs.DeleteRecRFIDocument)
			r.Post("/purchase-requests/{id}/recommendation/approvals/{type}", prs.SetRecApproval)
			// Request (add) or remove a single approval card without touching the others.
			r.Post("/purchase-requests/{id}/recommendation/approvals/{type}/request", prs.RequestRecApproval)
			r.Delete("/purchase-requests/{id}/recommendation/approvals/{type}", prs.RemoveRecApproval)
			r.Put("/purchase-requests/{id}/recommendation/approvals/{type}/assignee", prs.SetRecAssignee)
			r.Post("/purchase-requests/{id}/recommendation/approvals/{type}/assignee/remind", prs.RemindRecAssignee)
			r.Post("/purchase-requests/{id}/recommendation/approvals/budget/remind", prs.RemindBudgetApprovers)
			// Serial budget approval chain: procurement manages the additional steps;
			// each step's named approver (or the budget-unit approver for the base)
			// records the approve/reject/revert decision.
			r.Post("/purchase-requests/{id}/recommendation/budget-steps", prs.AddBudgetStep)
			r.Put("/purchase-requests/{id}/recommendation/budget-steps/{stepID}", prs.UpdateBudgetStep)
			r.Delete("/purchase-requests/{id}/recommendation/budget-steps/{stepID}", prs.DeleteBudgetStep)
			r.Post("/purchase-requests/{id}/recommendation/budget-steps/{stepID}/decision", prs.SetBudgetStepDecision)
			r.Post("/purchase-requests/{id}/recommendation/budget-steps/{stepID}/remind", prs.RemindBudgetStep)
			r.Post("/purchase-requests/{id}/recommendation/comments", prs.AddRecComment)
			r.Post("/purchase-requests/{id}/recommendation/comments/{commentID}/documents", prs.UploadRecCommentDocument)
			r.Get("/purchase-requests/{id}/recommendation/comments/{commentID}/documents/{docID}/download", prs.DownloadRecCommentDocument)
			r.Delete("/purchase-requests/{id}/recommendation/comments/{commentID}/documents/{docID}", prs.DeleteRecCommentDocument)

			// Vendors (management is procurement; the lookup is open to any
			// authenticated user for the PR "proposed supplier" dropdown).
			// Register /lookup before /{id} so it is not captured as an id.
			r.Get("/vendors/lookup", vendors.Lookup)
			r.Get("/vendors", vendors.List)
			r.Post("/vendors", vendors.Create)
			r.Get("/vendors/{id}", vendors.Get)
			r.Put("/vendors/{id}", vendors.Update)
			r.Get("/vendors/{id}/usage", vendors.Usage)

			// Business units (management is admin/procurement_admin; the lookup and
			// approver list are open to any authenticated user for the PR form).
			// Register /lookup before /{id} so it is not captured as an id.
			r.Get("/business-units/lookup", businessUnits.Lookup)
			r.Get("/business-units", businessUnits.List)
			r.Post("/business-units", businessUnits.Create)
			r.Get("/business-units/{id}", businessUnits.Get)
			r.Put("/business-units/{id}", businessUnits.Update)
			r.Get("/business-units/{id}/usage", businessUnits.Usage)
			r.Get("/business-units/{id}/invoices", businessUnits.Invoices)
			r.Get("/business-units/{id}/approvers", businessUnits.Approvers)

			// Configurable dropdown lists. The lookup is open to any authenticated
			// user (populates the requisition-form dropdowns); managing the lists
			// is admin/procurement_admin (enforced in the handlers). Register the
			// fixed paths before /{id} so they are not captured as an id.
			r.Get("/config/options/lookup", configOptions.Lookup)
			r.Get("/config/options", configOptions.List)
			r.Post("/config/options", configOptions.Create)
			r.Put("/config/options/{id}", configOptions.Update)
			r.Delete("/config/options/{id}", configOptions.Delete)

			// Teams (Legal/Security/Procurement): reading is open to any authenticated
			// user (the approval-card assignee dropdown needs membership); managing
			// members and the team email is procurement_admin/admin (enforced in handlers).
			r.Get("/teams", teams.List)
			r.Put("/teams/{key}/email", teams.UpdateEmail)
			r.Post("/teams/{key}/members", teams.AddMember)
			r.Delete("/teams/{key}/members/{userID}", teams.RemoveMember)

			// Audit/process events view (admin/procurement_admin — enforced in the
			// handlers). Read-only over the two append-only logs; the action lists
			// back the filter dropdowns.
			r.Get("/events/process", events.ListProcess)
			r.Get("/events/audit", events.ListAudit)
			r.Get("/events/actions", events.Actions)

			// BPM analytics (admin/procurement_admin — the same gate, over the
			// same process_events rows). Cross-organisation by design: the PR
			// list applies no visibility gate, and the per-PR flow does not go
			// through the gated PR read. See docs/bpm-analytics.md.
			r.Get("/analytics/purchase-requests", analytics.ListPRs)
			r.Get("/analytics/purchase-requests/{id}", analytics.PRFlow)

			// File storage configuration (admin only — enforced in the handlers).
			r.Get("/storage/status", storageH.Status)
			r.Get("/storage/params", storageH.Params)
			r.Post("/storage/connect", storageH.Connect)
			r.Post("/storage/folder", storageH.SetFolder)

			// Quotations
			r.Get("/quotations", quotes.List)
			r.Get("/quotations/{id}", quotes.Get)
			r.Put("/quotations/{id}", quotes.Update)
			r.Post("/quotations/{id}/select", quotes.Select)
			r.Delete("/quotations/{id}", quotes.Delete)
			r.Post("/quotations/{id}/initial-quotation-document", quotes.UploadInitialQuotationPDF)
			r.Delete("/quotations/{id}/initial-quotation-document", quotes.DeleteInitialQuotationPDF)
			r.Post("/quotations/{id}/final-quotation-document", quotes.UploadFinalQuotationPDF)
			r.Delete("/quotations/{id}/final-quotation-document", quotes.DeleteFinalQuotationPDF)
			r.Post("/quotations/{id}/documents", quotes.UploadDocument)
			r.Get("/quotations/{id}/documents/{docID}/download", quotes.DownloadDocument)
			r.Delete("/quotations/{id}/documents/{docID}", quotes.DeleteDocument)

			// Quotation PDF extraction (Claude). Reads vendor / currency / total /
			// validity / line items out of a quotation PDF and stages the result
			// for a procurement user to review and apply — nothing is written onto
			// the quotation automatically. See docs/quotation-extraction.md.
			// `status` is open to any authenticated user so the UI can hide the
			// feature when no API key is configured; the rest is procurement work.
			r.Get("/quotation-extractions/status", quotes.ExtractionStatus)
			r.Get("/quotation-extractions/{id}", quotes.GetExtraction)
			r.Post("/quotations/{id}/extract", quotes.ExtractForQuotation)

			// Contracts
			r.Get("/contracts", contracts.List)
			r.Get("/contracts/{id}", contracts.Get)
			r.Put("/contracts/{id}", contracts.Update)
			r.Post("/contracts/{id}/signed-document", contracts.UploadSignedDocument)
			r.Delete("/contracts/{id}/signed-document", contracts.DeleteSignedDocument)
			r.Post("/contracts/{id}/documents", contracts.UploadDocument)
			r.Put("/contracts/{id}/documents/{docID}", contracts.UpdateDocument)
			r.Get("/contracts/{id}/documents/{docID}/download", contracts.DownloadDocument)
			r.Delete("/contracts/{id}/documents/{docID}", contracts.DeleteDocument)

			// GRNs (goods received notes) — created against a signed contract
			r.Get("/contracts/{id}/grns", grns.ListForContract)
			r.Post("/contracts/{id}/grns", grns.Create)
			r.Get("/grns", grns.List)
			r.Get("/grns/{id}", grns.Get)
			r.Put("/grns/{id}", grns.Update)
			r.Delete("/grns/{id}", grns.Delete)
			r.Post("/grns/{id}/documents", grns.UploadDocument)
			r.Get("/grns/{id}/documents/{docID}/download", grns.DownloadDocument)
			r.Delete("/grns/{id}/documents/{docID}", grns.DeleteDocument)

			// Invoices — created against a signed contract
			r.Get("/contracts/{id}/invoices", invoices.ListForContract)
			r.Post("/contracts/{id}/invoices", invoices.Create)
			r.Get("/invoices", invoices.List)
			r.Get("/invoices/{id}", invoices.Get)
			r.Put("/invoices/{id}", invoices.Update)
			r.Put("/invoices/{id}/status", invoices.SetStatus)
			r.Delete("/invoices/{id}", invoices.Delete)
			r.Post("/invoices/{id}/documents", invoices.UploadDocument)
			r.Get("/invoices/{id}/documents/{docID}/download", invoices.DownloadDocument)
			r.Delete("/invoices/{id}/documents/{docID}", invoices.DeleteDocument)
		})
	})

	return r
}
