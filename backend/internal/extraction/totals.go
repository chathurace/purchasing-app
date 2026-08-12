package extraction

import "math"

// Money arithmetic for an extracted totals block.
//
// This is the Go twin of the frontend's webapp/src/lib/extractionTotals.ts, and the
// two must stay in step: the quotation cards read a PDF's figures through the TS
// version, while the quotation *comparison* is assembled server-side and reads them
// through this one. A divergence would show the same PDF as two different totals on
// the same page. Change one, change the other (and both test files).

// Round2 rounds through cents, so 283.2 doesn't come back as 283.20000000000005.
func Round2(n float64) float64 { return math.Round(n*100) / 100 }

// TotalReading is the outcome of reading a document's total: the amount payable and
// how it was arrived at.
type TotalReading struct {
	// Value is the amount payable including tax, or nil if the document gave us
	// nothing to work from.
	Value *float64 `json:"value"`
	// AddedTax: tax was added to the figure the document printed as its total.
	AddedTax bool `json:"added_tax"`
	// Derived: no grand total was printed; this was built from the breakdown.
	Derived bool `json:"derived"`
	// Stated is the figure printed as the total, when there was one.
	Stated *float64 `json:"stated"`
}

// TaxInclusiveTotal returns the total *including tax* — the amount the buyer
// actually pays, which is the only total worth storing on a quotation.
//
// Most quotations print a tax-inclusive grand total and this returns it unchanged.
// The interesting cases are the others:
//
//   - TotalIncludesTax == false ("plus taxes", "tax extra"): tax is added.
//   - The flag is absent (the document didn't say) but the printed total equals a
//     pre-tax base — the subtotal, or the subtotal less the discount. Arithmetic
//     settles what the wording didn't, so tax is added.
//   - No total printed at all: built from subtotal − discount + tax + charges.
//
// Anything else keeps the printed total: adding tax on top of an explicit grand
// total would overstate the commitment, which is the more expensive way to be wrong.
func TaxInclusiveTotal(s Suggestion, taxTotal float64) TotalReading {
	charges := s.ChargesTotal()
	stated := s.TotalAmount
	net := s.SubtotalAmount
	discount := 0.0
	if s.DiscountAmount != nil {
		discount = *s.DiscountAmount
	}

	if stated == nil {
		if net == nil {
			return TotalReading{Stated: stated}
		}
		v := Round2(*net - discount + taxTotal + charges)
		return TotalReading{Value: &v, AddedTax: taxTotal > 0, Derived: true, Stated: stated}
	}
	if taxTotal == 0 || (s.TotalIncludesTax != nil && *s.TotalIncludesTax) {
		return TotalReading{Value: stated, Stated: stated}
	}
	if s.TotalIncludesTax != nil && !*s.TotalIncludesTax {
		v := Round2(*stated + taxTotal)
		return TotalReading{Value: &v, AddedTax: true, Stated: stated}
	}
	// The document didn't say. If its "total" is a pre-tax figure, tax still has to
	// be added; the discount may or may not already be in it, so both bases count.
	if net != nil {
		for _, base := range []float64{*net, Round2(*net - discount)} {
			if math.Abs(*stated-base) < 0.01 {
				v := Round2(*stated + taxTotal)
				return TotalReading{Value: &v, AddedTax: true, Stated: stated}
			}
		}
	}
	return TotalReading{Value: stated, Stated: stated}
}

// ReconciledTotal is what the line items add up to once the document's own
// discount, taxes and charges are applied — the figure a grand total should equal.
//
// The cross-check has to be made at the same level as the total: comparing a
// tax-inclusive total against a pre-tax item sum would flag every taxed quotation
// as a mismatch. With no discount, tax or charges this reduces to the item sum.
func ReconciledTotal(s Suggestion, itemsTotal, taxTotal float64) float64 {
	discount := 0.0
	if s.DiscountAmount != nil {
		discount = *s.DiscountAmount
	}
	return Round2(itemsTotal - discount + taxTotal + s.ChargesTotal())
}
