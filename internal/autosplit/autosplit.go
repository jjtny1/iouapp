// Package autosplit maps a host's spoken transcript onto a bill's line items,
// deciding who claimed what so the existing split math can run unchanged.
package autosplit

import (
	"context"
	"log"
	"strings"

	"github.com/jjtny1/iouapp/internal/config"
)

// Item is a single claimable line item presented to the assigner. Index is
// 1-based: it is the item's position in the list plus one.
type Item struct {
	Index      int    `json:"index"`
	Name       string `json:"name"`
	PriceCents int    `json:"price_cents"`
}

// ItemAssignment links one item (by 1-based Index) to the people who shared it.
type ItemAssignment struct {
	Index  int      `json:"index"`
	People []string `json:"people"`
}

// Assignment is the structured result of mapping a transcript onto the items:
// the full set of people named, which people share each item, and any free-text
// notes the model wants to surface to the host.
type Assignment struct {
	People []string         `json:"people"`
	Items  []ItemAssignment `json:"items"`
	Notes  string           `json:"notes"`
}

// Assigner maps a transcript and a bill's items onto an Assignment.
type Assigner interface {
	Assign(ctx context.Context, items []Item, transcript, hostName string) (Assignment, error)
}

// New selects an Assigner based on configuration: ClaudeAssigner when an
// Anthropic API key is present, otherwise the StubAssigner fallback.
func New(cfg config.Config) Assigner {
	if cfg.AnthropicKey != "" {
		log.Printf("autosplit: using ClaudeAssigner (Anthropic API)")
		return NewClaudeAssigner(cfg.AnthropicKey)
	}
	log.Printf("autosplit: no ANTHROPIC_API_KEY set, using StubAssigner")
	return StubAssigner{}
}

// Validate cleans a raw Assignment against the real item list: it drops item
// indices outside [1..len(items)], deduplicates people case-insensitively
// (keeping the first-seen casing), and guarantees every name appearing in an
// item assignment is also present in the top-level people list.
func Validate(a Assignment, items []Item) Assignment {
	out := Assignment{Notes: a.Notes}

	// canonical maps a lowercased name to its first-seen display casing.
	canonical := map[string]string{}
	order := []string{}
	add := func(name string) string {
		name = strings.TrimSpace(name)
		if name == "" {
			return ""
		}
		key := strings.ToLower(name)
		if c, ok := canonical[key]; ok {
			return c
		}
		canonical[key] = name
		order = append(order, key)
		return name
	}

	for _, p := range a.People {
		add(p)
	}

	for _, ia := range a.Items {
		if ia.Index < 1 || ia.Index > len(items) {
			continue
		}
		seen := map[string]bool{}
		people := []string{}
		for _, p := range ia.People {
			c := add(p)
			if c == "" {
				continue
			}
			key := strings.ToLower(c)
			if seen[key] {
				continue
			}
			seen[key] = true
			people = append(people, c)
		}
		out.Items = append(out.Items, ItemAssignment{Index: ia.Index, People: people})
	}

	for _, key := range order {
		out.People = append(out.People, canonical[key])
	}
	return out
}
