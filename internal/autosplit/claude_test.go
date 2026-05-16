package autosplit

import (
	"context"
	"testing"

	"github.com/jjtny1/iouapp/internal/config"
)

const bareJSON = `{"people":["Sam","Riley"],` +
	`"items":[{"index":1,"people":["Sam"]},{"index":2,"people":["Riley","Sam"]}],` +
	`"notes":"shared the appetizer"}`

func TestParseAssignmentJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "bare JSON object", input: bareJSON},
		{name: "fenced json code block", input: "```json\n" + bareJSON + "\n```"},
		{name: "fenced code block without language", input: "```\n" + bareJSON + "\n```"},
		{name: "surrounding prose", input: "Here is the split:\n" + bareJSON + "\nHope that helps."},
		{name: "no JSON at all", input: "I could not work out the split.", wantErr: true},
		{name: "malformed JSON", input: `{"people":["Sam",`, wantErr: true},
		{name: "empty string", input: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAssignmentJSON(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got assignment %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got.People) != 2 {
				t.Errorf("people = %v, want 2", got.People)
			}
			if len(got.Items) != 2 {
				t.Errorf("items = %v, want 2", got.Items)
			}
			if got.Notes != "shared the appetizer" {
				t.Errorf("notes = %q", got.Notes)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	items := []Item{
		{Index: 1, Name: "Soup", PriceCents: 800},
		{Index: 2, Name: "Steak", PriceCents: 2995},
	}

	t.Run("drops out-of-range indices", func(t *testing.T) {
		in := Assignment{
			People: []string{"Sam"},
			Items: []ItemAssignment{
				{Index: 0, People: []string{"Sam"}},
				{Index: 1, People: []string{"Sam"}},
				{Index: 3, People: []string{"Sam"}},
				{Index: -1, People: []string{"Sam"}},
			},
		}
		got := Validate(in, items)
		if len(got.Items) != 1 || got.Items[0].Index != 1 {
			t.Errorf("items = %+v, want only index 1", got.Items)
		}
	})

	t.Run("dedups names case-insensitively keeping first casing", func(t *testing.T) {
		in := Assignment{
			People: []string{"Sam", "sam", "Riley"},
			Items: []ItemAssignment{
				{Index: 1, People: []string{"SAM", "Sam"}},
			},
		}
		got := Validate(in, items)
		if len(got.People) != 2 {
			t.Errorf("people = %v, want 2 distinct", got.People)
		}
		if got.People[0] != "Sam" || got.People[1] != "Riley" {
			t.Errorf("people = %v, want [Sam Riley]", got.People)
		}
		if len(got.Items[0].People) != 1 || got.Items[0].People[0] != "Sam" {
			t.Errorf("item people = %v, want [Sam]", got.Items[0].People)
		}
	})

	t.Run("adds item people missing from top-level people", func(t *testing.T) {
		in := Assignment{
			People: []string{"Sam"},
			Items: []ItemAssignment{
				{Index: 2, People: []string{"Jordan"}},
			},
		}
		got := Validate(in, items)
		found := false
		for _, p := range got.People {
			if p == "Jordan" {
				found = true
			}
		}
		if !found {
			t.Errorf("people = %v, want Jordan included", got.People)
		}
	})

	t.Run("preserves notes", func(t *testing.T) {
		got := Validate(Assignment{Notes: "hello"}, items)
		if got.Notes != "hello" {
			t.Errorf("notes = %q, want hello", got.Notes)
		}
	})
}

func TestNewSelectsStubWithoutKey(t *testing.T) {
	if _, ok := New(config.Config{}).(StubAssigner); !ok {
		t.Error("New with no Anthropic key should return a StubAssigner")
	}
	if _, ok := New(config.Config{AnthropicKey: "sk-test"}).(ClaudeAssigner); !ok {
		t.Error("New with an Anthropic key should return a ClaudeAssigner")
	}
}

func TestStubAssigner(t *testing.T) {
	got, err := StubAssigner{}.Assign(context.Background(), nil, "", "Sam")
	if err != nil {
		t.Fatalf("StubAssigner.Assign error: %v", err)
	}
	if len(got.People) == 0 {
		t.Error("expected people")
	}
	if len(got.Items) == 0 {
		t.Error("expected item assignments")
	}
}
