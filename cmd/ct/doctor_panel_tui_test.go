package main

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MichielDean/cistern/internal/cistern"
)

// ── initial state ─────────────────────────────────────────────────────────────

// TestDoctorPanel_NewPanel_TitleIsDoctor verifies the panel title.
//
// Given: a new doctorPanel
// When:  Title() is called
// Then:  "Doctor" is returned
func TestDoctorPanel_NewPanel_TitleIsDoctor(t *testing.T) {
	p := newDoctorPanel()
	if p.Title() != "Doctor" {
		t.Errorf("Title() = %q, want %q", p.Title(), "Doctor")
	}
}

// TestDoctorPanel_NewPanel_OverlayNotActive verifies no overlay is active by default.
//
// Given: a new doctorPanel
// When:  OverlayActive() is called
// Then:  false is returned
func TestDoctorPanel_NewPanel_OverlayNotActive(t *testing.T) {
	p := newDoctorPanel()
	if p.OverlayActive() {
		t.Error("OverlayActive() = true, want false")
	}
}

// TestDoctorPanel_NewPanel_PaletteActionsNil verifies no palette actions for doctor panel.
//
// Given: a new doctorPanel
// When:  PaletteActions() is called with a non-nil droplet
// Then:  nil is returned
func TestDoctorPanel_NewPanel_PaletteActionsNil(t *testing.T) {
	p := newDoctorPanel()
	d := &cistern.Droplet{ID: "ci-test01"}
	if actions := p.PaletteActions(d); actions != nil {
		t.Errorf("PaletteActions() = %v, want nil", actions)
	}
}

// TestDoctorPanel_NewPanel_KeyHelpNonEmpty verifies a non-empty key help string.
//
// Given: a new doctorPanel
// When:  KeyHelp() is called
// Then:  a non-empty string is returned
func TestDoctorPanel_NewPanel_KeyHelpNonEmpty(t *testing.T) {
	p := newDoctorPanel()
	if p.KeyHelp() == "" {
		t.Error("KeyHelp() = empty string, want non-empty")
	}
}

// TestDoctorPanel_NewPanel_RunningIsTrue verifies that a new panel starts in running state
// because Init() dispatches the first run immediately.
//
// Given: a new doctorPanel
// When:  running field is inspected
// Then:  running = true
func TestDoctorPanel_NewPanel_RunningIsTrue(t *testing.T) {
	p := newDoctorPanel()
	if !p.running {
		t.Error("running = false, want true (run dispatched on Init)")
	}
}

// ── View: running state ───────────────────────────────────────────────────────

// TestDoctorPanel_View_WhenRunning_ShowsRunningIndicator verifies the loading state
// when a doctor run is in progress.
//
// Given: a doctorPanel with running=true
// When:  View() is called
// Then:  output contains "Running"
func TestDoctorPanel_View_WhenRunning_ShowsRunningIndicator(t *testing.T) {
	p := newDoctorPanel()
	p.running = true
	v := p.View()
	if !strings.Contains(v, "Running") {
		t.Errorf("View() = %q, want it to contain %q", v, "Running")
	}
}

// ── View: with output ─────────────────────────────────────────────────────────

// TestDoctorPanel_View_WithOutput_ShowsOutput verifies that captured doctor output
// appears in the view.
//
// Given: a doctorPanel with output set to "✓ tmux installed" and running=false
// When:  View() is called
// Then:  output contains "✓ tmux installed"
func TestDoctorPanel_View_WithOutput_ShowsOutput(t *testing.T) {
	p := newDoctorPanel()
	p.running = false
	p.output = "✓ tmux installed\n✓ git installed\n"
	p.runAt = time.Now()
	v := p.View()
	if !strings.Contains(v, "tmux installed") {
		t.Errorf("View() does not contain %q; output:\n%s", "tmux installed", v)
	}
}

// TestDoctorPanel_View_WithRunAt_ShowsLastRunTimestamp verifies that the footer shows
// last-run timestamp and re-run hint.
//
// Given: a doctorPanel with runAt set and running=false
// When:  View() is called
// Then:  output contains "last run" and "r to re-run"
func TestDoctorPanel_View_WithRunAt_ShowsLastRunTimestamp(t *testing.T) {
	p := newDoctorPanel()
	p.running = false
	p.output = "✓ git installed\n"
	p.runAt = time.Now().Add(-10 * time.Second)
	v := p.View()
	for _, want := range []string{"last run", "r to re-run"} {
		if !strings.Contains(v, want) {
			t.Errorf("View() does not contain %q; output:\n%s", want, v)
		}
	}
}

