package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestDashboard_PeekKey_InTmux_SpawnsNewWindow verifies that when 'p' is pressed
// inside a tmux session with one active aqueduct, a new tmux window is spawned
// targeting the correct session rather than opening the inline peek overlay.
//
// Given: a dashboard model with one active aqueduct (Name="virgo", RepoName="myrepo",
//
//	DropletID="ci-test01") and insideTmux() returning true
//
// When:  the 'p' key is pressed and the returned tea.Cmd is executed
// Then:  tmuxNewWindowFunc is called with dropletID="ci-test01" and session="myrepo-virgo",
//
//	and peekActive remains false (dashboard is not interrupted)
func TestDashboard_PeekKey_InTmux_SpawnsNewWindow(t *testing.T) {
	// Inject insideTmux to simulate running inside a tmux session.
	origInsideTmux := insideTmux
	insideTmux = func() bool { return true }
	defer func() { insideTmux = origInsideTmux }()

	// Capture the new-window call.
	var gotDropletID, gotSession string
	origNewWindow := tmuxNewWindowFunc
	tmuxNewWindowFunc = func(dropletID, session string) error {
		gotDropletID = dropletID
		gotSession = session
		return nil
	}
	defer func() { tmuxNewWindowFunc = origNewWindow }()

	// Build a dashboard model with one active aqueduct.
	m := newDashboardTUIModel("", "")
	m.data = &DashboardData{
		Cataractae: []CataractaeInfo{
			{
				Name:      "virgo",
				RepoName:  "myrepo",
				DropletID: "ci-test01",
				Step:      "implement",
				Steps:     []string{"implement", "review"},
			},
		},
	}

	// Press 'p' to trigger peek.
	updatedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})

	// Execute the returned cmd to trigger tmuxNewWindowFunc.
	if cmd != nil {
		cmd()
	}

	// The dashboard should NOT have entered inline peek mode.
	um := updatedModel.(dashboardTUIModel)
	if um.peekActive {
		t.Error("peekActive should be false when spawning a new tmux window")
	}

	// Verify the new-window was called with the correct identifiers.
	if gotDropletID != "ci-test01" {
		t.Errorf("tmuxNewWindowFunc dropletID = %q, want %q", gotDropletID, "ci-test01")
	}
	wantSession := "myrepo-virgo"
	if gotSession != wantSession {
		t.Errorf("tmuxNewWindowFunc session = %q, want %q", gotSession, wantSession)
	}
}

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
