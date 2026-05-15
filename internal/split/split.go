// Package split computes how a restaurant bill is divided among the friends
// who claimed each item, prorating tax and tip exactly.
package split

import "sort"

// Item is a single claimable bill item; TotalCents is its full price.
type Item struct {
	ID         string
	TotalCents int
}

// ParticipantShare is one participant's computed portion of the bill.
type ParticipantShare struct {
	ParticipantID     string `json:"participant_id"`
	ItemSubtotalCents int    `json:"item_subtotal_cents"`
	TaxCents          int    `json:"tax_cents"`
	TipCents          int    `json:"tip_cents"`
	TotalCents        int    `json:"total_cents"`
}

// Summary is the full result of a split computation.
type Summary struct {
	Participants    []ParticipantShare `json:"participants"`
	UnclaimedCents  int                `json:"unclaimed_cents"`
	GrandTotalCents int                `json:"grand_total_cents"`
}

// Compute splits items, tax and tip among claimers. claims maps an itemID to
// the participant IDs who claimed it. The result satisfies the invariant:
// sum(participant totals) + unclaimed == sum(item totals) + tax + tip.
func Compute(items []Item, taxCents, tipCents int, claims map[string][]string) Summary {
	subtotals := map[string]int{}
	var unclaimed int

	for _, it := range items {
		claimers := append([]string(nil), claims[it.ID]...)
		sort.Strings(claimers)
		n := len(claimers)
		if n == 0 {
			unclaimed += it.TotalCents
			continue
		}
		base := it.TotalCents / n
		rem := it.TotalCents % n
		for i, pid := range claimers {
			share := base
			if i < rem {
				share++
			}
			subtotals[pid] += share
		}
	}

	ids := make([]string, 0, len(subtotals))
	for pid := range subtotals {
		ids = append(ids, pid)
	}
	sort.Strings(ids)

	var claimedSubtotal int
	for _, pid := range ids {
		claimedSubtotal += subtotals[pid]
	}

	taxShares := prorate(ids, subtotals, claimedSubtotal, taxCents)
	tipShares := prorate(ids, subtotals, claimedSubtotal, tipCents)

	if claimedSubtotal == 0 {
		unclaimed += taxCents + tipCents
	}

	shares := make([]ParticipantShare, 0, len(ids))
	for _, pid := range ids {
		ps := ParticipantShare{
			ParticipantID:     pid,
			ItemSubtotalCents: subtotals[pid],
			TaxCents:          taxShares[pid],
			TipCents:          tipShares[pid],
		}
		ps.TotalCents = ps.ItemSubtotalCents + ps.TaxCents + ps.TipCents
		shares = append(shares, ps)
	}

	var grand int
	for _, ps := range shares {
		grand += ps.TotalCents
	}
	grand += unclaimed

	return Summary{
		Participants:    shares,
		UnclaimedCents:  unclaimed,
		GrandTotalCents: grand,
	}
}

// prorate distributes amount across participants proportional to their
// subtotals using the largest-remainder method so the result sums exactly to
// amount. Leftover pennies go to the largest fractional remainders, ties
// broken by sorted participant ID. ids must be sorted.
func prorate(ids []string, subtotals map[string]int, claimedSubtotal, amount int) map[string]int {
	out := map[string]int{}
	if claimedSubtotal == 0 || amount == 0 {
		return out
	}

	type frac struct {
		id        string
		remainder int
	}
	var fracs []frac
	allocated := 0
	for _, pid := range ids {
		num := amount * subtotals[pid]
		out[pid] = num / claimedSubtotal
		allocated += out[pid]
		fracs = append(fracs, frac{id: pid, remainder: num % claimedSubtotal})
	}

	leftover := amount - allocated
	sort.SliceStable(fracs, func(i, j int) bool {
		if fracs[i].remainder != fracs[j].remainder {
			return fracs[i].remainder > fracs[j].remainder
		}
		return fracs[i].id < fracs[j].id
	})
	for i := 0; i < leftover && i < len(fracs); i++ {
		out[fracs[i].id]++
	}
	return out
}
