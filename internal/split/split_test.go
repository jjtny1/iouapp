package split

import "testing"

func sumItems(items []Item) int {
	var s int
	for _, it := range items {
		s += it.TotalCents
	}
	return s
}

// compute runs Compute for the common tax/tip-only case (no service charge,
// participants derived from claims), keeping the older tests concise.
func compute(items []Item, tax, tip int, claims map[string][]string) Summary {
	return Compute(Input{Items: items, TaxCents: tax, TipCents: tip, Claims: claims})
}

// checkInvariant asserts each participant's total is the sum of its parts and
// that participant totals plus the unclaimed remainder reconcile exactly to
// the bill's items + tax + tip + service charge.
func checkInvariant(t *testing.T, name string, items []Item, tax, tip int, sum Summary) {
	t.Helper()
	var totals int
	for _, ps := range sum.Participants {
		if ps.TotalCents != ps.ItemSubtotalCents+ps.TaxCents+ps.TipCents+ps.ServiceCents {
			t.Errorf("%s: participant %s total mismatch: %d != %d+%d+%d+%d",
				name, ps.ParticipantID, ps.TotalCents,
				ps.ItemSubtotalCents, ps.TaxCents, ps.TipCents, ps.ServiceCents)
		}
		totals += ps.TotalCents
	}
	want := sumItems(items) + tax + tip + sum.ServiceChargeCents
	if totals+sum.UnclaimedCents != want {
		t.Errorf("%s: invariant violated: %d + %d unclaimed = %d, want %d",
			name, totals, sum.UnclaimedCents, totals+sum.UnclaimedCents, want)
	}
	if sum.GrandTotalCents != want {
		t.Errorf("%s: grand total %d, want %d", name, sum.GrandTotalCents, want)
	}
}

func participant(sum Summary, id string) ParticipantShare {
	for _, ps := range sum.Participants {
		if ps.ParticipantID == id {
			return ps
		}
	}
	return ParticipantShare{}
}

func TestEvenSplit(t *testing.T) {
	items := []Item{{ID: "i1", TotalCents: 1000}}
	claims := map[string][]string{"i1": {"a", "b"}}
	sum := compute(items, 0, 0, claims)
	checkInvariant(t, "even", items, 0, 0, sum)
	if participant(sum, "a").ItemSubtotalCents != 500 || participant(sum, "b").ItemSubtotalCents != 500 {
		t.Errorf("even split wrong: %+v", sum.Participants)
	}
}

func TestIndivisibleSplit(t *testing.T) {
	items := []Item{{ID: "i1", TotalCents: 1001}}
	claims := map[string][]string{"i1": {"b", "a", "c"}}
	sum := compute(items, 0, 0, claims)
	checkInvariant(t, "indivisible", items, 0, 0, sum)
	// 1001/3 = 333 r2; sorted a,b,c -> a,b get 334, c gets 333.
	if got := participant(sum, "a").ItemSubtotalCents; got != 334 {
		t.Errorf("a got %d want 334", got)
	}
	if got := participant(sum, "b").ItemSubtotalCents; got != 334 {
		t.Errorf("b got %d want 334", got)
	}
	if got := participant(sum, "c").ItemSubtotalCents; got != 333 {
		t.Errorf("c got %d want 333", got)
	}
}

func TestUnclaimedItems(t *testing.T) {
	items := []Item{
		{ID: "i1", TotalCents: 1000},
		{ID: "i2", TotalCents: 500},
	}
	claims := map[string][]string{"i1": {"a"}}
	sum := compute(items, 0, 0, claims)
	checkInvariant(t, "unclaimed", items, 0, 0, sum)
	if sum.UnclaimedCents != 500 {
		t.Errorf("unclaimed = %d want 500", sum.UnclaimedCents)
	}
}

func TestZeroClaimersOverall(t *testing.T) {
	items := []Item{{ID: "i1", TotalCents: 1000}}
	sum := compute(items, 200, 150, map[string][]string{})
	checkInvariant(t, "zero claimers", items, 200, 150, sum)
	if sum.UnclaimedCents != 1350 {
		t.Errorf("unclaimed = %d want 1350", sum.UnclaimedCents)
	}
	if len(sum.Participants) != 0 {
		t.Errorf("expected no participants, got %d", len(sum.Participants))
	}
}

func TestNoParticipants(t *testing.T) {
	sum := compute(nil, 0, 0, nil)
	checkInvariant(t, "empty", nil, 0, 0, sum)
}

func TestTaxTipProrationExact(t *testing.T) {
	items := []Item{
		{ID: "i1", TotalCents: 700},
		{ID: "i2", TotalCents: 300},
	}
	claims := map[string][]string{"i1": {"a"}, "i2": {"b"}}
	// tax 10, tip 5 -> a subtotal 700, b 300. 70% / 30%.
	sum := compute(items, 10, 5, claims)
	checkInvariant(t, "proration", items, 10, 5, sum)
	var tax, tip int
	for _, ps := range sum.Participants {
		tax += ps.TaxCents
		tip += ps.TipCents
	}
	if tax != 10 {
		t.Errorf("tax sum = %d want 10", tax)
	}
	if tip != 5 {
		t.Errorf("tip sum = %d want 5", tip)
	}
}