// TestDoctorPanel_View_ScrollClamped_WhenScrollYExceedsContent verifies that View()
// clamps scrollY to the actual content length without panicking.
//
// Given: a doctorPanel with output and scrollY set far beyond content length
// When:  View() is called
// Then:  output is non-empty and no index-out-of-range panic occurs
func TestDoctorPanel_View_ScrollClamped_WhenScrollYExceedsContent(t *testing.T) {
	p := newDoctorPanel()
	p.running = false
	p.output = "✓ tmux installed\n✓ git installed\n"
	p.runAt = time.Now()
	p.height = 5
	p.scrollY = 999999

	v := p.View()
	if v == "" {
		t.Error("View() = empty string, want non-empty output after scroll clamping")
	}
}

// ── Update: doctorOutputMsg ───────────────────────────────────────────────────

// TestDoctorPanel_Update_OutputMsg_StoresOutput verifies that receiving a doctorOutputMsg
// stores the output text.
//
// Given: a doctorPanel with no output
// When:  a doctorOutputMsg with output "✓ all good" is processed
// Then:  the model's output field contains "✓ all good"
func TestDoctorPanel_Update_OutputMsg_StoresOutput(t *testing.T) {
	p := newDoctorPanel()

	updated, _ := p.Update(doctorOutputMsg{output: "✓ all good", runAt: time.Now()})
	up := updated.(doctorPanel)

	if !strings.Contains(up.output, "✓ all good") {
		t.Errorf("output = %q, want it to contain %q", up.output, "✓ all good")
	}
}

// TestDoctorPanel_Update_OutputMsg_ClearsRunning verifies that receiving a doctorOutputMsg
// sets running=false.
//
// Given: a doctorPanel with running=true
// When:  a doctorOutputMsg is processed
// Then:  running = false
func TestDoctorPanel_Update_OutputMsg_ClearsRunning(t *testing.T) {
	p := newDoctorPanel()
	p.running = true

	updated, _ := p.Update(doctorOutputMsg{output: "ok", runAt: time.Now()})
	up := updated.(doctorPanel)

	if up.running {
		t.Error("running = true after doctorOutputMsg, want false")
	}
}

// TestDoctorPanel_Update_OutputMsg_ResetsScroll verifies that new output resets scroll
// to the top.
//
// Given: a doctorPanel with scrollY=20
// When:  a doctorOutputMsg is processed
// Then:  scrollY = 0
func TestDoctorPanel_Update_OutputMsg_ResetsScroll(t *testing.T) {
	p := newDoctorPanel()
	p.scrollY = 20

	updated, _ := p.Update(doctorOutputMsg{output: "ok", runAt: time.Now()})
	up := updated.(doctorPanel)

	if up.scrollY != 0 {
		t.Errorf("scrollY = %d, want 0 after new output", up.scrollY)
	}
}

// TestDoctorPanel_Update_OutputMsg_StoresRunAt verifies that runAt is set from the message.
//
// Given: a doctorPanel
// When:  a doctorOutputMsg with a specific runAt is processed
// Then:  the model's runAt matches the message
func TestDoctorPanel_Update_OutputMsg_StoresRunAt(t *testing.T) {
	p := newDoctorPanel()
	ts := time.Now()

	updated, _ := p.Update(doctorOutputMsg{output: "ok", runAt: ts})
	up := updated.(doctorPanel)

	if !up.runAt.Equal(ts) {
		t.Errorf("runAt = %v, want %v", up.runAt, ts)
	}
}

// ── Update: r key re-run ──────────────────────────────────────────────────────

// TestDoctorPanel_Update_RKey_SetsRunning verifies that pressing 'r' sets running=true.
//
// Given: a doctorPanel with running=false
// When:  'r' is pressed
// Then:  running = true
func TestDoctorPanel_Update_RKey_SetsRunning(t *testing.T) {
	p := newDoctorPanel()
	p.running = false
	p.output = "old output"

	updated, _ := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	up := updated.(doctorPanel)

	if !up.running {
		t.Error("running = false after 'r', want true")
	}
}

// TestDoctorPanel_Update_RKey_ClearsOutput verifies that pressing 'r' clears old output.
//
// Given: a doctorPanel with some output
// When:  'r' is pressed
// Then:  output = ""
func TestDoctorPanel_Update_RKey_ClearsOutput(t *testing.T) {
	p := newDoctorPanel()
	p.running = false
	p.output = "old output"

	updated, _ := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	up := updated.(doctorPanel)

	if up.output != "" {
		t.Errorf("output = %q, want empty after 'r'", up.output)
	}
}

