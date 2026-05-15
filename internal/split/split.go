// Package split computes how a restaurant bill is divided among the friends
// who claimed each item, prorating tax, tip and the service charge exactly.
package split

import "sort"

// Item is a single claimable bill item; TotalCents is its full price.
type Item struct {
	ID         string
	TotalCents int
}

// Service charge kinds.
const (
	ServiceNone    = "none"
	ServicePercent = "percent"
	ServiceFixed   = "fixed"
)

// ServiceCharge is a bill's mandatory service fee and the rule for splitting it.
//
// Percent: RateBps (basis points, e.g. 1250 == 12.5%) is applied to the bill's
// full item subtotal. The resulting amount is prorated over claimers in
// proportion to what they ordered — exactly like tax — and the part owed on
// unclaimed items stays unclaimed.
//
// Fixed: FixedCents is divided into equal shares across the diner headcount.
// Each joined participant pays one share; if Headcount exceeds the number of
// joined participants the uncovered shares fall to UnclaimedCents. Headcount
// is clamped up to the joined-participant count so the bill never
// over-collects.
type ServiceCharge struct {
	Kind       string
	RateBps    int
	FixedCents int
	Headcount  int
}

// Input is everything Compute needs to split a bill.
type Input struct {
	Items          []Item
	TaxCents       int
	TipCents       int
	Service        ServiceCharge
	Claims         map[string][]string // itemID -> participant IDs claiming it
	ParticipantIDs []string            // every joined participant
}

// ParticipantShare is one participant's computed portion of the bill.
type ParticipantShare struct {
	ParticipantID     string `json:"participant_id"`
	ItemSubtotalCents int    `json:"item_subtotal_cents"`
	TaxCents          int    `json:"tax_cents"`
	TipCents          int    `json:"tip_cents"`
	ServiceCents      int    `json:"service_cents"`
	TotalCents        int    `json:"total_cents"`
}

// Summary is the full result of a split computation.
type Summary struct {
	Participants       []ParticipantShare `json:"participants"`
	UnclaimedCents     int                `json:"unclaimed_cents"`
	ServiceChargeCents int                `json:"service_charge_cents"`
	GrandTotalCents    int                `json:"grand_total_cents"`
}