func TestProrationLargestRemainder(t *testing.T) {
	// 3 participants each with subtotal 1, tax 2. Each gets 2/3 floored = 0,
	// leftover 2 pennies go to the two with largest remainder (all equal, so
	// by sorted ID: a, b).
	items := []Item{
		{ID: "i1", TotalCents: 1},
		{ID: "i2", TotalCents: 1},
		{ID: "i3", TotalCents: 1},
	}
	claims := map[string][]string{"i1": {"a"}, "i2": {"b"}, "i3": {"c"}}
	sum := compute(items, 2, 0, claims)
	checkInvariant(t, "largest remainder", items, 2, 0, sum)
	if participant(sum, "a").TaxCents != 1 || participant(sum, "b").TaxCents != 1 {
		t.Errorf("expected a and b to get extra penny: %+v", sum.Participants)
	}
	if participant(sum, "c").TaxCents != 0 {
		t.Errorf("expected c to get 0: %+v", sum.Participants)
	}
}

func TestSharedAndTaxTip(t *testing.T) {
	items := []Item{
		{ID: "i1", TotalCents: 2000},
		{ID: "i2", TotalCents: 1333},
		{ID: "i3", TotalCents: 999},
	}
	claims := map[string][]string{
		"i1": {"alice", "bob"},
		"i2": {"alice", "bob", "carol"},
		// i3 unclaimed
	}
	sum := compute(items, 777, 555, claims)
	checkInvariant(t, "shared+taxtip", items, 777, 555, sum)
	if sum.UnclaimedCents < 999 {
		t.Errorf("unclaimed should include i3 (999), got %d", sum.UnclaimedCents)
	}
}

// TestServicePercentProration: a percent service charge is prorated over
// claimers in proportion to what each ordered, exactly like tax.
func TestServicePercentProration(t *testing.T) {
	items := []Item{
		{ID: "i1", TotalCents: 700},
		{ID: "i2", TotalCents: 300},
	}
	sum := Compute(Input{
		Items:          items,
		Service:        ServiceCharge{Kind: ServicePercent, RateBps: 1000}, // 10%
		Claims:         map[string][]string{"i1": {"a"}, "i2": {"b"}},
		ParticipantIDs: []string{"a", "b"},
	})
	checkInvariant(t, "percent proration", items, 0, 0, sum)
	if sum.ServiceChargeCents != 100 {
		t.Errorf("service charge total = %d, want 100", sum.ServiceChargeCents)
	}
	if got := participant(sum, "a").ServiceCents; got != 70 {
		t.Errorf("a service = %d, want 70", got)
	}
	if got := participant(sum, "b").ServiceCents; got != 30 {
		t.Errorf("b service = %d, want 30", got)
	}
}

// TestServicePercentRoundsHalfUp checks the receipt example: 12.5% of 122.00.
func TestServicePercentRoundsHalfUp(t *testing.T) {
	items := []Item{{ID: "i1", TotalCents: 12200}}
	sum := Compute(Input{
		Items:          items,
		Service:        ServiceCharge{Kind: ServicePercent, RateBps: 1250},
		Claims:         map[string][]string{"i1": {"a"}},
		ParticipantIDs: []string{"a"},
	})
	checkInvariant(t, "percent rounding", items, 0, 0, sum)
	if sum.ServiceChargeCents != 1525 {
		t.Errorf("service charge total = %d, want 1525", sum.ServiceChargeCents)
	}
}

// TestServicePercentAllUnclaimed: with nothing claimed, a percent charge
// cannot be prorated and falls entirely to the unclaimed remainder.
func TestServicePercentAllUnclaimed(t *testing.T) {
	items := []Item{{ID: "i1", TotalCents: 1000}}
	sum := Compute(Input{
		Items:   items,
		Service: ServiceCharge{Kind: ServicePercent, RateBps: 1000},
		Claims:  map[string][]string{},
	})
	checkInvariant(t, "percent unclaimed", items, 0, 0, sum)
	if sum.UnclaimedCents != 1100 {
		t.Errorf("unclaimed = %d, want 1100 (1000 item + 100 service)", sum.UnclaimedCents)
	}
}

