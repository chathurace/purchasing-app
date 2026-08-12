package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/cs/purchasing-app/internal/extraction"
	"github.com/cs/purchasing-app/internal/middleware"
	"github.com/cs/purchasing-app/internal/model"
	"github.com/cs/purchasing-app/internal/repository"
	"github.com/jackc/pgx/v5"
)

// Quotation PDF extraction endpoints. An extraction is a *suggestion* — it is
// staged in quotation_extractions and applied only when a procurement user
// confirms it. See docs/quotation-extraction.md.

// extractionResponse is what the UI renders: the staged row, the parsed suggestion,
// ranked vendor candidates for the extracted name, and the line-item total so the
// client can show the same non-blocking mismatch warning invoices use.
// ItemsTotal / TaxTotal / ChargesTotal are computed server-side so the client shows
// the same arithmetic everywhere; the *stated* total still wins over any of them.
type extractionResponse struct {
	Extraction    *repository.QuotationExtraction `json:"extraction"`
	Suggestion    *extraction.Suggestion          `json:"suggestion,omitempty"`
	VendorMatches []repository.VendorMatch        `json:"vendor_matches"`
	ItemsTotal    float64                         `json:"items_total"`
	TaxTotal      float64                         `json:"tax_total"`
	ChargesTotal  float64                         `json:"charges_total"`
}

// ExtractionStatus reports whether extraction is available, so the UI can hide the
// feature instead of offering a button that 503s. Any authenticated user.
func (h *QuotationsHandler) ExtractionStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":       h.Extraction.Enabled(),
		"model":         h.Extraction.Model(),
		"max_pdf_bytes": h.Extraction.MaxPDFBytes(),
	})
}

// ExtractForPR stages a quotation PDF against a purchase request and extracts its
// details *before* any quotation exists — the pre-create flow. The stored document
// is adopted into the quotation's initial-PDF slot when the quotation is created
// with this extraction's id, so the client never uploads the same bytes twice.
func (h *QuotationsHandler) ExtractForPR(w http.ResponseWriter, r *http.Request) {
	if !h.extractionAvailable(w) {
		return
	}
	if !middleware.HasProcurementAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "procurement access required")
		return
	}
	prID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request id")
		return
	}
	pr, err := h.Repo.GetPurchaseRequest(r.Context(), prID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "request not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load request")
		return
	}
	// Same gates as quotation-create: extraction is procurement work on this PR.
	if !model.IsTeamLeadApproved(pr.TeamLeadStatus) {
		writeError(w, http.StatusConflict, "purchase request is awaiting team lead approval")
		return
	}
	if code, msg, ok := assignmentWorkGate(r, pr); !ok {
		writeError(w, code, msg)
		return
	}

	// Store the PDF up front (under the PR's directory, as every document is) so a
	// failed extraction still leaves a re-runnable artifact rather than losing the
	// upload. owner_id is filled in once the extraction row exists.
	doc, ok := saveUploadedDoc(w, r, h.Repo, h.Storage, prID, model.OwnerQuotationExtraction, 0, pdfOnly)
	if !ok {
		return
	}
	h.runExtraction(w, r, prID, doc, nil, model.QualifierStaged)
}

// ExtractForQuotation re-reads one of an existing quotation's two primary PDFs —
// the post-create flow, for quotations created by hand and for a final
// (post-negotiation) PDF whose numbers differ from the initial one.
func (h *QuotationsHandler) ExtractForQuotation(w http.ResponseWriter, r *http.Request) {
	if !h.extractionAvailable(w) {
		return
	}
	q, ok := h.load(w, r)
	if !ok {
		return
	}
	slot := repository.QuotationDocInitial
	qualifier := model.QualifierInitial
	if strings.EqualFold(r.URL.Query().Get("slot"), "final") {
		slot, qualifier = repository.QuotationDocFinal, model.QualifierFinal
	}
	docID := q.InitialQuotationDocumentID
	if slot == repository.QuotationDocFinal {
		docID = q.FinalQuotationDocumentID
	}
	if docID == nil {
		writeError(w, http.StatusConflict, "no "+qualifier+" quotation PDF is attached")
		return
	}
	doc, err := h.Repo.GetOwnedDocument(r.Context(), model.OwnerQuotation, q.ID, *docID)
	if err != nil {
		writeError(w, http.StatusNotFound, "quotation PDF not found")
		return
	}
	h.runExtraction(w, r, q.PurchaseRequestID, doc, &q.ID, qualifier)
}

