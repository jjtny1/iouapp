package split

import "testing"

func sumItems(items []Item) int {
	var s int
	for _, it := range items {
		s += it.TotalCents
	}
	return s
}

func checkInvariant(t *testing.T, name string, items []Item, tax, tip int, sum Summary) {
	t.Helper()
	var totals int
	for _, ps := range sum.Participants {
		if ps.TotalCents != ps.ItemSubtotalCents+ps.TaxCents+ps.TipCents {
			t.Errorf("%s: participant %s total mismatch: %d != %d+%d+%d",
				name, ps.ParticipantID, ps.TotalCents, ps.ItemSubtotalCents, ps.TaxCents, ps.TipCents)
		}
		totals += ps.TotalCents
	}
	want := sumItems(items) + tax + tip
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
	sum := Compute(items, 0, 0, claims)
	checkInvariant(t, "even", items, 0, 0, sum)
	if participant(sum, "a").ItemSubtotalCents != 500 || participant(sum, "b").ItemSubtotalCents != 500 {
		t.Errorf("even split wrong: %+v", sum.Participants)
	}
}

func TestIndivisibleSplit(t *testing.T) {
	items := []Item{{ID: "i1", TotalCents: 1001}}
	claims := map[string][]string{"i1": {"b", "a", "c"}}
	sum := Compute(items, 0, 0, claims)
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
	sum := Compute(items, 0, 0, claims)
	checkInvariant(t, "unclaimed", items, 0, 0, sum)
	if sum.UnclaimedCents != 500 {
		t.Errorf("unclaimed = %d want 500", sum.UnclaimedCents)
	}
}

func TestZeroClaimersOverall(t *testing.T) {
	items := []Item{{ID: "i1", TotalCents: 1000}}
	sum := Compute(items, 200, 150, map[string][]string{})
	checkInvariant(t, "zero claimers", items, 200, 150, sum)
	if sum.UnclaimedCents != 1350 {
		t.Errorf("unclaimed = %d want 1350", sum.UnclaimedCents)
	}
	if len(sum.Participants) != 0 {
		t.Errorf("expected no participants, got %d", len(sum.Participants))
	}
}

func TestNoParticipants(t *testing.T) {
	sum := Compute(nil, 0, 0, nil)
	checkInvariant(t, "empty", nil, 0, 0, sum)
}

func TestTaxTipProrationExact(t *testing.T) {
	items := []Item{
		{ID: "i1", TotalCents: 700},
		{ID: "i2", TotalCents: 300},
	}
	claims := map[string][]string{"i1": {"a"}, "i2": {"b"}}
	// tax 10, tip 5 -> a subtotal 700, b 300. 70% / 30%.
	sum := Compute(items, 10, 5, claims)
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
	sum := Compute(items, 2, 0, claims)
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
	sum := Compute(items, 777, 555, claims)
	checkInvariant(t, "shared+taxtip", items, 777, 555, sum)
	if sum.UnclaimedCents < 999 {
		t.Errorf("unclaimed should include i3 (999), got %d", sum.UnclaimedCents)
	}
}