// TestDoctorPanel_Update_RKey_WhileRunning_ReturnsNoCmd verifies that pressing 'r' while
// a run is already in progress does NOT spawn a second concurrent subprocess.
//
// Given: a doctorPanel with running=true
// When:  'r' is pressed
// Then:  cmd = nil (guard prevents a second concurrent run)
func TestDoctorPanel_Update_RKey_WhileRunning_ReturnsNoCmd(t *testing.T) {
	p := newDoctorPanel()
	p.running = true

	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd != nil {
		t.Error("cmd non-nil after 'r' pressed while already running, want nil (guard active)")
	}
}

// TestDoctorPanel_Update_UpperRKey_WhileRunning_ReturnsNoCmd verifies that pressing 'R'
// while a run is already in progress does NOT spawn a second concurrent subprocess.
//
// Given: a doctorPanel with running=true
// When:  'R' is pressed
// Then:  cmd = nil (guard prevents a second concurrent run)
func TestDoctorPanel_Update_UpperRKey_WhileRunning_ReturnsNoCmd(t *testing.T) {
	p := newDoctorPanel()
	p.running = true

	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	if cmd != nil {
		t.Error("cmd non-nil after 'R' pressed while already running, want nil (guard active)")
	}
}

// TestDoctorPanel_Update_RKey_ReturnsCmd verifies that pressing 'r' triggers
// a re-run command when the panel is idle.
//
// Given: a doctorPanel with running=false
// When:  'r' is pressed
// Then:  a non-nil command is returned
func TestDoctorPanel_Update_RKey_ReturnsCmd(t *testing.T) {
	p := newDoctorPanel()
	p.running = false

	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd == nil {
		t.Error("cmd = nil after 'r' key press, want a run command")
	}
}

// TestDoctorPanel_Update_UpperRKey_ReturnsCmd verifies that 'R' also triggers a re-run
// when the panel is idle.
//
// Given: a doctorPanel with running=false
// When:  'R' is pressed
// Then:  a non-nil command is returned
func TestDoctorPanel_Update_UpperRKey_ReturnsCmd(t *testing.T) {
	p := newDoctorPanel()
	p.running = false

	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	if cmd == nil {
		t.Error("cmd = nil after 'R' key press, want a run command")
	}
}

// ── Update: scroll ────────────────────────────────────────────────────────────

// TestDoctorPanel_Update_DownKey_IncrementsScrollY verifies 'j' scrolls down.
//
// Given: scrollY=0
// When:  'j' is pressed
// Then:  scrollY = 1
func TestDoctorPanel_Update_DownKey_IncrementsScrollY(t *testing.T) {
	p := newDoctorPanel()
	p.scrollY = 0

	updated, _ := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	up := updated.(doctorPanel)

	if up.scrollY != 1 {
		t.Errorf("scrollY = %d, want 1", up.scrollY)
	}
}

// TestDoctorPanel_Update_UpKey_DecrementsScrollY verifies 'k' scrolls up.
//
// Given: scrollY=3
// When:  'k' is pressed
// Then:  scrollY = 2
func TestDoctorPanel_Update_UpKey_DecrementsScrollY(t *testing.T) {
	p := newDoctorPanel()
	p.scrollY = 3

	updated, _ := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	up := updated.(doctorPanel)

	if up.scrollY != 2 {
		t.Errorf("scrollY = %d, want 2", up.scrollY)
	}
}

// TestDoctorPanel_Update_UpKey_AtTop_StaysAtZero verifies 'k' at the top does not
// set a negative scrollY.
//
// Given: scrollY=0
// When:  'k' is pressed
// Then:  scrollY = 0 (no underflow)
func TestDoctorPanel_Update_UpKey_AtTop_StaysAtZero(t *testing.T) {
	p := newDoctorPanel()
	p.scrollY = 0

	updated, _ := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	up := updated.(doctorPanel)

	if up.scrollY != 0 {
		t.Errorf("scrollY = %d, want 0 (should not underflow)", up.scrollY)
	}
}

// TestDoctorPanel_Update_HomeKey_ResetsScroll verifies 'g' jumps to the top.
//
// Given: scrollY=10
// When:  'g' is pressed
// Then:  scrollY = 0
func TestDoctorPanel_Update_HomeKey_ResetsScroll(t *testing.T) {
	p := newDoctorPanel()
	p.scrollY = 10

	updated, _ := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	up := updated.(doctorPanel)

	if up.scrollY != 0 {
		t.Errorf("scrollY = %d, want 0 after 'g'", up.scrollY)
	}
}

