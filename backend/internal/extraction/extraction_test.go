package extraction

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// The tests below drive the real SDK against a fake Messages endpoint (via the
// Config.BaseURL override), so the request shape, SSE accumulation, JSON parsing
// and normalisation are all exercised without an API key or a live call.

// sseMessage renders a minimal streaming response whose single text block carries
// body — the shape a structured-output request produces.
func sseMessage(body, stopReason string) string {
	esc, _ := json.Marshal(body)
	var b strings.Builder
	fmt.Fprintf(&b, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-opus-5\",\"content\":[],\"stop_reason\":null,\"usage\":{\"input_tokens\":1234,\"output_tokens\":0}}}\n\n")
	fmt.Fprintf(&b, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
	fmt.Fprintf(&b, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":%s}}\n\n", esc)
	fmt.Fprintf(&b, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
	fmt.Fprintf(&b, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":%q},\"usage\":{\"output_tokens\":567}}\n\n", stopReason)
	fmt.Fprintf(&b, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	return b.String()
}

// newFakeService returns a Service pointed at a stub endpoint that replies with
// body, plus a pointer to the captured request payload.
func newFakeService(t *testing.T, body, stopReason string) (*Service, *map[string]any) {
	t.Helper()
	captured := map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		if err := json.Unmarshal(raw, &captured); err != nil {
			t.Errorf("unmarshal request body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, sseMessage(body, stopReason))
	}))
	t.Cleanup(srv.Close)

	svc := New(Config{
		Enabled: true,
		APIKey:  "test-key",
		Model:   "claude-opus-5",
		BaseURL: srv.URL,
	}, zerolog.Nop())
	return svc, &captured
}

func TestExtractDisabled(t *testing.T) {
	svc := New(Config{Enabled: false, APIKey: "x"}, zerolog.Nop())
	if svc.Enabled() {
		t.Fatal("service with Enabled=false reports enabled")
	}
	if _, err := svc.Extract(context.Background(), "q.pdf", []byte("%PDF-1.4")); err != ErrDisabled {
		t.Fatalf("Extract on disabled service = %v, want ErrDisabled", err)
	}

	// An empty key must also disable it, so a half-filled config can't produce
	// requests that fail at the API instead of being cleanly unavailable.
	svc = New(Config{Enabled: true, APIKey: "  "}, zerolog.Nop())
	if svc.Enabled() {
		t.Fatal("service with a blank api key reports enabled")
	}
}

func TestExtractTooLarge(t *testing.T) {
	svc := New(Config{Enabled: true, APIKey: "k", MaxPDFBytes: 10}, zerolog.Nop())
	_, err := svc.Extract(context.Background(), "q.pdf", []byte("more than ten bytes"))
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("Extract of oversized pdf = %v, want ErrTooLarge", err)
	}
}

func TestExtractRequestShape(t *testing.T) {
	svc, captured := newFakeService(t, `{"vendor_name":"Acme","items":[]}`, "end_turn")
	pdf := []byte("%PDF-1.7 fake bytes")
	if _, err := svc.Extract(context.Background(), "quote.pdf", pdf); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	req := *captured

	if req["model"] != "claude-opus-5" {
		t.Errorf("model = %v, want claude-opus-5", req["model"])
	}
	if req["stream"] != true {
		t.Error("request is not streaming; a large max_tokens on a non-streaming call risks an HTTP timeout")
	}

	// Structured outputs: the schema must be attached, or the model is free to
	// answer in prose and parsing becomes guesswork.
	oc, ok := req["output_config"].(map[string]any)
	if !ok {
		t.Fatalf("output_config missing: %v", req["output_config"])
	}
	format, ok := oc["format"].(map[string]any)
	if !ok || format["type"] != "json_schema" {
		t.Fatalf("output_config.format = %v, want a json_schema", oc["format"])
	}
	if _, ok := format["schema"].(map[string]any); !ok {
		t.Error("output_config.format.schema missing")
	}

	// Adaptive thinking, no budget_tokens (removed on this model family).
	thinking, ok := req["thinking"].(map[string]any)
	if !ok || thinking["type"] != "adaptive" {
		t.Errorf("thinking = %v, want type adaptive", req["thinking"])
	}
	if _, present := thinking["budget_tokens"]; present {
		t.Error("budget_tokens is set; it is rejected on claude-opus-5")
	}

	// The system prompt is the cached prefix.
	system, ok := req["system"].([]any)
	if !ok || len(system) == 0 {
		t.Fatalf("system = %v, want a non-empty block list", req["system"])
	}
	sys0 := system[0].(map[string]any)
	if _, ok := sys0["cache_control"].(map[string]any); !ok {
		t.Error("system block has no cache_control; the stable prefix should be cached")
	}

	// The PDF rides as a base64 document block ahead of the text instruction.
	messages := req["messages"].([]any)
	content := messages[0].(map[string]any)["content"].([]any)
	if len(content) != 2 {
		t.Fatalf("user content has %d blocks, want 2 (document + text)", len(content))
	}
	doc := content[0].(map[string]any)
	if doc["type"] != "document" {
		t.Fatalf("first content block is %v, want document", doc["type"])
	}
	src := doc["source"].(map[string]any)
	if src["media_type"] != "application/pdf" {
		t.Errorf("source.media_type = %v, want application/pdf", src["media_type"])
	}
	gotData, err := base64.StdEncoding.DecodeString(src["data"].(string))
	if err != nil {
		t.Fatalf("source.data is not valid base64: %v", err)
	}
	if string(gotData) != string(pdf) {
		t.Errorf("round-tripped pdf = %q, want %q", gotData, pdf)
	}
	if !strings.Contains(content[1].(map[string]any)["text"].(string), "quote.pdf") {
		t.Error("the filename hint is missing from the text block")
	}
}

func TestExtractParsesAndNormalizes(t *testing.T) {
	// Deliberately messy: lowercase currency, padded strings, and a blank-description
	// item that carries no meaning for a reviewer.
	body := `{
		"vendor_name": "  SUSE Software Solutions  ",
		"currency": "brl",
		"total_amount": 12500.5,
		"valid_until": "2026-09-30",
		"quote_reference": " Q-4471 ",
		"quote_date": "2026-07-28",
		"items": [
			{"description": " Patrocínio FEBRABAN ", "quantity": 1, "unit_price": 12000},
			{"description": "   ", "quantity": 3, "unit_price": 99},
			{"description": "Setup", "quantity": 2, "unit_price": 250.25}
		],
		"confidence": "HIGH",
		"notes": "  Tax handling unclear.  "
	}`
	svc, _ := newFakeService(t, body, "end_turn")
	res, err := svc.Extract(context.Background(), "suse.pdf", []byte("%PDF"))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	s := res.Suggestion

	if s.VendorName != "SUSE Software Solutions" {
		t.Errorf("VendorName = %q, want trimmed", s.VendorName)
	}
	if s.Currency != "BRL" {
		t.Errorf("Currency = %q, want BRL (upper-cased)", s.Currency)
	}
	if s.TotalAmount == nil || *s.TotalAmount != 12500.5 {
		t.Errorf("TotalAmount = %v, want 12500.5", s.TotalAmount)
	}
	if s.ValidUntil == nil || *s.ValidUntil != "2026-09-30" {
		t.Errorf("ValidUntil = %v, want 2026-09-30", s.ValidUntil)
	}
	if s.QuoteReference != "Q-4471" {
		t.Errorf("QuoteReference = %q, want trimmed", s.QuoteReference)
	}
	if s.Confidence != "high" {
		t.Errorf("Confidence = %q, want lower-cased high", s.Confidence)
	}
	if s.Notes != "Tax handling unclear." {
		t.Errorf("Notes = %q, want trimmed", s.Notes)
	}
	if len(s.Items) != 2 {
		t.Fatalf("kept %d items, want 2 (the blank-description row is dropped)", len(s.Items))
	}
	if s.Items[0].Description != "Patrocínio FEBRABAN" {
		t.Errorf("Items[0].Description = %q, want trimmed", s.Items[0].Description)
	}
	// 1×12000 + 2×250.25
	if got := s.ItemsTotal(); got != 12500.5 {
		t.Errorf("ItemsTotal() = %v, want 12500.5", got)
	}

	if res.InputTokens != 1234 || res.OutputTokens != 567 {
		t.Errorf("tokens = %d/%d, want 1234/567", res.InputTokens, res.OutputTokens)
	}
	if res.Model != "claude-opus-5" {
		t.Errorf("Model = %q", res.Model)
	}
	if len(res.RawJSON) == 0 {
		t.Error("RawJSON is empty; the verbatim output should be persisted")
	}
}

// TestExtractOmittedFieldsStayNil is the load-bearing guarantee: a field the model
// omits must surface as nil, never as a zero that looks like a real value. A
// defaulted 0 total would silently misstate a purchase.
func TestExtractOmittedFieldsStayNil(t *testing.T) {
	svc, _ := newFakeService(t, `{"vendor_name":"Acme","items":[]}`, "end_turn")
	res, err := svc.Extract(context.Background(), "q.pdf", []byte("%PDF"))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if res.Suggestion.TotalAmount != nil {
		t.Errorf("TotalAmount = %v, want nil when the document doesn't state one", *res.Suggestion.TotalAmount)
	}
	if res.Suggestion.ValidUntil != nil {
		t.Errorf("ValidUntil = %v, want nil", *res.Suggestion.ValidUntil)
	}
	if res.Suggestion.Currency != "" {
		t.Errorf("Currency = %q, want empty (not defaulted to USD here — the reviewer picks)", res.Suggestion.Currency)
	}
	if res.Suggestion.Items == nil {
		t.Error("Items should normalise to an empty slice, not nil, so the UI can render a table")
	}
}

func TestExtractRefusal(t *testing.T) {
	svc, _ := newFakeService(t, "", "refusal")
	_, err := svc.Extract(context.Background(), "q.pdf", []byte("%PDF"))
	if err == nil || !strings.Contains(err.Error(), "declined") {
		t.Fatalf("Extract on a refusal = %v, want a decline error", err)
	}
}

func TestExtractTruncated(t *testing.T) {
	svc, _ := newFakeService(t, "", "max_tokens")
	_, err := svc.Extract(context.Background(), "q.pdf", []byte("%PDF"))
	if err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("Extract on a truncated response = %v, want a truncation error", err)
	}
}

