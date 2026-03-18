package main

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/MichielDean/cistern/internal/cistern"
	"github.com/MichielDean/cistern/internal/aqueduct"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const (
	refreshInterval          = 2 * time.Second
	recentEventLimit         = 5
	defaultDashboardHTMLPort = 5737

	// ANSI color codes
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorDim    = "\033[2m"
	colorReset  = "\033[0m"

	// ANSI cursor/screen
	clearScreen = "\033[2J\033[H"
)

const dashboardEasterEggText = `Four letters guard the gate you seek,
Each one counted in a way that's unique.
Not by their place in the alphabet's line,
But by where they stand among numbers prime.

Take each letter's secret prime,
Then trim away what's second in time.
What's left behind, when placed in a row,
Reveals the port where you must go.`

// CataractaInfo describes the state of a single aqueduct — its name, which droplet it carries, and where in the cataracta chain that droplet is.
type CataractaInfo struct {
	Name            string
	DropletID       string
	Step            string
	Steps           []string // workflow step names in order
	Elapsed         time.Duration
	CataractaIndex  int // 1-based index of current cataracta; 0 if unknown
	TotalCataractae int
}

// DashboardData holds all data required to render the dashboard.
type DashboardData struct {
	CataractaCount int
	FlowingCount   int
	QueuedCount    int
	DoneCount      int
	Cataractae     []CataractaInfo
	CisternItems   []*cistern.Droplet // flowing + queued
	RecentItems    []*cistern.Droplet // recently closed/escalated
	BlockedByMap   map[string]string  // droplet ID -> first blocking dep ID
	FarmRunning    bool
	FetchedAt      time.Time
}

// fetchDashboardData loads config and queue state into a DashboardData.
// On any error (missing config, missing DB) it returns a partial/drought result
// rather than an error, so the dashboard degrades gracefully.
func fetchDashboardData(cfgPath, dbPath string) *DashboardData {
	data := &DashboardData{FetchedAt: time.Now()}

	cfg, err := aqueduct.ParseAqueductConfig(cfgPath)
	if err != nil {
		// Config not found — show aqueducts closed.
		return data
	}

	// Build aqueduct list and load cataracta chain for each repo.
	type cataractaEntry struct {
		name string
		repo string
	}
	var configCataractae []cataractaEntry
	allSteps := map[string][]aqueduct.WorkflowCataracta{}
	cfgDir := filepath.Dir(cfgPath)
	for _, repo := range cfg.Repos {
		names := repoWorkerNames(repo)
		for _, name := range names {
			configCataractae = append(configCataractae, cataractaEntry{name, repo.Name})
		}
		data.CataractaCount += len(names)

		wfPath := repo.WorkflowPath
		if !filepath.IsAbs(wfPath) {
			wfPath = filepath.Join(cfgDir, wfPath)
		}
		if wf, wfErr := aqueduct.ParseWorkflow(wfPath); wfErr == nil {
			allSteps[repo.Name] = wf.Cataractae
		}
	}

	// Open queue — if it fails, show aqueducts as idle.
	c, err := cistern.New(dbPath, "")
	if err != nil {
		cataractae := make([]CataractaInfo, len(configCataractae))
		for i, ch := range configCataractae {
			ci := CataractaInfo{Name: ch.name}
			if wf, ok := allSteps[ch.repo]; ok {
				ci.Steps = stepNames(wf)
			}
			cataractae[i] = ci
		}
		data.Cataractae = cataractae
		return data
	}
	defer c.Close()

	allItems, err := c.List("", "")
	if err != nil {
		cataractae := make([]CataractaInfo, len(configCataractae))
		for i, ch := range configCataractae {
			ci := CataractaInfo{Name: ch.name}
			if wf, ok := allSteps[ch.repo]; ok {
				ci.Steps = stepNames(wf)
			}
			cataractae[i] = ci
		}
		data.Cataractae = cataractae
		return data
	}

	// Tally counts and build assignee map.
	assigneeMap := map[string]*cistern.Droplet{}
	for _, item := range allItems {
		switch item.Status {
		case "in_progress":
			data.FlowingCount++
			if item.Assignee != "" {
				assigneeMap[item.Assignee] = item
			}
		case "open":
			data.QueuedCount++
		case "delivered":
			data.DoneCount++
		}
	}

	// Build cataracta infos.
	cataractae := make([]CataractaInfo, len(configCataractae))
	for i, ch := range configCataractae {
		ci := CataractaInfo{Name: ch.name}
		if wf, ok := allSteps[ch.repo]; ok {
			ci.Steps = stepNames(wf)
		}
		if item, ok := assigneeMap[ch.name]; ok {
			ci.DropletID = item.ID
			ci.Step = item.CurrentCataracta
			ci.Elapsed = time.Since(item.UpdatedAt)
			wfCataractae := allSteps[ch.repo]
			ci.TotalCataractae = len(wfCataractae)
			ci.CataractaIndex = cataractaIndexInWorkflow(item.CurrentCataracta, wfCataractae)
		}
		cataractae[i] = ci
	}
	data.Cataractae = cataractae

	// Cistern: in_progress and open items; build blocked-by map.
	data.BlockedByMap = map[string]string{}
	for _, item := range allItems {
		if item.Status == "in_progress" || item.Status == "open" {
			data.CisternItems = append(data.CisternItems, item)
		}
		if item.Status == "open" {
			if blockedBy, err := c.GetBlockedBy(item.ID); err == nil && len(blockedBy) > 0 {
				data.BlockedByMap[item.ID] = blockedBy[0]
			}
		}
	}

	// Recent flow: most recently updated delivered/stagnant items.
	var recent []*cistern.Droplet
	for _, item := range allItems {
		if item.Status == "delivered" || item.Status == "stagnant" {
			recent = append(recent, item)
		}
	}
	sort.Slice(recent, func(i, j int) bool {
		return recent[i].UpdatedAt.After(recent[j].UpdatedAt)
	})
	if len(recent) > recentEventLimit {
		recent = recent[:recentEventLimit]
	}
	data.RecentItems = recent

	data.FarmRunning = true
	return data
}

