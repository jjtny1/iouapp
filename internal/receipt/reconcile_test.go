package receipt

import "testing"

// TestReconcile covers the backend reconciliation guard: parsed parts that do
// not sum to the receipt's printed total are corrected (the common VAT case)
// or left alone, and a receipt with no printed total is untouched.
func TestReconcile(t *testing.T) {
	// Mirrors IMG_8588: three items totalling 122.00, a 12.5% service charge,
	// and two VAT lines (8.84 + 6.67 = 15.51) that are already included in the
	// prices — the receipt was actually paid as 137.25.
	vatReceipt := func() ParsedReceipt {
		return ParsedReceipt{
			Items: []ParsedItem{
				{Name: "Toastie", PriceCents: 5800, Qty: 1},
				{Name: "Coke", PriceCents: 1600, Qty: 2},
				{Name: "Biscotto", PriceCents: 3200, Qty: 1},
			},
			TaxCents:        1551,
			ServiceCharge:   ParsedServiceCharge{Kind: "percent", Percent: 12.5},
			GrandTotalCents: 13725,
		}
	}

	t.Run("drops included VAT misreported as tax", func(t *testing.T) {
		got := reconcile(vatReceipt())
		if got.TaxCents != 0 {
			t.Errorf("tax_cents = %d, want 0 (VAT was included in prices)", got.TaxCents)
		}
		// 12200 items + 0 tax + 0 tip + 1525 service == 13725 printed total.
		if total := itemsTotal(got.Items) + got.TaxCents +
			parsedServiceChargeCents(got.ServiceCharge, itemsTotal(got.Items)); total != got.GrandTotalCents {
			t.Errorf("reconciled total = %d, want %d", total, got.GrandTotalCents)
		}
	})

	t.Run("leaves a genuine added tax alone", func(t *testing.T) {
		// US-style: 10.00 item + 0.80 tax added on top == 10.80 paid.
		r := ParsedReceipt{
			Items:           []ParsedItem{{Name: "Burger", PriceCents: 1000, Qty: 1}},
			TaxCents:        80,
			ServiceCharge:   ParsedServiceCharge{Kind: "none"},
			GrandTotalCents: 1080,
		}
		if got := reconcile(r); got.TaxCents != 80 {
			t.Errorf("tax_cents = %d, want 80 (genuine added tax)", got.TaxCents)
		}
	})

	t.Run("no printed total leaves the receipt unchanged", func(t *testing.T) {
		r := vatReceipt()
		r.GrandTotalCents = 0
		if got := reconcile(r); got.TaxCents != 1551 {
			t.Errorf("tax_cents = %d, want 1551 (nothing to reconcile against)", got.TaxCents)
		}
	})

	t.Run("unexplained mismatch keeps tax and warns", func(t *testing.T) {
		// Zeroing tax would not reconcile this, so reconcile must not guess.
		r := ParsedReceipt{
			Items:           []ParsedItem{{Name: "Burger", PriceCents: 1000, Qty: 1}},
			TaxCents:        50,
			ServiceCharge:   ParsedServiceCharge{Kind: "none"},
			GrandTotalCents: 2000,
		}
		if got := reconcile(r); got.TaxCents != 50 {
			t.Errorf("tax_cents = %d, want 50 (left alone — cannot be reconciled)", got.TaxCents)
		}
	})
}

// TestParseReceiptJSONReconciles checks reconciliation runs end-to-end through
// parseReceiptJSON: a receipt whose grand total excludes its (included) VAT
// comes back with tax_cents zeroed.
func TestParseReceiptJSONReconciles(t *testing.T) {
	input := `{"restaurant":"Gaia","currency":"PLN",` +
		`"items":[{"name":"Toastie","price_cents":5800,"qty":1},` +
		`{"name":"Coke","price_cents":1600,"qty":2},` +
		`{"name":"Biscotto","price_cents":3200,"qty":1}],` +
		`"tax_cents":1551,"tip_cents":0,` +
		`"service_charge":{"kind":"percent","percent":12.5},` +
		`"grand_total_cents":13725}`
	got, err := parseReceiptJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.TaxCents != 0 {
		t.Errorf("tax_cents = %d, want 0 after reconciliation", got.TaxCents)
	}
	if got.GrandTotalCents != 13725 {
		t.Errorf("grand_total_cents = %d, want 13725", got.GrandTotalCents)
	}
}