func TestExtractInvalidJSON(t *testing.T) {
	svc, _ := newFakeService(t, "not json at all", "end_turn")
	_, err := svc.Extract(context.Background(), "q.pdf", []byte("%PDF"))
	if err == nil || !strings.Contains(err.Error(), "parse extraction output") {
		t.Fatalf("Extract on malformed output = %v, want a parse error", err)
	}
}

// TestExtractReadsTotalsBlock covers the totals block: subtotal, discount, split
// tax lines and non-tax charges. Split taxes must stay separate rows — summing
// CGST+SGST (or ICMS+PIS+COFINS) into one line loses information the reviewer needs
// to reconcile the document.
func TestExtractReadsTotalsBlock(t *testing.T) {
	body := `{
		"vendor_name": "Acme GmbH",
		"currency": "EUR",
		"subtotal_amount": 1000,
		"discount_amount": 50,
		"taxes": [
			{"label": "  CGST ", "rate": 9, "amount": 90},
			{"label": "SGST", "rate": 9, "amount": 90},
			{"label": " Withholding ", "rate": 2, "amount": null},
			{"label": "  ", "rate": null, "amount": null}
		],
		"other_charges": [
			{"label": " Shipping ", "amount": 25},
			{"label": "", "amount": null}
		],
		"total_amount": 1205,
		"total_includes_tax": true,
		"valid_until": null,
		"quote_reference": "",
		"quote_date": null,
		"items": [{"description": "Widget", "quantity": 10, "unit_price": 100}],
		"confidence": "medium",
		"notes": ""
	}`
	svc, _ := newFakeService(t, body, "end_turn")
	res, err := svc.Extract(context.Background(), "q.pdf", []byte("%PDF"))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	s := res.Suggestion

	if s.SubtotalAmount == nil || *s.SubtotalAmount != 1000 {
		t.Errorf("SubtotalAmount = %v, want 1000", s.SubtotalAmount)
	}
	if s.DiscountAmount == nil || *s.DiscountAmount != 50 {
		t.Errorf("DiscountAmount = %v, want 50", s.DiscountAmount)
	}
	if s.TotalIncludesTax == nil || !*s.TotalIncludesTax {
		t.Errorf("TotalIncludesTax = %v, want true", s.TotalIncludesTax)
	}

	// Three tax rows survive; the fully blank one is dropped. The rate-only
	// "Withholding" row is kept — a stated rate with no printed amount is real
	// information for the reviewer.
	if len(s.Taxes) != 3 {
		t.Fatalf("kept %d tax rows, want 3: %+v", len(s.Taxes), s.Taxes)
	}
	if s.Taxes[0].Label != "CGST" || s.Taxes[1].Label != "SGST" {
		t.Errorf("tax labels = %q/%q, want trimmed CGST/SGST", s.Taxes[0].Label, s.Taxes[1].Label)
	}
	if s.Taxes[2].Amount != nil {
		t.Errorf("Taxes[2].Amount = %v, want nil (no amount printed)", *s.Taxes[2].Amount)
	}
	if got := s.TaxTotal(); got != 180 {
		t.Errorf("TaxTotal() = %v, want 180 (the amount-less row contributes nothing)", got)
	}

	if len(s.OtherCharges) != 1 || s.OtherCharges[0].Label != "Shipping" {
		t.Fatalf("OtherCharges = %+v, want one trimmed Shipping row", s.OtherCharges)
	}
	if got := s.ChargesTotal(); got != 25 {
		t.Errorf("ChargesTotal() = %v, want 25", got)
	}
}

