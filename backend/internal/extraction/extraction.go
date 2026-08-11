// Package extraction pulls structured quotation details out of a vendor's
// quotation PDF using the Anthropic API (Claude).
//
// The result is a *suggestion*, never an authoritative value: the caller stages it
// (quotation_extractions) and a procurement user reviews and applies it. See
// docs/plans/20-quotation-pdf-extraction.md.
//
// Like internal/directory, the service is config-gated — when disabled it reports
// Enabled() == false and every Extract call returns ErrDisabled, so dev and CI run
// without an API key.
package extraction

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/rs/zerolog"
)

// ErrDisabled is returned by Extract when no API key is configured.
var ErrDisabled = errors.New("quotation extraction is not configured")

// ErrTooLarge is returned when the PDF exceeds the configured size cap. The
// Anthropic API caps a request at 32MB total, and base64 inflates by ~4/3, so the
// effective ceiling is well under that; MaxPDFBytes keeps us clear of it.
var ErrTooLarge = errors.New("pdf is too large to extract")

// DefaultModel is the model used when config leaves anthropic.model empty.
const DefaultModel = "claude-opus-5"

// DefaultMaxPDFBytes caps the PDF we will send (20MB — comfortably inside the
// API's 32MB request limit once base64-encoded).
const DefaultMaxPDFBytes = 20 << 20

// DefaultTimeout bounds a single extraction call.
const DefaultTimeout = 3 * time.Minute

// Config configures the service. Enabled=false (or an empty APIKey) yields a
// disabled service.
type Config struct {
	Enabled     bool
	APIKey      string
	Model       string
	MaxPDFBytes int64
	Timeout     time.Duration
	BaseURL     string // optional override, for tests
}

// LineItem is one row of the quotation's line-item table as read from the PDF.
type LineItem struct {
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
}

// TaxLine is one tax row as printed on the quotation. A list rather than a single
// tax field because split taxes are normal: CGST + SGST in India, ICMS + PIS +
// COFINS in Brazil, VAT alongside a withholding line.
type TaxLine struct {
	Label  string   `json:"label"`  // "VAT", "GST", "ICMS", "Sales tax"
	Rate   *float64 `json:"rate"`   // percent, e.g. 20 for 20%
	Amount *float64 `json:"amount"`
}

// Charge is a non-tax charge line — shipping, freight, installation, handling.
type Charge struct {
	Label  string   `json:"label"`
	Amount *float64 `json:"amount"`
}

// Suggestion is the model's reading of the quotation. Every field is optional —
// the prompt instructs the model to omit rather than guess, so a nil pointer means
// "not stated in the document" and must not be defaulted silently by the caller.
//
// VendorName is deliberately a name and not an ID: the backend ranks candidates
// from the vendors table and the user picks. See repository.MatchVendors.
//
// SubtotalAmount / DiscountAmount / Taxes / OtherCharges are the totals block as
// printed. They are read for display only — a quotation row stores just the grand
// total, so the breakdown lives with the extraction (and differs between the initial
// and final PDF, which is why it is shown per document rather than on the quotation).
type Suggestion struct {
	VendorName       string     `json:"vendor_name"`
	Currency         string     `json:"currency"`
	SubtotalAmount   *float64   `json:"subtotal_amount"`
	DiscountAmount   *float64   `json:"discount_amount"`
	Taxes            []TaxLine  `json:"taxes"`
	OtherCharges     []Charge   `json:"other_charges"`
	TotalAmount      *float64   `json:"total_amount"`
	TotalIncludesTax *bool      `json:"total_includes_tax"`
	ValidUntil       *string    `json:"valid_until"`
	QuoteReference   string     `json:"quote_reference"`
	QuoteDate        *string    `json:"quote_date"`
	Items            []LineItem `json:"items"`
	Confidence       string     `json:"confidence"`
	Notes            string     `json:"notes"`
}

// TaxTotal sums the tax rows that stated an amount.
func (s Suggestion) TaxTotal() float64 {
	var t float64
	for _, tx := range s.Taxes {
		if tx.Amount != nil {
			t += *tx.Amount
		}
	}
	return t
}

// ChargesTotal sums the non-tax charge rows that stated an amount.
func (s Suggestion) ChargesTotal() float64 {
	var t float64
	for _, c := range s.OtherCharges {
		if c.Amount != nil {
			t += *c.Amount
		}
	}
	return t
}

// ItemsTotal sums the line items. Used to cross-check the stated total — the same
// non-blocking mismatch-warning pattern invoices already use for entered_total.
func (s Suggestion) ItemsTotal() float64 {
	var t float64
	for _, it := range s.Items {
		t += it.Quantity * it.UnitPrice
	}
	return t
}

