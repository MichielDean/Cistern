package main

import (
	"strings"
	"testing"
)

// stripANSITUI strips ANSI escape codes for visual content checks.
func stripANSITUI(s string) string {
	var out strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\033' {
			inEsc = true
			continue
		}
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}

// TestTUIAqueductRow_ChannelWidth_ColW16 verifies that the channel mortar cap row
// contains n*16 ▀ characters (colW=16, not the broken colW=14).
func TestTUIAqueductRow_ChannelWidth_ColW16(t *testing.T) {
	m := dashboardTUIModel{}
	ch := CataractaeInfo{
		Name:  "virgo",
		Steps: []string{"implement"},
	}
	lines := m.tuiAqueductRow(ch, 0)
	if len(lines) == 0 {
		t.Fatal("tuiAqueductRow returned no lines")
	}
	// lines[0] is l1: the channel mortar cap — prefix + ▀*chanW where chanW = n*colW.
	// For n=1 step, colW=16 → 16 ▀ chars; colW=14 (broken) → 14 ▀ chars.
	clean := stripANSITUI(lines[0])
	got := strings.Count(clean, "▀")
	if got != 16 {
		t.Errorf("channel cap ▀ count = %d, want 16 (colW=16); broken colW=14 would give 14", got)
	}
}

// TestTUIAqueductRow_HalfBlockEdge checks that ▄ (lower half-block) appears in the
// rendered arch output. This character marks the intrados edge in mortar sub-rows
// where the parabolic curve frac > 0.5, giving sub-pixel edge resolution.
// The broken colW=14/archTopW=9/taperRows=4 constants never produce frac>0.5 in
// mortar sub-rows, so this test fails until the constants and rendering are both fixed.
func TestTUIAqueductRow_HalfBlockEdge(t *testing.T) {
	m := dashboardTUIModel{}
	ch := CataractaeInfo{
		Name:  "virgo",
		Steps: []string{"implement", "review"},
	}
	lines := m.tuiAqueductRow(ch, 0)
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "▄") {
		t.Error("expected ▄ (lower half-block) in arch output for intrados edge softening; " +
			"check that colW=16/archTopW=8/taperRows=3 constants are set and half-block rendering is active")
	}
}

// TestTUIAqueductRow_PierW_AtLeastTwo verifies the pier body is at least 2 chars wide.
// With broken constants (colW=14, archTopW=9, taperRows=4): pierW = 9-8 = 1 (toothpick).
// With correct constants (colW=16, archTopW=8, taperRows=3): pierW = 8-6 = 2 (solid).
// The pier rows (lr >= taperRows) render bodyW=pierW; we check that two consecutive
// solid █ chars appear in those rows (strip ANSI first to count accurately).
func TestTUIAqueductRow_PierW_AtLeastTwo(t *testing.T) {
	m := dashboardTUIModel{}
	ch := CataractaeInfo{
		Name:  "virgo",
		Steps: []string{"implement"},
	}
	lines := m.tuiAqueductRow(ch, 0)
	// Arch lines start at index 2 (after l1, l2). The pier sub-rows are the last
	// 2*(pierRows) lines before the label line. Check for "██" in any arch line.
	found := false
	for _, line := range lines[2 : len(lines)-1] {
		if strings.Contains(stripANSITUI(line), "██") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected pier body ██ (at least 2 chars wide) in arch lines; broken pierW=1 has only one █")
	}
}