// ListExtractionsForPR returns every succeeded extraction under a purchase request
// so the PR page can show what each quotation PDF said — **per PDF slot**, since the
// initial and final quotation of the same vendor legitimately carry different
// figures and line items. Gated like ListForPR (procurement access): it is the same
// procurement view of the same PR, and the caller can already download the PDFs.
//
// Vendor matches are deliberately not computed here. They exist to help *pick* a
// vendor while applying a suggestion; this is a read of results, and ranking vendors
// for every extraction on the page would be a query each.
func (h *QuotationsHandler) ListExtractionsForPR(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasProcurementAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "procurement access required")
		return
	}
	prID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request id")
		return
	}
	rows, err := h.Repo.ListSucceededExtractionsForPR(r.Context(), prID)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("list quotation extractions for pr")
		writeError(w, http.StatusInternalServerError, "failed to list extractions")
		return
	}
	out := make([]extractionResponse, 0, len(rows))
	for _, ext := range rows {
		out = append(out, h.decorate(r, ext, false))
	}
	writeJSON(w, http.StatusOK, out)
}

// GetExtraction returns a staged extraction (procurement access; must belong to a
// PR the caller may work on).
func (h *QuotationsHandler) GetExtraction(w http.ResponseWriter, r *http.Request) {
	if !middleware.HasProcurementAccess(r.Context()) {
		writeError(w, http.StatusForbidden, "procurement access required")
		return
	}
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid extraction id")
		return
	}
	ext, err := h.Repo.GetExtraction(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "extraction not found")
		return
	}
	writeJSON(w, http.StatusOK, h.buildResponse(r, ext))
}

// runExtraction reads the stored PDF, calls Claude, records the outcome, and
// returns the suggestion. The staged row is written before the call so a failure is
// recorded rather than lost, and the document is re-owned to the row so it is
// traceable while it waits to be adopted.
func (h *QuotationsHandler) runExtraction(
	w http.ResponseWriter, r *http.Request,
	prID int64, doc *repository.Document, quotationID *int64, qualifier string,
) {
	user := middleware.UserFromCtx(r.Context())
	ext, err := h.Repo.CreateExtraction(r.Context(), prID, doc.ID, quotationID, h.Extraction.Model(), user.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("stage quotation extraction")
		writeError(w, http.StatusInternalServerError, "failed to stage extraction")
		return
	}
	if quotationID == nil {
		// Point the staging document at its extraction row (it was saved with a
		// placeholder owner_id, since the row didn't exist yet).
		if err := h.Repo.SetDocumentOwner(r.Context(), doc.ID, model.OwnerQuotationExtraction, ext.ID); err != nil {
			reqLog(r).Warn().Err(err).Int64("doc_id", doc.ID).Msg("set extraction document owner")
		}
	}

	pdf, err := h.readDocument(doc)
	if err != nil {
		h.failExtraction(r, ext.ID, "could not read the stored PDF")
		reqLog(r).Error().Err(err).Str("path", doc.StoredPath).Msg("read quotation pdf")
		writeError(w, http.StatusInternalServerError, "failed to read the stored PDF")
		return
	}

	res, err := h.Extraction.Extract(r.Context(), doc.Filename, pdf)
	if err != nil {
		msg := "extraction failed"
		code := http.StatusBadGateway
		switch {
		case errors.Is(err, extraction.ErrTooLarge):
			msg, code = "this PDF is too large to extract", http.StatusRequestEntityTooLarge
		case errors.Is(err, extraction.ErrDisabled):
			msg, code = "quotation extraction is not configured", http.StatusServiceUnavailable
		default:
			// Surface the model-side reason (refusal, truncation) — it tells the
			// user whether retrying is worthwhile.
			msg = err.Error()
		}
		h.failExtraction(r, ext.ID, msg)
		reqLog(r).Error().Err(err).Int64("extraction_id", ext.ID).Msg("extract quotation pdf")
		writeError(w, code, msg)
		return
	}

	if err := h.Repo.FinishExtraction(r.Context(), ext.ID, repository.ExtractionSucceeded,
		res.RawJSON, res.InputTokens, res.OutputTokens, ""); err != nil {
		reqLog(r).Error().Err(err).Msg("record quotation extraction")
		writeError(w, http.StatusInternalServerError, "failed to record extraction")
		return
	}
	recordProcessEvent(r, h.Repo, prID, model.ProcessExtractQuotation, qualifier)
	ensureCollaborator(r, h.Repo, prID)

	ext, err = h.Repo.GetExtraction(r.Context(), ext.ID)
	if err != nil {
		reqLog(r).Error().Err(err).Msg("reload extraction")
		writeError(w, http.StatusInternalServerError, "failed to reload extraction")
		return
	}
	reqLog(r).Info().
		Int64("extraction_id", ext.ID).
		Int("input_tokens", res.InputTokens).
		Int("output_tokens", res.OutputTokens).
		Msg("quotation extracted")
	writeJSON(w, http.StatusOK, h.buildResponse(r, ext))
}

