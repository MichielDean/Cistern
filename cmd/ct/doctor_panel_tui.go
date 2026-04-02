package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MichielDean/cistern/internal/cistern"
)

// doctorOutputMsg carries the result of a ct doctor run.
type doctorOutputMsg struct {
	output string
	runAt  time.Time
}

// Compile-time interface check.
var _ TUIPanel = doctorPanel{}

// doctorPanel is the Doctor cockpit module (key: 5).
// It runs ct doctor on panel activation and streams the output into a scrollable
// pane. Shows last-run timestamp. r re-runs. No continuous polling.
type doctorPanel struct {
	output  string
	runAt   time.Time
	running bool
	width   int
	height  int
	scrollY int
}

func newDoctorPanel() doctorPanel {
	return doctorPanel{
		width:   100,
		height:  24,
		running: true, // a run is dispatched immediately via Init
	}
}

func (p doctorPanel) Init() tea.Cmd {
	return p.execDoctorCmd()
}

// execDoctorCmd returns a tea.Cmd that runs ct doctor and returns doctorOutputMsg.
// It uses os.Executable so the correct binary is invoked even after installation.
func (p doctorPanel) execDoctorCmd() tea.Cmd {
	return func() tea.Msg {
		exe, err := os.Executable()
		if err != nil {
			exe = "ct"
		}
		cmd := exec.Command(exe, "doctor")
		out, _ := cmd.CombinedOutput() // non-zero exit is expected when checks fail
		return doctorOutputMsg{output: string(out), runAt: time.Now()}
	}
}

func (p doctorPanel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height

	case doctorOutputMsg:
		p.output = msg.output
		p.runAt = msg.runAt
		p.running = false
		p.scrollY = 0

	case tea.KeyMsg:
		switch msg.String() {
		case "r", "R":
			p.running = true
			p.output = ""
			p.scrollY = 0
			return p, p.execDoctorCmd()
		case "up", "k":
			if p.scrollY > 0 {
				p.scrollY--
			}
		case "down", "j":
			p.scrollY++
		case "home", "g":
			p.scrollY = 0
		case "end", "G":
			p.scrollY = 999999
		}
	}
	return p, nil
}

func (p doctorPanel) View() string {
	if p.running {
		return "\n  Running ct doctor…\n"
	}

	var lines []string
	lines = append(lines, "")

	for _, line := range strings.Split(p.output, "\n") {
		lines = append(lines, "  "+line)
	}
	lines = append(lines, "")

	if !p.runAt.IsZero() {
		age := time.Since(p.runAt).Round(time.Second)
		lines = append(lines, tuiStyleDim.Render(fmt.Sprintf(
			"  last run %s ago  ·  r to re-run", formatElapsed(age))))
	}

	total := len(lines)
	viewH := max(1, p.height-1)
	top := min(p.scrollY, max(0, total-viewH))
	end := min(top+viewH, total)
	return strings.Join(lines[top:end], "\n")
}

func (p doctorPanel) Title() string { return "Doctor" }

func (p doctorPanel) KeyHelp() string { return "r re-run  ↑↓/jk scroll  g/G top/bottom" }

func (p doctorPanel) OverlayActive() bool { return false }

func (p doctorPanel) PaletteActions(_ *cistern.Droplet) []PaletteAction { return nil }
