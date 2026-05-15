package receipt

import "fmt"

// FlatItem is a single claimable unit from a receipt: one physical thing
// with no quantity. Each unit is its own item so it can be claimed
// independently by whoever ordered it.
type FlatItem struct {
	Name       string
	PriceCents int
}

// Flatten expands parsed receipt lines into one FlatItem per unit. A line
// with quantity N>1 becomes N items named "Name (1 of N)" … "Name (N of N)"
// so each person who ordered one can claim their own; a quantity-1 line
// keeps its name unchanged. price_cents is per unit, so every unit inherits
// the line's price.
func Flatten(items []ParsedItem) []FlatItem {
	out := make([]FlatItem, 0, len(items))
	for _, it := range items {
		qty := it.Qty
		if qty < 1 {
			qty = 1
		}
		price := it.PriceCents
		if price < 0 {
			price = 0
		}
		if qty == 1 {
			out = append(out, FlatItem{Name: it.Name, PriceCents: price})
			continue
		}
		for i := 1; i <= qty; i++ {
			name := it.Name
			if name != "" {
				name = fmt.Sprintf("%s (%d of %d)", name, i, qty)
			}
			out = append(out, FlatItem{Name: name, PriceCents: price})
		}
	}
	return out
}
