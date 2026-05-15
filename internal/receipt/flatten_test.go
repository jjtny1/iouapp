package receipt

import (
	"reflect"
	"testing"
)

func TestFlatten(t *testing.T) {
	tests := []struct {
		name  string
		input []ParsedItem
		want  []FlatItem
	}{
		{
			name:  "quantity 1 keeps its name",
			input: []ParsedItem{{Name: "Caesar Salad", PriceCents: 1050, Qty: 1}},
			want:  []FlatItem{{Name: "Caesar Salad", PriceCents: 1050}},
		},
		{
			name:  "quantity 2 expands into numbered units",
			input: []ParsedItem{{Name: "Coca-Cola", PriceCents: 300, Qty: 2}},
			want: []FlatItem{
				{Name: "Coca-Cola (1 of 2)", PriceCents: 300},
				{Name: "Coca-Cola (2 of 2)", PriceCents: 300},
			},
		},
		{
			name:  "zero quantity is treated as one",
			input: []ParsedItem{{Name: "Water", PriceCents: 0, Qty: 0}},
			want:  []FlatItem{{Name: "Water", PriceCents: 0}},
		},
		{
			name:  "negative price is clamped to zero",
			input: []ParsedItem{{Name: "Comp", PriceCents: -500, Qty: 1}},
			want:  []FlatItem{{Name: "Comp", PriceCents: 0}},
		},
		{
			name:  "empty name stays empty when expanded",
			input: []ParsedItem{{Name: "", PriceCents: 200, Qty: 2}},
			want: []FlatItem{
				{Name: "", PriceCents: 200},
				{Name: "", PriceCents: 200},
			},
		},
		{
			name: "multiple lines keep order and per-unit price",
			input: []ParsedItem{
				{Name: "Burger", PriceCents: 1395, Qty: 2},
				{Name: "Fries", PriceCents: 595, Qty: 1},
			},
			want: []FlatItem{
				{Name: "Burger (1 of 2)", PriceCents: 1395},
				{Name: "Burger (2 of 2)", PriceCents: 1395},
				{Name: "Fries", PriceCents: 595},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Flatten(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Flatten() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
