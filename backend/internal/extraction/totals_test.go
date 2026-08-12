package extraction

import "testing"

// These cases mirror the frontend's lib/extractionTotals.ts contract: the quotation
// cards read a PDF's figures through the TS version and the comparison card through
// this one, so the same PDF must produce the same total on both sides.

func f(v float64) *float64 { return &v }
func b(v bool) *bool       { return &v }

func TestTaxInclusiveTotal(t *testing.T) {
	tests := []struct {
		name     string
		sug      Suggestion
		want     *float64
		addedTax bool
		derived  bool
	}{
		{
			name: "plain tax-inclusive total is returned unchanged",
			sug: Suggestion{
				SubtotalAmount: f(1000), TotalAmount: f(1200), TotalIncludesTax: b(true),
				Taxes: []TaxLine{{Label: "VAT", Amount: f(200)}},
			},
			want: f(1200),
		},
		{
			name: "no tax stated keeps the printed total",
			sug:  Suggestion{SubtotalAmount: f(1000), TotalAmount: f(1000)},
			want: f(1000),
		},
		{
			name: "tax explicitly excluded is added",
			sug: Suggestion{
				SubtotalAmount: f(1000), TotalAmount: f(1000), TotalIncludesTax: b(false),
				Taxes: []TaxLine{{Label: "VAT", Amount: f(180)}},
			},
			want: f(1180), addedTax: true,
		},
		{
			name: "silent document whose total equals the subtotal gets tax added",
			sug: Suggestion{
				SubtotalAmount: f(1000), TotalAmount: f(1000),
				Taxes: []TaxLine{{Label: "CGST", Amount: f(90)}, {Label: "SGST", Amount: f(90)}},
			},
			want: f(1180), addedTax: true,
		},
		{
			name: "silent document whose total equals subtotal less discount gets tax added",
			sug: Suggestion{
				SubtotalAmount: f(1000), DiscountAmount: f(100), TotalAmount: f(900),
				Taxes: []TaxLine{{Label: "VAT", Amount: f(162)}},
			},
			want: f(1062), addedTax: true,
		},
		{
			name: "silent document with a genuine grand total is left alone",
			sug: Suggestion{
				SubtotalAmount: f(1000), TotalAmount: f(1180),
				Taxes: []TaxLine{{Label: "VAT", Amount: f(180)}},
			},
			want: f(1180),
		},
		{
			name: "no total printed is built from the breakdown",
			sug: Suggestion{
				SubtotalAmount: f(1000), DiscountAmount: f(50),
				Taxes:        []TaxLine{{Label: "VAT", Amount: f(190)}},
				OtherCharges: []Charge{{Label: "Shipping", Amount: f(25)}},
			},
			want: f(1165), addedTax: true, derived: true,
		},
		{
			name: "nothing to work from returns nil",
			sug:  Suggestion{},
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := TaxInclusiveTotal(tc.sug, tc.sug.TaxTotal())
			switch {
			case tc.want == nil && got.Value != nil:
				t.Fatalf("value = %v, want nil", *got.Value)
			case tc.want != nil && got.Value == nil:
				t.Fatalf("value = nil, want %v", *tc.want)
			case tc.want != nil && *got.Value != *tc.want:
				t.Errorf("value = %v, want %v", *got.Value, *tc.want)
			}
			if got.AddedTax != tc.addedTax {
				t.Errorf("addedTax = %v, want %v", got.AddedTax, tc.addedTax)
			}
			if got.Derived != tc.derived {
				t.Errorf("derived = %v, want %v", got.Derived, tc.derived)
			}
		})
	}
}

func TestReconciledTotal(t *testing.T) {
	sug := Suggestion{
		DiscountAmount: f(100),
		Taxes:          []TaxLine{{Label: "VAT", Amount: f(180)}},
		OtherCharges:   []Charge{{Label: "Freight", Amount: f(20)}},
		Items:          []LineItem{{Description: "Widget", Quantity: 10, UnitPrice: 100}},
	}
	if got := ReconciledTotal(sug, sug.ItemsTotal(), sug.TaxTotal()); got != 1100 {
		t.Errorf("ReconciledTotal = %v, want 1100", got)
	}
	// With no discount, tax or charges it collapses to the plain item sum.
	plain := Suggestion{Items: []LineItem{{Quantity: 3, UnitPrice: 33.33}}}
	if got := ReconciledTotal(plain, plain.ItemsTotal(), 0); got != 99.99 {
		t.Errorf("ReconciledTotal = %v, want 99.99", got)
	}
}
