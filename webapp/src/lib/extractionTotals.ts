import type { ExtractionSuggestion } from "../types/api";

// Money arithmetic for a quotation's extracted totals block. Pure functions, no UI —
// the display components in components/ExtractedQuotationDetails.tsx and
// components/QuotationExtractionReview.tsx both read from here so the figure shown,
// the figure warned about, and the figure saved can't drift apart.

// Round through cents, so 283.2 doesn't come back as 283.20000000000005.
export const round2 = (n: number) => Math.round(n * 100) / 100;

export const sumTaxes = (s: ExtractionSuggestion) =>
  (s.taxes ?? []).reduce((n, t) => n + (t.amount ?? 0), 0);

export const sumCharges = (s: ExtractionSuggestion) =>
  (s.other_charges ?? []).reduce((n, c) => n + (c.amount ?? 0), 0);

export interface TotalReading {
  /** The amount payable including tax, or null if the document gave us nothing. */
  value: number | null;
  /** Tax was added to the figure the document printed as its total. */
  addedTax: boolean;
  /** No grand total was printed; this was built up from the breakdown. */
  derived: boolean;
  /** The figure printed as the total, when there was one. */
  stated: number | null;
}

/**
 * The total *including tax* — the amount the buyer actually pays, which is the only
 * total worth storing on a quotation.
 *
 * Most quotations print a tax-inclusive grand total and this returns it unchanged.
 * The interesting cases are the others:
 *
 *  - `total_includes_tax === false` ("plus taxes", "tax extra"): tax is added.
 *  - The flag is absent (the document didn't say) but the printed total equals a
 *    pre-tax base — the subtotal, or the subtotal less the discount. Arithmetic
 *    settles what the wording didn't, so tax is added.
 *  - No total printed at all: built from subtotal − discount + tax + charges.
 *
 * Anything else keeps the printed total: adding tax on top of an explicit grand total
 * would overstate the commitment, which is the more expensive way to be wrong.
 */
export function taxInclusiveTotal(s: ExtractionSuggestion, taxTotal?: number): TotalReading {
  const tax = taxTotal ?? sumTaxes(s);
  const charges = sumCharges(s);
  const stated = s.total_amount;
  const net = s.subtotal_amount;
  const discount = s.discount_amount ?? 0;

  if (stated == null) {
    if (net == null) return { value: null, addedTax: false, derived: false, stated };
    return {
      value: round2(net - discount + tax + charges),
      addedTax: tax > 0,
      derived: true,
      stated,
    };
  }
  if (tax === 0 || s.total_includes_tax === true) {
    return { value: stated, addedTax: false, derived: false, stated };
  }
  if (s.total_includes_tax === false) {
    return { value: round2(stated + tax), addedTax: true, derived: false, stated };
  }
  // The document didn't say. If its "total" is a pre-tax figure, tax still has to be
  // added; the discount may or may not already be in it, so both bases count.
  const preTaxBases = net == null ? [] : [net, round2(net - discount)];
  if (preTaxBases.some((b) => Math.abs(stated - b) < 0.01)) {
    return { value: round2(stated + tax), addedTax: true, derived: false, stated };
  }
  return { value: stated, addedTax: false, derived: false, stated };
}

/**
 * What the line items add up to once the document's own discount, taxes and charges
 * are applied — the figure a grand total should equal.
 *
 * The cross-check has to be made at the same level as the total: comparing a
 * tax-inclusive total against a pre-tax item sum would flag every taxed quotation as
 * a mismatch. With no discount, tax or charges this reduces to the plain item sum.
 */
export function reconciledTotal(
  s: ExtractionSuggestion,
  itemsTotal: number,
  taxTotal?: number,
): number {
  return round2(itemsTotal - (s.discount_amount ?? 0) + (taxTotal ?? sumTaxes(s)) + sumCharges(s));
}
