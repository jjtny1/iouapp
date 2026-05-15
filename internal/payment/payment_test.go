package payment

import (
	"net/url"
	"strings"
	"testing"
)

func TestNormalizeHandle(t *testing.T) {
	cases := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"jjtny1", "jjtny1", true},
		{"@jjtny1", "jjtny1", true},
		{"  @Maya-Lopez  ", "Maya-Lopez", true},
		{"with_underscore", "with_underscore", true},
		{"", "", false},
		{"@", "", false},
		{"shrt", "", false},                  // under 5 chars
		{"has spaces", "", false},            // space is invalid
		{"emoji😀here", "", false},            // non-ASCII rejected
		{strings.Repeat("x", 31), "", false}, // over 30 chars
	}
	for _, c := range cases {
		got, ok := NormalizeHandle(c.in)
		if got != c.want || ok != c.wantOK {
			t.Errorf("NormalizeHandle(%q) = (%q, %v), want (%q, %v)",
				c.in, got, ok, c.want, c.wantOK)
		}
	}
}

func TestAppURL(t *testing.T) {
	got := AppURL("host-venmo", 1234, "Dinner · split with IOU")
	if !strings.HasPrefix(got, "venmo://paycharge?") {
		t.Fatalf("AppURL = %q, want a venmo://paycharge link", got)
	}
	q := mustQuery(t, got)
	if q.Get("txn") != "pay" {
		t.Errorf("txn = %q, want pay", q.Get("txn"))
	}
	if q.Get("recipients") != "host-venmo" {
		t.Errorf("recipients = %q, want host-venmo", q.Get("recipients"))
	}
	if q.Get("amount") != "12.34" {
		t.Errorf("amount = %q, want 12.34", q.Get("amount"))
	}
	if q.Get("note") != "Dinner · split with IOU" {
		t.Errorf("note = %q", q.Get("note"))
	}
}

func TestWebURL(t *testing.T) {
	got := WebURL("host-venmo", 500, "note")
	if !strings.HasPrefix(got, "https://account.venmo.com/pay?") {
		t.Fatalf("WebURL = %q, want an account.venmo.com link", got)
	}
	if q := mustQuery(t, got); q.Get("amount") != "5.00" {
		t.Errorf("amount = %q, want 5.00", q.Get("amount"))
	}
}

// mustQuery parses the query string of a URL or fails the test.
func mustQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	_, qs, _ := strings.Cut(raw, "?")
	v, err := url.ParseQuery(qs)
	if err != nil {
		t.Fatalf("parse query of %q: %v", raw, err)
	}
	return v
}