// TestTotalsBlockOmissionsStayNil: a document with no tax section must not produce
// invented zeros — "no tax stated" and "tax of 0.00" are different facts.
func TestTotalsBlockOmissionsStayNil(t *testing.T) {
	svc, _ := newFakeService(t, `{"vendor_name":"Acme","items":[]}`, "end_turn")
	res, err := svc.Extract(context.Background(), "q.pdf", []byte("%PDF"))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	s := res.Suggestion
	if s.SubtotalAmount != nil || s.DiscountAmount != nil {
		t.Errorf("subtotal/discount = %v/%v, want nil", s.SubtotalAmount, s.DiscountAmount)
	}
	if s.TotalIncludesTax != nil {
		t.Errorf("TotalIncludesTax = %v, want nil when the document doesn't say", *s.TotalIncludesTax)
	}
	if s.Taxes == nil || len(s.Taxes) != 0 {
		t.Errorf("Taxes = %v, want an empty slice so the UI can render an empty block", s.Taxes)
	}
	if s.OtherCharges == nil || len(s.OtherCharges) != 0 {
		t.Errorf("OtherCharges = %v, want an empty slice", s.OtherCharges)
	}
}

// TestSchemaEnumsCarryNoType guards a live 400 we hit: the API's schema validator
// checks each enum value against a *single* declared type, so pairing "enum" with
// the ["string","null"] union every other field uses is rejected
// (`Enum value 'high' does not match declared type '["string","null"]'`). An enum
// alone — with null as a member — constrains the field fully.
func TestSchemaEnumsCarryNoType(t *testing.T) {
	var walk func(path string, node map[string]any)
	walk = func(path string, node map[string]any) {
		if _, hasEnum := node["enum"]; hasEnum {
			if typ, hasType := node["type"]; hasType {
				t.Errorf("%s declares both enum and type %v; the API rejects that combination", path, typ)
			}
		}
		if props, ok := node["properties"].(map[string]any); ok {
			for name, child := range props {
				if m, ok := child.(map[string]any); ok {
					walk(path+"."+name, m)
				}
			}
		}
		if items, ok := node["items"].(map[string]any); ok {
			walk(path+"[]", items)
		}
	}
	walk("schema", schema)

	// Every declared property must also be required: structured outputs treat
	// optionality as nullability, and a property missing from `required` is a
	// silent hole in the contract.
	props := schema["properties"].(map[string]any)
	required := map[string]bool{}
	for _, name := range schema["required"].([]string) {
		required[name] = true
	}
	for name := range props {
		if !required[name] {
			t.Errorf("property %q is not in the schema's required list", name)
		}
	}
	if len(required) != len(props) {
		t.Errorf("required lists %d names for %d properties", len(required), len(props))
	}
}