// Compute splits items, tax, tip and the service charge among participants.
//
// Tax, tip and a percent service charge are prorated over the bill's full item
// subtotal: a participant pays them only in proportion to the items they
// actually claimed, so no one is ever charged for items someone else (or
// no one) ordered. The amount owed on unclaimed items stays in UnclaimedCents.
//
// The result satisfies the invariant:
//
//	sum(participant totals) + unclaimed ==
//	    sum(item totals) + tax + tip + service charge
func Compute(in Input) Summary {
	subtotals := map[string]int{}
	var itemSubtotal, claimedSubtotal int

	for _, it := range in.Items {
		itemSubtotal += it.TotalCents
		claimers := append([]string(nil), in.Claims[it.ID]...)
		sort.Strings(claimers)
		n := len(claimers)
		if n == 0 {
			continue
		}
		claimedSubtotal += it.TotalCents
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

	// unclaimed starts as the value of items nobody claimed; tax, tip and the
	// service charge each add back the portion owed on those items.
	unclaimed := itemSubtotal - claimedSubtotal

	// ids is every participant who claimed an item or joined the bill: a
	// joined participant who claimed nothing still owes a fixed service share.
	idset := map[string]bool{}
	for pid := range subtotals {
		idset[pid] = true
	}
	for _, pid := range in.ParticipantIDs {
		idset[pid] = true
	}
	ids := make([]string, 0, len(idset))
	for pid := range idset {
		ids = append(ids, pid)
	}
	sort.Strings(ids)

	taxShares, taxUnclaimed := prorate(ids, subtotals, itemSubtotal, in.TaxCents)
	tipShares, tipUnclaimed := prorate(ids, subtotals, itemSubtotal, in.TipCents)
	unclaimed += taxUnclaimed + tipUnclaimed

	serviceCents := serviceTotal(in.Service, itemSubtotal)
	serviceShares, serviceUnclaimed := splitService(ids, subtotals, itemSubtotal, in.Service, serviceCents)
	unclaimed += serviceUnclaimed

	shares := make([]ParticipantShare, 0, len(ids))
	for _, pid := range ids {
		ps := ParticipantShare{
			ParticipantID:     pid,
			ItemSubtotalCents: subtotals[pid],
			TaxCents:          taxShares[pid],
			TipCents:          tipShares[pid],
			ServiceCents:      serviceShares[pid],
		}
		ps.TotalCents = ps.ItemSubtotalCents + ps.TaxCents + ps.TipCents + ps.ServiceCents
		shares = append(shares, ps)
	}

	var grand int
	for _, ps := range shares {
		grand += ps.TotalCents
	}
	grand += unclaimed

	return Summary{
		Participants:       shares,
		UnclaimedCents:     unclaimed,
		ServiceChargeCents: serviceCents,
		GrandTotalCents:    grand,
	}
}

// serviceTotal resolves a service charge to a cent amount: a percent charge is
// RateBps applied to the item subtotal (rounded half up), a fixed charge is
// its stored amount.
func serviceTotal(sc ServiceCharge, itemSubtotal int) int {
	switch sc.Kind {
	case ServicePercent:
		if sc.RateBps <= 0 || itemSubtotal <= 0 {
			return 0
		}
		// RateBps is hundredths of a percent; (rate/10000)*subtotal, rounded.
		return (sc.RateBps*itemSubtotal + 5000) / 10000
	case ServiceFixed:
		if sc.FixedCents < 0 {
			return 0
		}
		return sc.FixedCents
	default:
		return 0
	}
}

// splitService distributes a service charge of total cents. A percent charge
// tracks the items, so it prorates exactly like tax — the share owed on
// unclaimed items stays unclaimed. A fixed charge divides into equal shares
// across the diner headcount, with shares beyond the joined participants left
// unclaimed. ids must be sorted.
func splitService(ids []string, subtotals map[string]int, itemSubtotal int, sc ServiceCharge, total int) (shares map[string]int, unclaimed int) {
	shares = map[string]int{}
	if total <= 0 {
		return shares, 0
	}
	switch sc.Kind {
	case ServicePercent:
		return prorate(ids, subtotals, itemSubtotal, total)
	case ServiceFixed:
		joined := len(ids)
		if joined == 0 {
			return shares, total
		}
		divisor := sc.Headcount
		if divisor < joined {
			divisor = joined
		}
		// Cut total into `divisor` equal integer shares via largest remainder.
		// The first `joined` shares go to participants in sorted order; any
		// remaining shares (headcount beyond the joined diners) are unclaimed.
		base := total / divisor
		rem := total % divisor
		for i := 0; i < divisor; i++ {
			s := base
			if i < rem {
				s++
			}
			if i < joined {
				shares[ids[i]] = s
			} else {
				unclaimed += s
			}
		}
		return shares, unclaimed
	default:
		return shares, 0
	}
}

// prorate distributes amount (tax, tip, or a percent service charge) across
// claimers in proportion to what each one claimed, measured against the full
// item subtotal. The portion of amount owed on unclaimed items is returned
// separately as unclaimed, so a claimer is never charged for items they did
// not claim. The largest-remainder method — with the unclaimed items as one
// extra bucket — makes the shares plus unclaimed sum exactly to amount.
// Leftover pennies go to the largest fractional remainders, ties broken by id
// (the unclaimed bucket, id "", sorts first). ids must be sorted.
func prorate(ids []string, subtotals map[string]int, itemSubtotal, amount int) (shares map[string]int, unclaimed int) {
	shares = map[string]int{}
	if amount == 0 {
		return shares, 0
	}
	if itemSubtotal <= 0 {
		// Nothing to attribute the amount to; it cannot be claimed.
		return shares, amount
	}

	// Each claimer is a bucket weighted by their claimed subtotal; the items
	// nobody claimed form one more bucket (id "").
	type bucket struct {
		id     string
		weight int
	}
	buckets := make([]bucket, 0, len(ids)+1)
	claimed := 0
	for _, pid := range ids {
		buckets = append(buckets, bucket{id: pid, weight: subtotals[pid]})
		claimed += subtotals[pid]
	}
	if rest := itemSubtotal - claimed; rest > 0 {
		buckets = append(buckets, bucket{id: "", weight: rest})
	}

	type frac struct {
		id        string
		remainder int
	}
	alloc := make(map[string]int, len(buckets))
	fracs := make([]frac, 0, len(buckets))
	allocated := 0
	for _, b := range buckets {
		num := amount * b.weight
		alloc[b.id] = num / itemSubtotal
		allocated += alloc[b.id]
		fracs = append(fracs, frac{id: b.id, remainder: num % itemSubtotal})
	}

	leftover := amount - allocated
	sort.SliceStable(fracs, func(i, j int) bool {
		if fracs[i].remainder != fracs[j].remainder {
			return fracs[i].remainder > fracs[j].remainder
		}
		return fracs[i].id < fracs[j].id
	})
	for i := 0; i < leftover && i < len(fracs); i++ {
		alloc[fracs[i].id]++
	}

	for _, pid := range ids {
		shares[pid] = alloc[pid]
	}
	return shares, alloc[""]
}