// failExtraction records a failed attempt, best-effort — the HTTP error is what the
// caller acts on.
func (h *QuotationsHandler) failExtraction(r *http.Request, id int64, msg string) {
	if err := h.Repo.FinishExtraction(r.Context(), id, repository.ExtractionFailed, nil, 0, 0, msg); err != nil {
		reqLog(r).Warn().Err(err).Int64("extraction_id", id).Msg("record extraction failure")
	}
}

// readDocument loads a stored document's bytes, bounded by the extraction size cap
// so a huge file can't balloon memory before Extract rejects it.
func (h *QuotationsHandler) readDocument(doc *repository.Document) ([]byte, error) {
	f, err := h.Storage.Open(doc.StoredPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, h.Extraction.MaxPDFBytes()+1))
}

// buildResponse decorates a staged row with the parsed suggestion and ranked vendor
// candidates — the shape returned when the user is about to review and apply.
func (h *QuotationsHandler) buildResponse(r *http.Request, ext *repository.QuotationExtraction) extractionResponse {
	return h.decorate(r, ext, true)
}

// decorate parses a staged row's stored JSON into a suggestion. withVendorMatches
// controls whether existing vendors are ranked against the extracted name — worth a
// query when the user is choosing a vendor, wasted on a read-only display.
// A failed or pending row carries no suggestion.
func (h *QuotationsHandler) decorate(r *http.Request, ext *repository.QuotationExtraction, withVendorMatches bool) extractionResponse {
	out := extractionResponse{Extraction: ext, VendorMatches: []repository.VendorMatch{}}
	if ext.Status != repository.ExtractionSucceeded || len(ext.RawJSON) == 0 {
		return out
	}
	var sug extraction.Suggestion
	if err := json.Unmarshal(ext.RawJSON, &sug); err != nil {
		reqLog(r).Warn().Err(err).Int64("extraction_id", ext.ID).Msg("parse stored extraction json")
		return out
	}
	out.Suggestion = &sug
	out.ItemsTotal = sug.ItemsTotal()
	out.TaxTotal = sug.TaxTotal()
	out.ChargesTotal = sug.ChargesTotal()
	if withVendorMatches && sug.VendorName != "" {
		matches, err := h.Repo.MatchVendors(r.Context(), sug.VendorName, 5)
		if err != nil {
			reqLog(r).Warn().Err(err).Msg("match vendors for extraction")
		} else {
			out.VendorMatches = matches
		}
	}
	return out
}

// extractionAvailable guards the endpoints when no API key is configured.
func (h *QuotationsHandler) extractionAvailable(w http.ResponseWriter) bool {
	if !h.Extraction.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "quotation extraction is not configured")
		return false
	}
	return true
}