// TestDoctorPanel_Update_EndKey_SetsScrollYToBottom verifies 'G' jumps to the bottom
// by setting scrollY to a large sentinel value.
//
// Given: scrollY=0
// When:  'G' is pressed
// Then:  scrollY > 0 (set to a large sentinel so View() clamps to last line)
func TestDoctorPanel_Update_EndKey_SetsScrollYToBottom(t *testing.T) {
	p := newDoctorPanel()
	p.scrollY = 0

	updated, _ := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	up := updated.(doctorPanel)

	if up.scrollY <= 0 {
		t.Errorf("scrollY = %d, want large value after 'G'", up.scrollY)
	}
}

// ── Update: window resize ─────────────────────────────────────────────────────

// TestDoctorPanel_Update_WindowSizeMsg_UpdatesDimensions verifies that
// tea.WindowSizeMsg updates the panel's width and height.
//
// Given: a doctorPanel with default dimensions
// When:  a WindowSizeMsg{Width: 120, Height: 40} is processed
// Then:  width=120, height=40
func TestDoctorPanel_Update_WindowSizeMsg_UpdatesDimensions(t *testing.T) {
	p := newDoctorPanel()

	updated, _ := p.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	up := updated.(doctorPanel)

	if up.width != 120 {
		t.Errorf("width = %d, want 120", up.width)
	}
	if up.height != 40 {
		t.Errorf("height = %d, want 40", up.height)
	}
}

// ── Init ─────────────────────────────────────────────────────────────────────

// TestDoctorPanel_Init_ReturnsCmd verifies that Init returns a non-nil command
// that will dispatch the first ct doctor run.
//
// Given: a new doctorPanel
// When:  Init() is called
// Then:  a non-nil command is returned
func TestDoctorPanel_Init_ReturnsCmd(t *testing.T) {
	p := newDoctorPanel()
	cmd := p.Init()
	if cmd == nil {
		t.Error("Init() = nil, want a non-nil run command")
	}
}

// ── cockpit integration ───────────────────────────────────────────────────────

// TestCockpit_Panel5_IsDoctorPanel verifies the cockpit panel at index 4 (key: 5)
// is a doctorPanel with title "Doctor".
//
// Given: a new cockpitModel
// When:  panels[4] title is inspected
// Then:  title = "Doctor"
func TestCockpit_Panel5_IsDoctorPanel(t *testing.T) {
	m := newCockpitModel("", "")
	if len(m.panels) < 5 {
		t.Fatalf("len(panels) = %d, want at least 5", len(m.panels))
	}
	if m.panels[4].Title() != "Doctor" {
		t.Errorf("panels[4].Title() = %q, want %q", m.panels[4].Title(), "Doctor")
	}
}

// TestCockpit_Key5_ActivatesDoctorPanel verifies that pressing '5' in sidebar mode
// jumps to the doctor panel (index 4) and activates panel focus.
//
// Given: a cockpitModel in sidebar mode, cursor=0
// When:  '5' is pressed
// Then:  cursor=4, panelFocused=true
func TestCockpit_Key5_ActivatesDoctorPanel(t *testing.T) {
	m := newCockpitModel("", "")
	m.cursor = 0

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'5'}})
	um := updated.(cockpitModel)

	if um.cursor != 4 {
		t.Errorf("cursor = %d, want 4", um.cursor)
	}
	if !um.panelFocused {
		t.Error("panelFocused = false, want true after pressing '5'")
	}
}

// TestCockpit_DoctorOutputMsg_RoutesToDoctorPanel_WhenCursorNotAtFour verifies that
// doctorOutputMsg is always delivered to panels[4] regardless of which panel is active.
//
// Given: a cockpitModel with cursor=0 (Droplets panel active)
// When:  a doctorOutputMsg with output "✓ all checks passed" is processed
// Then:  panels[4] (doctorPanel) has output containing "✓ all checks passed"
func TestCockpit_DoctorOutputMsg_RoutesToDoctorPanel_WhenCursorNotAtFour(t *testing.T) {
	m := newCockpitModel("", "")
	m.cursor = 0

	updated, _ := m.Update(doctorOutputMsg{output: "✓ all checks passed", runAt: time.Now()})
	um := updated.(cockpitModel)

	dp, ok := um.panels[4].(doctorPanel)
	if !ok {
		t.Fatalf("panels[4] is not a doctorPanel")
	}
	if !strings.Contains(dp.output, "✓ all checks passed") {
		t.Errorf("doctorPanel.output = %q, want it to contain %q", dp.output, "✓ all checks passed")
	}
}