// Result is a completed extraction: the parsed suggestion plus the metadata worth
// persisting (which model read the PDF, what it cost, and the raw JSON).
type Result struct {
	Suggestion   Suggestion
	Model        string
	InputTokens  int
	OutputTokens int
	RawJSON      []byte
}

// Service performs extractions. Safe for concurrent use.
type Service struct {
	client      *anthropic.Client // nil when disabled
	model       string
	maxPDFBytes int64
	timeout     time.Duration
	log         zerolog.Logger
}

// New builds a Service. When cfg.Enabled is false or cfg.APIKey is empty the
// Service is disabled and Extract returns ErrDisabled.
func New(cfg Config, log zerolog.Logger) *Service {
	s := &Service{
		model:       strings.TrimSpace(cfg.Model),
		maxPDFBytes: cfg.MaxPDFBytes,
		timeout:     cfg.Timeout,
		log:         log,
	}
	if s.model == "" {
		s.model = DefaultModel
	}
	if s.maxPDFBytes <= 0 {
		s.maxPDFBytes = DefaultMaxPDFBytes
	}
	if s.timeout <= 0 {
		s.timeout = DefaultTimeout
	}
	if !cfg.Enabled || strings.TrimSpace(cfg.APIKey) == "" {
		return s
	}
	opts := []option.RequestOption{option.WithAPIKey(cfg.APIKey)}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	c := anthropic.NewClient(opts...)
	s.client = &c
	return s
}

// Enabled reports whether extraction is configured.
func (s *Service) Enabled() bool { return s != nil && s.client != nil }

// Model returns the configured model id (useful for recording what produced a
// result, and for surfacing in the UI).
func (s *Service) Model() string {
	if s == nil {
		return ""
	}
	return s.model
}

// MaxPDFBytes is the size cap enforced by Extract.
func (s *Service) MaxPDFBytes() int64 {
	if s == nil {
		return DefaultMaxPDFBytes
	}
	return s.maxPDFBytes
}

// systemPrompt is deliberately stable — it is the cached prefix, so it must not
// interpolate anything per-request (see docs/plans/20-quotation-pdf-extraction.md).
const systemPrompt = `You extract structured data from vendor quotation PDFs for a corporate purchasing system.

Read the attached document and return the quotation's commercial details.

Rules:
- Report only what the document actually states. If a field is absent, omit it (or use null) — never infer, estimate, or default a value. A missing field is fine; a wrong one causes an incorrect purchase decision.
- vendor_name is the company ISSUING the quotation (the seller/supplier), not the recipient. Purchase requests in this system are received BY the buyer, so the buyer's own name may also appear on the page — do not report it as the vendor.
- currency must be the ISO 4217 code (USD, EUR, BRL, LKR, ...). Infer it from an explicit code, a currency symbol, or the document's language/locale only when unambiguous; otherwise omit it.
- Numbers must be plain decimals with a period separator. Documents may use European or Latin American formatting where "." groups thousands and "," is the decimal separator (e.g. "1.234,56" means 1234.56, and "R$ 12.500,00" means 12500.00). Normalise carefully — misreading a grouping separator as a decimal point changes the value by 1000x.
- total_amount is the final amount payable, after discounts and including tax when the document presents it that way. If the document shows a subtotal, tax and grand total, report the grand total.
- Report the totals block exactly as printed, and only the lines that are printed: subtotal_amount (the pre-tax net, after any discount, when the document shows one), discount_amount (as a positive number — the amount deducted, not the discounted total; if only a percentage is given, omit unless the money amount is also shown), other_charges (non-tax lines such as shipping, freight, installation or handling, one entry each with its printed label).
- taxes is one entry per tax line printed on the document. Split taxes are common and must stay separate — CGST and SGST, or ICMS, PIS and COFINS, are separate entries, never summed into one. Use the document's own label ("VAT", "GST", "ICMS", "Sales tax"). rate is the percentage as a number (20 for "20%"); amount is the money value. Give whichever the document prints and omit the other. If the document mentions no tax at all, return an empty list — do not compute a tax line yourself.
- total_includes_tax is true when the document says the total is tax-inclusive (or shows tax added into the grand total), false when tax is explicitly excluded or "plus taxes"/"tax extra" is stated, and null when the document doesn't say. This changes what the buyer actually pays, so do not guess it from the arithmetic alone unless the numbers make it unambiguous.
- items is the line-item table, one entry per priced row. Skip headings, subtotals, tax rows, discount rows, shipping rows and free-text notes — those belong in the fields above. If the document has no itemised table, return an empty list rather than inventing rows from prose.
- valid_until and quote_date are calendar dates in YYYY-MM-DD form. Beware day/month order: "05/03/2026" is ambiguous, so use it only when the document's locale or another date makes the order certain; otherwise omit. If validity is expressed as a duration ("valid for 30 days"), compute it from the quote date only when that date is stated — otherwise omit and mention it in notes.
- confidence reflects how legible and unambiguous the document was: "high" for a clean digital quotation, "medium" if you had to interpret layout or formatting, "low" for scans, partial text, or heavy guesswork.
- notes is a short free-text remark for the human reviewer: anything ambiguous, any assumption you made, or anything commercially important that has no field of its own (payment terms, delivery lead time, excluded scope). Leave it empty when there is nothing to flag.

The output is reviewed by a procurement user before it is saved, so flagging uncertainty in notes is always better than guessing.`

