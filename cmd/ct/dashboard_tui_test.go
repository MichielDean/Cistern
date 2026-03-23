package main

import (
	"strings"
	"testing"
)

// TestTuiAqueductRow_WaterfallIndex_WidePoolRowsAtBottom verifies that when a
// droplet is on the final step the wide-pool waterfall rows (containing "≈")
// appear at the bottom of the arch, not near the top.
//
// wfRows is a 14-element array indexed 0..13. The arch loop runs r=5..13 (9
// iterations). Using wfRows[r] skips the first five entries and places the
// wide-pool rows (indices 7–8) at arch rows r=7 and r=8 — near the top.
// The correct index is wfRows[r-5], which maps r=12→wfRows[7] and
// r=13→wfRows[8], placing the pool at the very bottom of the waterfall.
//
// Result layout returned by tuiAqueductRow:
//
//	rows[0]    = nameLine
//	rows[1]    = infoLine
//	rows[2]    = lblLine
//	rows[3]    = l1 (channel top)
//	rows[4]    = l2 (channel water + wfExit)
//	rows[5..13] = arch rows for r=5..13
//
// Given: a CataractaeInfo with a droplet assigned to the last step
// When:  tuiAqueductRow is called at frame 0
// Then:  "≈" appears only in rows[12] and rows[13], never in rows[5..11]
func TestTuiAqueductRow_WaterfallIndex_WidePoolRowsAtBottom(t *testing.T) {
	steps := []string{"implement", "review", "merge"}
	ch := CataractaeInfo{
		Name:      "virgo",
		RepoName:  "myrepo",
		DropletID: "ci-test01",
		Step:      "merge", // last step → isLastStep = true
		Steps:     steps,
	}
	m := dashboardTUIModel{}
	rows := m.tuiAqueductRow(ch, 0)

	// Sanity: nameLine + infoLine + lblLine + l1 + l2 + 9 arch rows = 14.
	if len(rows) != 14 {
		t.Fatalf("tuiAqueductRow returned %d rows, want 14", len(rows))
	}

	// Upper arch rows must NOT contain the wide-pool "≈" glyph.
	for i := 5; i <= 11; i++ {
		if strings.Contains(rows[i], "≈") {
			t.Errorf("rows[%d] contains '≈' (wide-pool row should be at the bottom, not row %d); got: %q", i, i, rows[i])
		}
	}

	// The last two arch rows MUST contain "≈" (wfRows[7] and wfRows[8]).
	for i := 12; i <= 13; i++ {
		if !strings.Contains(rows[i], "≈") {
			t.Errorf("rows[%d] missing '≈' (wide-pool row should appear at bottom of waterfall); got: %q", i, rows[i])
		}
	}
}
