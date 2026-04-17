package benchmarks

import (
	"encoding/json"
	"testing"
)

func TestDefaultItems(t *testing.T) {
	items := DefaultItems()
	if len(items) == 0 {
		t.Error("expected at least one benchmark item")
	}

	seen := make(map[string]bool)
	for _, item := range items {
		if seen[item.ID] {
			t.Errorf("duplicate item ID: %s", item.ID)
		}
		seen[item.ID] = true

		if item.Title == "" {
			t.Errorf("item %s has no title", item.ID)
		}
		if item.Description == "" {
			t.Errorf("item %s has no description", item.ID)
		}
		if len(item.Exercises) == 0 {
			t.Errorf("item %s exercises no dimensions", item.ID)
		}
		if item.Complexity != "standard" && item.Complexity != "full" && item.Complexity != "critical" {
			t.Errorf("item %s has invalid complexity: %s", item.ID, item.Complexity)
		}
	}
}

func TestDefaultItemsJSON(t *testing.T) {
	items := DefaultItems()
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var items2 []Item
	if err := json.Unmarshal(data, &items2); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(items2) != len(items) {
		t.Errorf("expected %d items after round-trip, got %d", len(items), len(items2))
	}
}

func TestDefaultItemsCoverAllDimensions(t *testing.T) {
	items := DefaultItems()
	dimsCovered := make(map[string]bool)

	for _, item := range items {
		for _, dim := range item.Exercises {
			dimsCovered[dim] = true
		}
	}

	// Verify all 8 rubric dimensions are exercised by at least one item
	expectedDims := []string{
		"contract_correctness",
		"integration_coverage",
		"coupling",
		"migration_safety",
		"idiom_fit",
		"dry",
		"naming_clarity",
		"error_messages",
	}

	for _, dim := range expectedDims {
		if !dimsCovered[dim] {
			t.Errorf("dimension %s is not exercised by any benchmark item", dim)
		}
	}
}