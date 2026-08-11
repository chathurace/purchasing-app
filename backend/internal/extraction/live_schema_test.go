package extraction

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// TestLiveSchemaAccepted checks that the real API accepts `schema` and that the
// model populates the totals block. No PDF — a short text quotation stands in, so
// this is a contract check on the request, not an accuracy check on PDF parsing.
//
// It exists because a schema mistake is only visible live: a nullable enum paired
// with a union type is a 400 that no offline test can catch. Opt-in (it spends API
// credits) — skipped unless PURCHASING_LIVE_ANTHROPIC_KEY is set:
//
//	PURCHASING_LIVE_ANTHROPIC_KEY=sk-ant-... go test ./internal/extraction -run Live -v
func TestLiveSchemaAccepted(t *testing.T) {
	key := os.Getenv("PURCHASING_LIVE_ANTHROPIC_KEY")
	if key == "" {
		t.Skip("no live key")
	}
	c := anthropic.NewClient(option.WithAPIKey(key))
	stream := c.Messages.NewStreaming(context.Background(), anthropic.MessageNewParams{
		Model:     anthropic.Model(DefaultModel),
		MaxTokens: 4000,
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
			anthropic.NewUserMessage(anthropic.NewTextBlock(
				`Quotation QT-9001 from Acme Ltd, dated 2026-08-01, valid until 2026-09-15.
				 Widget x2 @ 50.00 = 100.00
				 Installation x1 @ 150.00 = 150.00
				 Subtotal 250,00
				 Discount 25,00
				 Shipping 15,00
				 CGST 9% 21,60
				 SGST 9% 21,60
				 Grand total EUR 283,20 (incl. taxes)`)),
		},
	})
	msg := anthropic.Message{}
	for stream.Next() {
		if err := msg.Accumulate(stream.Current()); err != nil {
			t.Fatalf("accumulate: %v", err)
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("live call rejected: %v", err)
	}
	raw := firstTextBlock(msg)
	t.Logf("stop_reason=%s raw=%s", msg.StopReason, raw)

	var s Suggestion
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	normalize(&s)
	if len(s.Taxes) != 2 {
		t.Errorf("got %d tax lines, want 2 (CGST + SGST kept separate): %+v", len(s.Taxes), s.Taxes)
	}
	if s.SubtotalAmount == nil || s.DiscountAmount == nil {
		t.Errorf("subtotal/discount not read: %v/%v", s.SubtotalAmount, s.DiscountAmount)
	}
	if len(s.OtherCharges) != 1 {
		t.Errorf("got %d charge lines, want 1 (shipping): %+v", len(s.OtherCharges), s.OtherCharges)
	}
	if s.TotalAmount == nil || *s.TotalAmount != 283.20 {
		t.Errorf("TotalAmount = %v, want 283.20 (comma decimals normalised)", s.TotalAmount)
	}
	if s.TotalIncludesTax == nil || !*s.TotalIncludesTax {
		t.Errorf("TotalIncludesTax = %v, want true", s.TotalIncludesTax)
	}
}