// schema is the JSON Schema constraining the response. Nullable throughout — see
// Suggestion. additionalProperties:false keeps the model from inventing fields.
var schema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"vendor_name": map[string]any{
			"type":        []string{"string", "null"},
			"description": "Name of the company issuing the quotation (the seller).",
		},
		"currency": map[string]any{
			"type":        []string{"string", "null"},
			"description": "ISO 4217 currency code, e.g. USD, EUR, BRL.",
		},
		"subtotal_amount": map[string]any{
			"type":        []string{"number", "null"},
			"description": "Pre-tax net total as printed, after any discount.",
		},
		"discount_amount": map[string]any{
			"type":        []string{"number", "null"},
			"description": "Discount deducted, as a positive number.",
		},
		"taxes": map[string]any{
			"type":        []string{"array", "null"},
			"description": "Tax lines as printed, one entry each. Split taxes (CGST/SGST, ICMS/PIS/COFINS) stay separate. Empty when the document states no tax.",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"label":  map[string]any{"type": "string", "description": "The document's own label, e.g. VAT, GST, ICMS."},
					"rate":   map[string]any{"type": []string{"number", "null"}, "description": "Percentage as a number, 20 for 20%."},
					"amount": map[string]any{"type": []string{"number", "null"}, "description": "Money value of this tax line."},
				},
				"required":             []string{"label", "rate", "amount"},
				"additionalProperties": false,
			},
		},
		"other_charges": map[string]any{
			"type":        []string{"array", "null"},
			"description": "Non-tax charge lines: shipping, freight, installation, handling.",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"label":  map[string]any{"type": "string"},
					"amount": map[string]any{"type": []string{"number", "null"}},
				},
				"required":             []string{"label", "amount"},
				"additionalProperties": false,
			},
		},
		"total_amount": map[string]any{
			"type":        []string{"number", "null"},
			"description": "Final amount payable stated on the quotation (grand total).",
		},
		"total_includes_tax": map[string]any{
			"type":        []string{"boolean", "null"},
			"description": "Whether the total is tax-inclusive. Null when the document doesn't say.",
		},
		"valid_until": map[string]any{
			"type":        []string{"string", "null"},
			"description": "Quotation validity end date, YYYY-MM-DD.",
		},
		"quote_reference": map[string]any{
			"type":        []string{"string", "null"},
			"description": "The vendor's own quotation/estimate number, if printed.",
		},
		"quote_date": map[string]any{
			"type":        []string{"string", "null"},
			"description": "Date the quotation was issued, YYYY-MM-DD.",
		},
		"items": map[string]any{
			"type":        []string{"array", "null"},
			"description": "Priced line items. Empty when the document has no itemised table.",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"description": map[string]any{"type": "string"},
					"quantity":    map[string]any{"type": "number"},
					"unit_price":  map[string]any{"type": "number"},
				},
				"required":             []string{"description", "quantity", "unit_price"},
				"additionalProperties": false,
			},
		},
		// No "type" here on purpose: the API's schema validator checks each enum
		// value against a *single* declared type, so pairing enum with the
		// ["string","null"] union 400s ("Enum value 'high' does not match declared
		// type"). The enum alone — null included — constrains the field fully.
		"confidence": map[string]any{
			"enum":        []any{"high", "medium", "low", nil},
			"description": "How legible and unambiguous the document was. Null if you cannot judge.",
		},
		"notes": map[string]any{
			"type":        []string{"string", "null"},
			"description": "Short remark for the human reviewer: ambiguities, assumptions, or commercially relevant details with no dedicated field.",
		},
	},
	"required": []string{"vendor_name", "currency", "subtotal_amount", "discount_amount", "taxes",
		"other_charges", "total_amount", "total_includes_tax", "valid_until", "quote_reference",
		"quote_date", "items", "confidence", "notes"},
	"additionalProperties": false,
}

