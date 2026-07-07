package model

// Auto-advance rules for the procurement workflow. Each function returns the
// next purchase-request status and whether a transition should occur, given the
// PR's current status. Transitions only ever move a PR forward — callers apply
// them with a conditional UPDATE (WHERE status = <current>) so concurrent or
// out-of-order actions are idempotent no-ops rather than regressions.

// NextPRStatusForQuotationCreated advances a freshly-submitted PR into review
// once a quotation is associated with it.
func NextPRStatusForQuotationCreated(cur string) (string, bool) {
	if cur == StatusSubmitted {
		return StatusUnderReview, true
	}
	return cur, false
}

// NextPRStatusForQuotationSelected advances a PR to vendor_selected once a
// quotation has been picked.
func NextPRStatusForQuotationSelected(cur string) (string, bool) {
	if cur == StatusSubmitted || cur == StatusUnderReview {
		return StatusVendorSelected, true
	}
	return cur, false
}

// NextPRStatusForContractCreated advances a PR to contract_prepared once a
// contract is drafted from a quotation.
func NextPRStatusForContractCreated(cur string) (string, bool) {
	switch cur {
	case StatusSubmitted, StatusUnderReview, StatusVendorSelected:
		return StatusContractPrepared, true
	}
	return cur, false
}

// NextPRStatusForContractSigned advances a PR to order_signed once its contract
// is signed (terminal for this phase).
func NextPRStatusForContractSigned(cur string) (string, bool) {
	if cur == StatusContractPrepared {
		return StatusOrderSigned, true
	}
	return cur, false
}