// TestServiceFixedEvenSplit: a fixed charge splits evenly across joined
// participants, including one who claimed nothing.
func TestServiceFixedEvenSplit(t *testing.T) {
	items := []Item{{ID: "i1", TotalCents: 100}}
	sum := Compute(Input{
		Items:          items,
		Service:        ServiceCharge{Kind: ServiceFixed, FixedCents: 1200},
		Claims:         map[string][]string{"i1": {"a"}},
		ParticipantIDs: []string{"a", "b", "c"},
	})
	checkInvariant(t, "fixed even", items, 0, 0, sum)
	for _, id := range []string{"a", "b", "c"} {
		if got := participant(sum, id).ServiceCents; got != 400 {
			t.Errorf("%s service = %d, want 400", id, got)
		}
	}
	if sum.UnclaimedCents != 0 {
		t.Errorf("unclaimed = %d, want 0", sum.UnclaimedCents)
	}
}

// TestServiceFixedHeadcountGap: a headcount larger than the joined count
// leaves the uncovered shares unclaimed.
func TestServiceFixedHeadcountGap(t *testing.T) {
	items := []Item{{ID: "i1", TotalCents: 100}}
	sum := Compute(Input{
		Items:          items,
		Service:        ServiceCharge{Kind: ServiceFixed, FixedCents: 1200, Headcount: 4},
		Claims:         map[string][]string{"i1": {"a"}},
		ParticipantIDs: []string{"a", "b"},
	})
	checkInvariant(t, "fixed headcount gap", items, 0, 0, sum)
	if got := participant(sum, "a").ServiceCents; got != 300 {
		t.Errorf("a service = %d, want 300", got)
	}
	if got := participant(sum, "b").ServiceCents; got != 300 {
		t.Errorf("b service = %d, want 300", got)
	}
	if sum.UnclaimedCents != 600 {
		t.Errorf("unclaimed = %d, want 600 (2 of 4 headcount shares)", sum.UnclaimedCents)
	}
}

// TestServiceFixedHeadcountClampedUp: a headcount below the joined count is
// clamped up so the bill never over-collects.
func TestServiceFixedHeadcountClampedUp(t *testing.T) {
	sum := Compute(Input{
		Service:        ServiceCharge{Kind: ServiceFixed, FixedCents: 600, Headcount: 1},
		ParticipantIDs: []string{"a", "b", "c"},
	})
	checkInvariant(t, "fixed clamp", nil, 0, 0, sum)
	for _, id := range []string{"a", "b", "c"} {
		if got := participant(sum, id).ServiceCents; got != 200 {
			t.Errorf("%s service = %d, want 200", id, got)
		}
	}
	if sum.UnclaimedCents != 0 {
		t.Errorf("unclaimed = %d, want 0", sum.UnclaimedCents)
	}
}

// TestServiceFixedNoParticipants: a fixed charge with nobody joined falls
// entirely to the unclaimed remainder.
func TestServiceFixedNoParticipants(t *testing.T) {
	items := []Item{{ID: "i1", TotalCents: 500}}
	sum := Compute(Input{
		Items:   items,
		Service: ServiceCharge{Kind: ServiceFixed, FixedCents: 1000},
		Claims:  map[string][]string{},
	})
	checkInvariant(t, "fixed no participants", items, 0, 0, sum)
	if sum.UnclaimedCents != 1500 {
		t.Errorf("unclaimed = %d, want 1500 (500 item + 1000 service)", sum.UnclaimedCents)
	}
}

// TestPartialClaimChargesOnlyClaimedItems is the no-overcharge guarantee: with
// items left unclaimed, a claimer pays tax, tip and a percent service charge
// only on what they themselves claimed. The rest stays unclaimed instead of
// inflating the claimer's bill.
func TestPartialClaimChargesOnlyClaimedItems(t *testing.T) {
	items := []Item{
		{ID: "i1", TotalCents: 5000}, // claimed by a
		{ID: "i2", TotalCents: 5000}, // unclaimed
	}
	sum := Compute(Input{
		Items:          items,
		TaxCents:       200,
		TipCents:       100,
		Service:        ServiceCharge{Kind: ServicePercent, RateBps: 1000}, // 10%
		Claims:         map[string][]string{"i1": {"a"}},
		ParticipantIDs: []string{"a"},
	})
	checkInvariant(t, "partial claim", items, 200, 100, sum)
	a := participant(sum, "a")
	// a claimed half the items, so owes half of each charge — not all of it.
	if a.TaxCents != 100 {
		t.Errorf("a tax = %d, want 100 (half of 200)", a.TaxCents)
	}
	if a.TipCents != 50 {
		t.Errorf("a tip = %d, want 50 (half of 100)", a.TipCents)
	}
	if a.ServiceCents != 500 {
		t.Errorf("a service = %d, want 500 (10%% of the 5000 a claimed)", a.ServiceCents)
	}
	if a.TotalCents != 5650 {
		t.Errorf("a total = %d, want 5650 (5000 items + 100 + 50 + 500)", a.TotalCents)
	}
	// The charges owed on the unclaimed item stay unclaimed: 5000 item +
	// 100 tax + 50 tip + 500 service.
	if sum.UnclaimedCents != 5650 {
		t.Errorf("unclaimed = %d, want 5650", sum.UnclaimedCents)
	}
}