// cataractaIndexInWorkflow returns the 1-based index of stepName in the cataracta list, or 0 if not found.
func cataractaIndexInWorkflow(stepName string, cataractae []aqueduct.WorkflowCataracta) int {
	for i, s := range cataractae {
		if s.Name == stepName {
			return i + 1
		}
	}
	return 0
}

// stepNames extracts step names from a workflow cataracta slice.
func stepNames(wf []aqueduct.WorkflowCataracta) []string {
	names := make([]string, len(wf))
	for i, s := range wf {
		names[i] = s.Name
	}
	return names
}

// progressBar renders a filled/empty progress bar of barWidth characters.
// E.g. stepIndex=2, total=6, barWidth=6 → "████░░"
func progressBar(stepIndex, total, barWidth int) string {
	if total <= 0 || stepIndex <= 0 {
		return strings.Repeat("░", barWidth)
	}
	filled := stepIndex * barWidth / total
	if filled > barWidth {
		filled = barWidth
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
}

// formatElapsed returns "Xm Ys" for durations >= 1 minute, "Xs" otherwise.
func formatElapsed(d time.Duration) string {
	d = d.Round(time.Second)
	if d < 0 {
		d = 0
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// padRight pads s to width using spaces, truncating if longer.
func padRight(s string, width int) string {
	r := []rune(s)
	if len(r) >= width {
		return string(r[:width])
	}
	return s + strings.Repeat(" ", width-len(r))
}

// renderAqueductRow renders a single aqueduct as a Roman aqueduct arch diagram.
// Each cataracta is an arch pier. The channel on top carries the flowing droplet.
// Returns a multi-line string (7 lines) suitable for the TUI dashboard.
//
// Example output (active in green, idle piers dim):
//
//	virgo  ╔════════════════════════════════════════════════════════╗
//	       ║ ≈ ≈ ≈  ci-pqz1q  implement  2m 14s  ████░░░░  ≈ ≈ ≈  ║
//	       ╚════════╤═══════════════╤═══════════════╤══════════════╝
//	                │               │               │               │
//	             ╔══╧══╗         ╔══╧══╗         ╔══╧══╗        ╔══╧══╗
//	             ║  ●  ║         ║  ○  ║         ║  ○  ║        ║  ○  ║
//	             ╚═════╝         ╚═════╝         ╚═════╝        ╚═════╝
//	           implement      adv-review            qa          delivery
func renderAqueductRow(ch CataractaInfo) string {
	const (
		colW    = 15 // visual width per cataracta column (label + spacing)
		pierInW = 5  // inner width of pier box: "  ●  " or " impl"
		nameW   = 10 // left label column width
	)

	steps := ch.Steps
	if len(steps) == 0 {
		steps = []string{"(empty)"}
	}
	n := len(steps)

	// Channel total inner width = n columns of colW, separated by ┬ joints.
	chanW := n*colW - 1

	// ── Line 1: channel top ────────────────────────────────────────────────
	prefix := "  " + padRight(ch.Name, nameW) + "  "
	indent := strings.Repeat(" ", len([]rune(prefix)))

	chanTop := prefix + colorDim + "╔" + strings.Repeat("═", chanW) + "╗" + colorReset

	// ── Line 2: water / droplet info ───────────────────────────────────────
	var waterInner string
	if ch.DropletID != "" {
		bar := progressBar(ch.CataractaIndex, ch.TotalCataractae, 8)
		content := fmt.Sprintf(" ≈ ≈  %s  %s  %s  ≈ ≈ ", ch.DropletID, formatElapsed(ch.Elapsed), bar)
		waterInner = padOrTruncCenter(content, chanW)
		waterInner = colorGreen + waterInner + colorReset
	} else {
		waterInner = colorDim + padOrTruncCenter(" — idle — ", chanW) + colorReset
	}
	chanMid := indent + colorDim + "║" + colorReset + waterInner + colorDim + "║" + colorReset

	// ── Line 3: channel bottom with ┬ connectors at each pier ──────────────
	var chanBot strings.Builder
	chanBot.WriteString(indent)
	chanBot.WriteString(colorDim + "╚" + colorReset)
	for i := range steps {
		half := (colW - 1) / 2
		rest := colW - 1 - half
		if i == 0 {
			chanBot.WriteString(colorDim + strings.Repeat("═", half) + "╤" + strings.Repeat("═", rest-1) + colorReset)
		} else {
			chanBot.WriteString(colorDim + strings.Repeat("═", half) + "╤" + strings.Repeat("═", rest-1) + colorReset)
		}
	}
	chanBot.WriteString(colorDim + "═╝" + colorReset)

	// ── Line 4: vertical stems from channel to pier caps ───────────────────
	var stems strings.Builder
	stems.WriteString(indent)
	for range steps {
		half := (colW - 1) / 2
		stems.WriteString(strings.Repeat(" ", half))
		stems.WriteString(colorDim + "│" + colorReset)
		stems.WriteString(strings.Repeat(" ", colW-half-1))
	}

	// ── Line 5: pier tops ╔══╧══╗ ─────────────────────────────────────────
	var pierTop strings.Builder
	pierTop.WriteString(indent)
	for i, step := range steps {
		half := (colW - 1) / 2
		pad := half - (pierInW/2 + 1)
		pierTop.WriteString(strings.Repeat(" ", pad))
		active := step == ch.Step && ch.DropletID != ""
		box := "╔" + strings.Repeat("═", pierInW) + "╗"
		if active {
			pierTop.WriteString(colorGreen + box + colorReset)
		} else {
			pierTop.WriteString(colorDim + box + colorReset)
		}
		_ = i
		pierTop.WriteString(strings.Repeat(" ", colW-pad-pierInW-2))
	}

	// ── Line 6: pier middle ║  ●  ║ ─────────────────────────────────────
	var pierMid strings.Builder
	pierMid.WriteString(indent)
	for _, step := range steps {
		half := (colW - 1) / 2
		pad := half - (pierInW/2 + 1)
		pierMid.WriteString(strings.Repeat(" ", pad))
		active := step == ch.Step && ch.DropletID != ""
		sym := "  ○  "
		if active {
			sym = "  ●  "
		}
		var body string
		if active {
			body = colorGreen + "║" + sym + "║" + colorReset
		} else {
			body = colorDim + "║" + sym + "║" + colorReset
		}
		pierMid.WriteString(body)
		pierMid.WriteString(strings.Repeat(" ", colW-pad-pierInW-2))
	}

	// ── Line 7: pier bottoms ╚═════╝ ─────────────────────────────────────
	var pierBot strings.Builder
	pierBot.WriteString(indent)
	for _, step := range steps {
		half := (colW - 1) / 2
		pad := half - (pierInW/2 + 1)
		pierBot.WriteString(strings.Repeat(" ", pad))
		active := step == ch.Step && ch.DropletID != ""
		box := "╚" + strings.Repeat("═", pierInW) + "╝"
		if active {
			pierBot.WriteString(colorGreen + box + colorReset)
		} else {
			pierBot.WriteString(colorDim + box + colorReset)
		}
		pierBot.WriteString(strings.Repeat(" ", colW-pad-pierInW-2))
	}

	// ── Line 8: labels ────────────────────────────────────────────────────
	var labels strings.Builder
	labels.WriteString(indent)
	for _, step := range steps {
		lbl := step
		if len([]rune(lbl)) > colW-1 {
			runes := []rune(lbl)
			lbl = string(runes[:colW-2]) + "…"
		}
		active := step == ch.Step && ch.DropletID != ""
		centered := padOrTruncCenter(lbl, colW)
		if active {
			labels.WriteString(colorGreen + centered + colorReset)
		} else {
			labels.WriteString(colorDim + centered + colorReset)
		}
	}

	return strings.Join([]string{
		chanTop,
		chanMid,
		chanBot.String(),
		stems.String(),
		pierTop.String(),
		pierMid.String(),
		pierBot.String(),
		labels.String(),
	}, "\n")
}

// padOrTruncCenter centers s within width w, padding with spaces.
// Truncates with … if s is too long.
func padOrTruncCenter(s string, w int) string {
	runes := []rune(s)
	if len(runes) > w {
		return string(runes[:w-1]) + "…"
	}
	total := w - len(runes)
	left := total / 2
	right := total - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}

// renderFlowGraphRow is kept for tests; the TUI now uses renderAqueductRow.
func renderFlowGraphRow(ch CataractaInfo) (graphLine, infoLine string) {
	const namePad = 12
	namePfx := padRight(ch.Name, namePad)
	const pfxWidth = namePad + 4

	if len(ch.Steps) == 0 {
		if ch.DropletID == "" {
			return "  " + namePfx + "  " + colorDim + "(idle)" + colorReset, ""
		}
		return "  " + colorGreen + namePfx + colorReset + "  " + ch.Step, ""
	}

	var g strings.Builder
	g.WriteString("  ")
	g.WriteString(colorDim + namePfx + colorReset)
	g.WriteString("  ")
	activeCol := -1
	visualCol := pfxWidth

	for i, step := range ch.Steps {
		if i > 0 {
			if step == ch.Step && ch.DropletID != "" {
				g.WriteString(colorDim + " ──" + colorReset + colorGreen + "●" + colorReset + colorDim + "──▶ " + colorReset)
			} else {
				g.WriteString(colorDim + " ──○──▶ " + colorReset)
			}
			visualCol += 8
		}
		if step == ch.Step && ch.DropletID != "" {
			g.WriteString(colorGreen + step + colorReset)
			activeCol = visualCol
		} else {
			g.WriteString(colorDim + step + colorReset)
		}
		visualCol += len([]rune(step))
	}

	graphLine = g.String()
	if activeCol >= 0 {
		bar := progressBar(ch.CataractaIndex, ch.TotalCataractae, 8)
		infoLine = strings.Repeat(" ", activeCol) + "↑ " + ch.Name + " · " + ch.DropletID + "  " + formatElapsed(ch.Elapsed) + "  " + bar
	}
	return
}

// renderDashboard produces the full dashboard string for the given data.
func renderDashboard(data *DashboardData) string {
	var sb strings.Builder
	sep := strings.Repeat("─", 70)

	// Aqueduct arch visualization — one arch diagram per aqueduct.
	if len(data.Cataractae) == 0 {
		sb.WriteString("  No aqueducts configured\n")
	} else {
		for i, ch := range data.Cataractae {
			if i > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(renderAqueductRow(ch))
			sb.WriteString("\n")
		}
	}
	sb.WriteString(sep + "\n")

	// Cistern counts.
	sb.WriteString(fmt.Sprintf("  ● %d flowing  ○ %d queued  ✓ %d delivered\n",
		data.FlowingCount, data.QueuedCount, data.DoneCount))
	sb.WriteString(sep + "\n")

	// Recent flow.
	sb.WriteString("  RECENT FLOW\n")
	if len(data.RecentItems) == 0 {
		sb.WriteString("  No recent flow.\n")
	} else {
		for _, item := range data.RecentItems {
			sb.WriteString("  " + renderRecentLine(item) + "\n")
		}
	}
	sb.WriteString(sep + "\n")

	// Footer.
	sb.WriteString(fmt.Sprintf("  q to quit  •  r to refresh  •  last update: %s\n",
		data.FetchedAt.Format("15:04:05")))

	return sb.String()
}

// renderRecentLine builds a recent-flow row string.
func renderRecentLine(item *cistern.Droplet) string {
	t := item.UpdatedAt.Format("15:04")
	id := padRight(item.ID, 10)
	step := item.CurrentCataracta
	if step == "" {
		step = "—"
	}
	status := displayStatus(item.Status)

	var icon string
	switch item.Status {
	case "delivered":
		icon = colorGreen + "✓" + colorReset
	case "stagnant":
		icon = colorRed + "✗" + colorReset
	default:
		icon = "·"
	}

	return fmt.Sprintf("  %s  %s  %-20s  %s  %s",
		t, id, step, icon, status)
}

// RunDashboard runs the refresh loop, writing to out. It reads single-byte
// events from inputCh: 'q' or 3 (Ctrl-C) to quit, 'r' to force refresh.
// The done channel is closed when the loop exits.
func RunDashboard(cfgPath, dbPath string, inputCh <-chan byte, out io.Writer) error {
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()

	// Initial render immediately.
	data := fetchDashboardData(cfgPath, dbPath)
	fmt.Fprint(out, clearScreen+renderDashboard(data))

	for {
		select {
		case <-ticker.C:
			data = fetchDashboardData(cfgPath, dbPath)
			fmt.Fprint(out, clearScreen+renderDashboard(data))

		case b, ok := <-inputCh:
			if !ok {
				return nil
			}
			switch b {
			case 'q', 'Q', 3: // 3 = Ctrl-C
				fmt.Fprint(out, clearScreen)
				return nil
			case 'r', 'R':
				data = fetchDashboardData(cfgPath, dbPath)
				fmt.Fprint(out, clearScreen+renderDashboard(data))
				ticker.Reset(refreshInterval)
			}
		}
	}
}

// startKeyboardReader starts a goroutine that puts stdin into raw mode and
// sends individual keystrokes to the returned channel. If raw mode fails
// (e.g., stdin is not a terminal), the channel is returned empty and only
// SIGINT will terminate the dashboard.
func startKeyboardReader() <-chan byte {
	ch := make(chan byte, 4)
	go func() {
		defer close(ch)

		fd := int(os.Stdin.Fd())
		if !term.IsTerminal(fd) {
			// Not interactive — block forever; SIGINT/signal will cancel.
			select {}
		}

		oldState, err := term.MakeRaw(fd)
		if err != nil {
			select {}
		}
		defer term.Restore(fd, oldState) //nolint:errcheck

		buf := make([]byte, 1)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil || n == 0 {
				return
			}
			ch <- buf[0]
		}
	}()
	return ch
}

// --- commands ---

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Live dashboard showing cataractae, cistern, and flow events",
	RunE:  runDashboard,
}

var feedCmd = &cobra.Command{
	Use:   "feed",
	Short: "Alias for dashboard",
	RunE:  runDashboard,
}

var dashboardHTML bool
var dashboardPort int

func dashboardListenAddr(port int) string {
	return fmt.Sprintf(":%d", port)
}

// renderDashboardHTML renders the full-page HTML dashboard using DashboardData —
// the same data source as the TUI. Layout mirrors the TUI: flow graph rows,
// cistern counts, recent flow, with auto-refresh every 3 seconds.
func renderDashboardHTML(data *DashboardData) string {
	var sb strings.Builder
	sb.WriteString(`<!doctype html><html><head><meta charset="utf-8">`)
	sb.WriteString(`<meta name="viewport" content="width=device-width,initial-scale=1">`)
	sb.WriteString(`<meta http-equiv="refresh" content="3">`)
	sb.WriteString(`<title>Cistern</title>`)
	sb.WriteString(`<style>
*{box-sizing:border-box}
body{font-family:ui-monospace,SFMono-Regular,Menlo,"Courier New",monospace;margin:0;background:#0b1020;color:#d6deeb;font-size:14px}
.wrap{max-width:1100px;margin:0 auto;padding:20px 24px}
.logo{color:#7ec8e3;font-size:13px;line-height:1.25;margin-bottom:12px;opacity:.85}
.sep{border:none;border-top:1px solid #1e2d4a;margin:12px 0}
.header{display:flex;align-items:baseline;gap:16px;margin-bottom:4px}
.title{font-size:18px;font-weight:700;color:#c8d8f0;letter-spacing:.05em}
.counts{font-size:13px;color:#7a91b8}
.flowing{color:#57d57a}.queued{color:#f0c86b}.delivered{color:#7a91b8}
.stagnant{color:#e05c5c}
.timestamp{font-size:12px;color:#4a5f80;margin-top:2px;margin-bottom:14px}
.section-label{font-size:11px;font-weight:700;letter-spacing:.08em;color:#4a5f80;text-transform:uppercase;margin:14px 0 6px}
/* Flow graph */
.flow-table{width:100%;border-collapse:collapse}
.flow-table td{padding:3px 0;vertical-align:top}
.aq-name{color:#5a7aaa;min-width:80px;padding-right:12px;font-size:13px}
.flow-graph{color:#8fa1c7;font-size:13px;letter-spacing:.02em;white-space:nowrap}
.flow-pointer{color:#6b85ad;font-size:12px;padding-left:80px;padding-bottom:6px}
.node-active{color:#57d57a;font-weight:700}
.node-idle{color:#3a4f6a}
.edge{color:#2a3d5a}
.progress-fill{color:#57d57a}
.progress-empty{color:#1e2d4a}
/* Cistern table */
.droplet-table{width:100%;border-collapse:collapse}
.droplet-table th{font-size:11px;font-weight:600;letter-spacing:.06em;text-transform:uppercase;color:#4a5f80;padding:4px 8px;border-bottom:1px solid #1e2d4a;text-align:left}
.droplet-table td{padding:5px 8px;border-bottom:1px solid #141e30;font-size:13px}
.droplet-table tr:last-child td{border-bottom:none}
.id{color:#7ec8e3}.muted{color:#4a5f80}.title-col{max-width:380px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.status-flowing{color:#57d57a}.status-queued{color:#f0c86b}.status-stagnant{color:#e05c5c}.status-delivered{color:#5a7aaa}
/* Easter egg */
#easter-egg{position:fixed;right:12px;bottom:10px;opacity:.2;cursor:default;user-select:none;font-size:11px;color:#7a91b8}
#easter-egg .hint{display:none;position:absolute;right:0;bottom:18px;white-space:pre-line;width:320px;padding:12px;border-radius:8px;background:#0f1728;border:1px solid #2a3d5a;color:#ced9f0;font-size:11px}
#easter-egg:hover .hint{display:block}
</style></head><body><div class="wrap">`)

	// Logo.
	sb.WriteString(`<div class="logo">◈  C I S T E R N  ◈</div>`)
	sb.WriteString(`<hr class="sep">`)

	// Header counts.
	sb.WriteString(fmt.Sprintf(
		`<div class="counts"><span class="flowing">● %d flowing</span>  <span class="queued">○ %d queued</span>  <span class="delivered">✓ %d delivered</span></div>`,
		data.FlowingCount, data.QueuedCount, data.DoneCount))
	sb.WriteString(fmt.Sprintf(`<div class="timestamp">last update: %s — auto-refreshes every 3s</div>`, data.FetchedAt.Format("15:04:05")))

	// Flow graph.
	sb.WriteString(`<hr class="sep">`)
	sb.WriteString(`<div class="section-label">Aqueducts</div>`)
	sb.WriteString(`<table class="flow-table">`)
	for _, ch := range data.Cataractae {
		sb.WriteString(`<tr><td class="aq-name">` + html.EscapeString(ch.Name) + `</td><td>`)
		// Build flow graph row.
		for i, step := range ch.Steps {
			if i > 0 {
				if ch.CataractaIndex > 0 && i == ch.CataractaIndex {
					sb.WriteString(`<span class="edge">──</span><span class="node-active">●</span><span class="edge">──▶ </span>`)
				} else {
					sb.WriteString(`<span class="edge"> ──○──▶ </span>`)
				}
			}
			if i == ch.CataractaIndex && ch.DropletID != "" {
				sb.WriteString(`<span class="node-active">` + html.EscapeString(step) + `</span>`)
			} else if ch.DropletID == "" {
				sb.WriteString(`<span class="node-idle">` + html.EscapeString(step) + `</span>`)
			} else {
				sb.WriteString(`<span class="flow-graph">` + html.EscapeString(step) + `</span>`)
			}
		}
		if ch.DropletID == "" {
			sb.WriteString(` <span class="muted">— idle</span>`)
		}
		sb.WriteString(`</td></tr>`)
		// Pointer row.
		if ch.DropletID != "" {
			elapsed := formatElapsed(ch.Elapsed)
			pct := 0
			if ch.TotalCataractae > 0 {
				pct = (ch.CataractaIndex * 8) / ch.TotalCataractae
			}
			bar := strings.Repeat("█", pct) + strings.Repeat("░", 8-pct)
			sb.WriteString(fmt.Sprintf(
				`<tr><td></td><td class="flow-pointer">↑ %s · <span class="id">%s</span>  %s  <span class="progress-fill">%s</span><span class="progress-empty">%s</span></td></tr>`,
				html.EscapeString(ch.Name),
				html.EscapeString(ch.DropletID),
				html.EscapeString(elapsed),
				html.EscapeString(bar[:pct]),
				html.EscapeString(bar[pct:]),
			))
		}
	}
	if len(data.Cataractae) == 0 {
		sb.WriteString(`<tr><td colspan="2" class="muted">No aqueducts configured</td></tr>`)
	}
	sb.WriteString(`</table>`)

	// Cistern — active droplets.
	sb.WriteString(`<hr class="sep">`)
	sb.WriteString(`<div class="section-label">Cistern</div>`)
	sb.WriteString(`<table class="droplet-table"><thead><tr><th>ID</th><th>Title</th><th>Status</th><th>Cataracta</th><th>Elapsed</th></tr></thead><tbody>`)
	if len(data.CisternItems) == 0 {
		sb.WriteString(`<tr><td colspan="5" class="muted">Cistern dry.</td></tr>`)
	} else {
		for _, d := range data.CisternItems {
			statusClass := "status-" + d.Status
			step := d.CurrentCataracta
			if step == "" {
				step = "—"
			}
			elapsed := "—"
			if d.Status == "in_progress" {
				elapsed = formatElapsed(time.Since(d.UpdatedAt))
			}
			title := d.Title
			if len(title) > 60 {
				title = title[:57] + "..."
			}
			sb.WriteString(fmt.Sprintf(
				`<tr><td class="id">%s</td><td class="title-col">%s</td><td class="%s">%s</td><td class="muted">%s</td><td class="muted">%s</td></tr>`,
				html.EscapeString(d.ID), html.EscapeString(title),
				statusClass, html.EscapeString(displayStatus(d.Status)),
				html.EscapeString(step), html.EscapeString(elapsed),
			))
		}
	}
	sb.WriteString(`</tbody></table>`)

	// Recent flow.
	sb.WriteString(`<hr class="sep">`)
	sb.WriteString(`<div class="section-label">Recent Flow</div>`)
	sb.WriteString(`<table class="droplet-table"><thead><tr><th>Time</th><th>ID</th><th>Title</th><th>Cataracta</th><th></th></tr></thead><tbody>`)
	if len(data.RecentItems) == 0 {
		sb.WriteString(`<tr><td colspan="5" class="muted">No recent flow.</td></tr>`)
	} else {
		for _, d := range data.RecentItems {
			icon := "✓"
			cls := "status-delivered"
			if d.Status == "stagnant" {
				icon = "✗"
				cls = "status-stagnant"
			}
			title := d.Title
			if len(title) > 60 {
				title = title[:57] + "..."
			}
			sb.WriteString(fmt.Sprintf(
				`<tr><td class="muted">%s</td><td class="id">%s</td><td class="title-col">%s</td><td class="muted">%s</td><td class="%s">%s</td></tr>`,
				html.EscapeString(d.UpdatedAt.Format("15:04")),
				html.EscapeString(d.ID),
				html.EscapeString(title),
				html.EscapeString(d.CurrentCataracta),
				cls, icon,
			))
		}
	}
	sb.WriteString(`</tbody></table>`)

	sb.WriteString(`<div id="easter-egg" aria-hidden="true">◈<span class="hint">`)
	sb.WriteString(html.EscapeString(dashboardEasterEggText))
	sb.WriteString(`</span></div>`)
	sb.WriteString(`</div></body></html>`)
	return sb.String()
}

func runDashboardHTML(cfgPath, dbPath string, out io.Writer) error {
	addr := dashboardListenAddr(dashboardPort)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data := fetchDashboardData(cfgPath, dbPath)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, renderDashboardHTML(data))
	})

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	fmt.Fprintf(out, "Dashboard available at http://localhost:%d\n", dashboardPort)
	fmt.Fprintln(out, "Press Ctrl-C to stop.")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	select {
	case err := <-errCh:
		if err != nil {
			return err
		}
		return nil
	case <-sigCh:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	}
}

func runDashboard(cmd *cobra.Command, args []string) error {
	cfgPath := resolveConfigPath()
	dbPath := resolveDBPath()

	if dashboardHTML {
		return runDashboardHTML(cfgPath, dbPath, os.Stdout)
	}

	return RunDashboardTUI(cfgPath, dbPath)
}

func init() {
	dashboardCmd.Flags().BoolVar(&dashboardHTML, "html", false, "serve dashboard as HTML instead of terminal UI")
	dashboardCmd.Flags().IntVar(&dashboardPort, "port", defaultDashboardHTMLPort, "port for --html dashboard server")
	feedCmd.Flags().BoolVar(&dashboardHTML, "html", false, "serve dashboard as HTML instead of terminal UI")
	feedCmd.Flags().IntVar(&dashboardPort, "port", defaultDashboardHTMLPort, "port for --html dashboard server")

	rootCmd.AddCommand(dashboardCmd)
	rootCmd.AddCommand(feedCmd)
}