// Extract reads the quotation details out of pdf. filename is used only to give
// the model a hint about the document; it is not trusted.
//
// The call streams: extraction on a long quotation can run for a while, and a
// large max_tokens on a non-streaming request risks an HTTP timeout.
func (s *Service) Extract(ctx context.Context, filename string, pdf []byte) (*Result, error) {
	if !s.Enabled() {
		return nil, ErrDisabled
	}
	if int64(len(pdf)) > s.maxPDFBytes {
		return nil, fmt.Errorf("%w: %d bytes (limit %d)", ErrTooLarge, len(pdf), s.maxPDFBytes)
	}
	if len(pdf) == 0 {
		return nil, errors.New("pdf is empty")
	}

	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	encoded := base64.StdEncoding.EncodeToString(pdf)
	name := strings.TrimSpace(filename)
	if name == "" {
		name = "quotation.pdf"
	}

	stream := s.client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(s.model),
		MaxTokens: 16000,
		// Stable prefix, cached: tools are unused, so the system block is the
		// whole cacheable prefix. The PDF follows it and varies per request.
		System: []anthropic.TextBlockParam{{
			Text:         systemPrompt,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		},
		OutputConfig: anthropic.OutputConfigParam{
			Format: anthropic.JSONOutputFormatParam{Schema: schema},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewDocumentBlock(anthropic.Base64PDFSourceParam{Data: encoded}),
				anthropic.NewTextBlock("Extract the quotation details from this document ("+name+")."),
			),
		},
	})

	var msg anthropic.Message
	for stream.Next() {
		if err := msg.Accumulate(stream.Current()); err != nil {
			return nil, fmt.Errorf("accumulate stream: %w", err)
		}
	}
	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("anthropic request: %w", err)
	}

	// A safety refusal is a successful HTTP response with an empty/partial body —
	// check the stop reason before reading content.
	if msg.StopReason == anthropic.StopReasonRefusal {
		return nil, errors.New("the model declined to process this document")
	}

	raw := firstTextBlock(msg)
	if raw == "" {
		if msg.StopReason == anthropic.StopReasonMaxTokens {
			return nil, errors.New("extraction output was truncated (document too long)")
		}
		return nil, errors.New("model returned no extraction output")
	}

	var sug Suggestion
	if err := json.Unmarshal([]byte(raw), &sug); err != nil {
		return nil, fmt.Errorf("parse extraction output: %w", err)
	}
	normalize(&sug)

	return &Result{
		Suggestion:   sug,
		Model:        s.model,
		InputTokens:  int(msg.Usage.InputTokens),
		OutputTokens: int(msg.Usage.OutputTokens),
		RawJSON:      []byte(raw),
	}, nil
}

// firstTextBlock returns the first text block's content. With output_config.format
// set, the response is a single text block holding the JSON object.
func firstTextBlock(msg anthropic.Message) string {
	for _, block := range msg.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			if s := strings.TrimSpace(t.Text); s != "" {
				return s
			}
		}
	}
	return ""
}

// normalize tidies the model's output for display: trims strings, uppercases the
// currency code, and drops line items with no description (which carry no meaning
// for a reviewer). It deliberately does NOT fill in missing values.
func normalize(s *Suggestion) {
	s.VendorName = strings.TrimSpace(s.VendorName)
	s.Currency = strings.ToUpper(strings.TrimSpace(s.Currency))
	s.QuoteReference = strings.TrimSpace(s.QuoteReference)
	s.Notes = strings.TrimSpace(s.Notes)
	s.Confidence = strings.ToLower(strings.TrimSpace(s.Confidence))

	items := make([]LineItem, 0, len(s.Items))
	for _, it := range s.Items {
		it.Description = strings.TrimSpace(it.Description)
		if it.Description == "" {
			continue
		}
		items = append(items, it)
	}
	s.Items = items

	// A tax or charge row with neither a label nor a number tells the reviewer
	// nothing, so drop it. A row with a label but no amount is kept — "VAT 20%,
	// amount not printed" is real information.
	taxes := make([]TaxLine, 0, len(s.Taxes))
	for _, tx := range s.Taxes {
		tx.Label = strings.TrimSpace(tx.Label)
		if tx.Label == "" && tx.Rate == nil && tx.Amount == nil {
			continue
		}
		taxes = append(taxes, tx)
	}
	s.Taxes = taxes

	charges := make([]Charge, 0, len(s.OtherCharges))
	for _, c := range s.OtherCharges {
		c.Label = strings.TrimSpace(c.Label)
		if c.Label == "" && c.Amount == nil {
			continue
		}
		charges = append(charges, c)
	}
	s.OtherCharges = charges
}
