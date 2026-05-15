package receipt

import "log"

// reconcileSlackCents is the tolerance, in cents, allowed when checking that a
// parsed receipt's parts sum to its printed total. It only absorbs a stray
// rounding cent — it is far smaller than any real mistake (a double-counted
// VAT line is off by the whole tax amount).
const reconcileSlackCents = 1

// reconcile checks that a parsed receipt's parts — item line totals, tax, tip
// and the service charge — sum to the grand total printed on the receipt, and
// corrects the most common parse error so the amounts match.
//
// That error is VAT: European receipts print VAT that is already included in
// the item prices, and the model sometimes reports it as tax added on top.
// Doing so overshoots the real total and overcharges every diner. When
// dropping the tax makes the receipt reconcile, reconcile zeroes it. Any
// mismatch it cannot explain is logged for the operator.
//
// With no printed total to check against, the receipt is returned unchanged.
func reconcile(r ParsedReceipt) ParsedReceipt {
	if r.GrandTotalCents <= 0 {
		return r
	}
	items := itemsTotal(r.Items)
	service := parsedServiceChargeCents(r.ServiceCharge, items)

	computed := items + r.TaxCents + r.TipCents + service
	if abs(computed-r.GrandTotalCents) <= reconcileSlackCents {
		return r // the parts already reconcile to the printed total
	}

	// Known fix: the reported tax is VAT already baked into the item prices,
	// so the items, tip and service charge alone equal the printed total.
	if r.TaxCents > 0 && abs(items+r.TipCents+service-r.GrandTotalCents) <= reconcileSlackCents {
		log.Printf("receipt: reconciliation dropped tax %d — items+tip+service already equal the printed total %d (VAT was included in the item prices)",
			r.TaxCents, r.GrandTotalCents)
		r.TaxCents = 0
		return r
	}

	log.Printf("receipt: reconciliation WARNING — parsed parts sum to %d but the receipt's printed total is %d; the bill will not match the receipt",
		computed, r.GrandTotalCents)
	return r
}

// itemsTotal sums a receipt's line totals (per-unit price times quantity).
func itemsTotal(items []ParsedItem) int {
	total := 0
	for _, it := range items {
		qty := it.Qty
		if qty < 1 {
			qty = 1
		}
		price := it.PriceCents
		if price < 0 {
			price = 0
		}
		total += price * qty
	}
	return total
}

// parsedServiceChargeCents resolves a parsed service charge to a cent amount,
// mirroring how the bill itself computes it (see split.serviceTotal): a
// percent rate is converted to basis points and applied to the item subtotal.
func parsedServiceChargeCents(sc ParsedServiceCharge, itemSubtotal int) int {
	switch sc.Kind {
	case "percent":
		if sc.Percent <= 0 || itemSubtotal <= 0 {
			return 0
		}
		rateBps := int(sc.Percent*100 + 0.5)
		return (rateBps*itemSubtotal + 5000) / 10000
	case "fixed":
		if sc.AmountCents < 0 {
			return 0
		}
		return sc.AmountCents
	}
	return 0
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
