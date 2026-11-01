// Package ui is the Bubble Tea front end: pick installed Steam games, bulk uninstall.
package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"steamsweep/internal/steam"
)

var (
	cAccent = lipgloss.Color("#66C0F4")
	cDanger = lipgloss.Color("#FF6B81")
	cOK     = lipgloss.Color("#7EE787")
	cWarn   = lipgloss.Color("#F9E2AF")
	cText   = lipgloss.Color("#CDD6F4")
	cMuted  = lipgloss.Color("#7F849C")
	cFaint  = lipgloss.Color("#45475A")
	cCursor = lipgloss.Color("#1E1E2E")

	sTitle  = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	sMuted  = lipgloss.NewStyle().Foreground(cMuted)
	sFaint  = lipgloss.NewStyle().Foreground(cFaint)
	sKey    = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	sDanger = lipgloss.NewStyle().Foreground(cDanger).Bold(true)
	sOK     = lipgloss.NewStyle().Foreground(cOK)
	sWarn   = lipgloss.NewStyle().Foreground(cWarn)
)

type mode int

const (
	modeList mode = iota
	modeFilter
	modeConfirm
	modeDeleting
	modeDone
)

type sortMode int

const (
	sortSize sortMode = iota
	sortPlayed
	sortUpdated
	sortName
	numSortModes
)

var sortLabels = [numSortModes]string{"size", "last played", "last updated", "name"}

// less orders a before b in the mode's natural direction: biggest first, and for the date modes
// oldest (or never) first, since those are the games you most likely want gone.
func (md sortMode) less(a, b steam.Game) bool {
	switch md {
	case sortPlayed:
		if a.LastPlayed != b.LastPlayed {
			return a.LastPlayed < b.LastPlayed
		}
	case sortUpdated:
		if a.LastUpdated != b.LastUpdated {
			return a.LastUpdated < b.LastUpdated
		}
	case sortName:
		la, lb := strings.ToLower(a.Name), strings.ToLower(b.Name)
		if la != lb {
			return la < lb
		}
		return false
	}
	return a.Size > b.Size
}

type deletedMsg struct{ err error }
type closedMsg struct{ err error }

// Options configures a UI session.
type Options struct {
	DryRun       bool
	SteamRunning bool
	Version      string
	CloseSteam   bool // close Steam automatically before uninstalling
}

type model struct {
	opts      Options
	all       []steam.Game
	multiLib  bool
	view      []int // indices into all, after filter/tools
	cursor    int
	offset    int
	selected  map[string]bool
	w, h      int
	mode      mode
	filter    string
	showTools bool
	sortMode  sortMode
	reverse   bool
	prefix    bool

	closing  bool
	closeErr string
	queue    []steam.Game
	qIdx     int
	freed    int64
	failed   []string
}

func key(g steam.Game) string { return g.Library + "\x00" + g.AppID }

// Run starts the TUI.
func Run(games []steam.Game, o Options) error {
	libs := map[string]bool{}
	for _, g := range games {
		libs[g.Library] = true
	}
	m := &model{opts: o, all: games, multiLib: len(libs) > 1, selected: map[string]bool{}, w: 80, h: 24}
	m.rebuild()
	_, err := tea.NewProgram(m).Run()
	return err
}

func (m *model) Init() tea.Cmd { return nil }

func (m *model) rebuild() {
	var cur string
	if m.cursor < len(m.view) {
		cur = key(m.all[m.view[m.cursor]])
	}
	order := make([]int, 0, len(m.all))
	q := strings.ToLower(m.filter)
	for i, g := range m.all {
		if g.Tool && !m.showTools {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(g.Name), q) {
			continue
		}
		order = append(order, i)
	}
	sort.SliceStable(order, func(a, b int) bool {
		ga, gb := m.all[order[a]], m.all[order[b]]
		if m.reverse {
			return m.sortMode.less(gb, ga)
		}
		return m.sortMode.less(ga, gb)
	})
	m.view = order
	m.cursor = 0
	for i, idx := range m.view {
		if key(m.all[idx]) == cur {
			m.cursor = i
		}
	}
	m.clamp()
}

func (m *model) rows() int {
	if r := m.h - 6; r > 3 {
		return r
	}
	return 3
}

func (m *model) clamp() {
	if m.cursor >= len(m.view) {
		m.cursor = len(m.view) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	r := m.rows()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+r {
		m.offset = m.cursor - r + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m *model) selectedGames() []steam.Game {
	var out []steam.Game
	for _, g := range m.all {
		if m.selected[key(g)] {
			out = append(out, g)
		}
	}
	return out
}

func (m *model) selectedSize() (n int, size int64) {
	for _, g := range m.selectedGames() {
		n++
		size += g.Size
	}
	return
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.clamp()
	case closedMsg:
		m.closing = false
		m.opts.SteamRunning = steam.Running()
		if msg.err != nil {
			m.closeErr = msg.err.Error()
			m.mode = modeConfirm // back to the confirm screen so nothing is deleted under a live Steam
			return m, nil
		}
		m.closeErr = ""
		if m.mode == modeDeleting {
			return m, m.uninstallCmd()
		}
	case deletedMsg:
		g := m.queue[m.qIdx]
		if msg.err != nil {
			m.failed = append(m.failed, fmt.Sprintf("%s: %v", g.Name, msg.err))
		} else {
			m.freed += g.Size
		}
		m.qIdx++
		if m.qIdx >= len(m.queue) {
			m.mode = modeDone
			return m, nil
		}
		return m, m.uninstallCmd()
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		switch m.mode {
		case modeList:
			return m.updateList(msg)
		case modeFilter:
			m.updateFilter(msg)
		case modeConfirm:
			return m.updateConfirm(msg)
		case modeDone:
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *model) updateList(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	page := m.rows() - 1
	switch msg.String() {
	case "q", "esc":
		if m.filter != "" && msg.String() == "esc" {
			m.filter = ""
			m.rebuild()
			return m, nil
		}
		return m, tea.Quit
	case "up", "k":
		m.cursor--
	case "down", "j":
		m.cursor++
	case "pgup", "ctrl+u":
		m.cursor -= page
	case "pgdown", "ctrl+d":
		m.cursor += page
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		m.cursor = len(m.view) - 1
	case "space", " ", "x":
		if len(m.view) > 0 {
			k := key(m.all[m.view[m.cursor]])
			m.selected[k] = !m.selected[k]
			m.cursor++
		}
	case "a":
		allOn := true
		for _, idx := range m.view {
			if !m.selected[key(m.all[idx])] {
				allOn = false
				break
			}
		}
		for _, idx := range m.view {
			m.selected[key(m.all[idx])] = !allOn
		}
	case "i":
		for _, idx := range m.view {
			k := key(m.all[idx])
			m.selected[k] = !m.selected[k]
		}
	case "c":
		m.selected = map[string]bool{}
	case "/":
		m.mode = modeFilter
	case "s":
		m.sortMode = (m.sortMode + 1) % numSortModes
		m.rebuild()
	case "r":
		m.reverse = !m.reverse
		m.rebuild()
	case "t":
		m.showTools = !m.showTools
		m.rebuild()
	case "p":
		m.prefix = !m.prefix
	case "enter":
		if n, _ := m.selectedSize(); n > 0 {
			m.mode = modeConfirm
		}
	}
	m.clamp()
	return m, nil
}

func (m *model) updateFilter(msg tea.KeyPressMsg) {
	switch msg.String() {
	case "enter":
		m.mode = modeList
	case "esc":
		m.filter = ""
		m.mode = modeList
		m.rebuild()
	case "backspace":
		if r := []rune(m.filter); len(r) > 0 {
			m.filter = string(r[:len(r)-1])
			m.rebuild()
		}
	case "up":
		m.cursor--
		m.clamp()
	case "down":
		m.cursor++
		m.clamp()
	default:
		if msg.Text != "" {
			m.filter += msg.Text
			m.rebuild()
		}
	}
}

func (m *model) updateConfirm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.closing {
		return m, nil
	}
	switch msg.String() {
	case "y", "enter":
		m.queue = m.selectedGames()
		m.qIdx, m.freed, m.failed = 0, 0, nil
		m.mode = modeDeleting
		if m.opts.SteamRunning && m.opts.CloseSteam {
			m.closing = true
			return m, closeCmd()
		}
		return m, m.uninstallCmd()
	case "c":
		if m.opts.SteamRunning {
			m.closing, m.closeErr = true, ""
			return m, closeCmd()
		}
	case "p":
		m.prefix = !m.prefix
	case "n", "esc", "q":
		m.mode = modeList
	}
	return m, nil
}

func closeCmd() tea.Cmd {
	return func() tea.Msg { return closedMsg{steam.Close()} }
}

func (m *model) uninstallCmd() tea.Cmd {
	g, o := m.queue[m.qIdx], steam.UninstallOpts{Prefix: m.prefix, DryRun: m.opts.DryRun}
	return func() tea.Msg { return deletedMsg{steam.Uninstall(g, o)} }
}

func humanSize(b int64) string {
	const unit = 1000 // Steam and most file managers report decimal units
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "kMGTPE"[exp])
}

func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = ansi.Truncate(s, w, "…")
	if pad := w - ansi.StringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

func bar(frac float64, w int) string {
	n := int(frac*float64(w) + 0.5)
	if frac > 0 && n == 0 {
		n = 1
	}
	return lipgloss.NewStyle().Foreground(cAccent).Render(strings.Repeat("━", n)) +
		sFaint.Render(strings.Repeat("─", w-n))
}

func (m *model) View() tea.View {
	var s string
	switch m.mode {
	case modeConfirm:
		s = m.viewConfirm()
	case modeDeleting, modeDone:
		s = m.viewProgress()
	default:
		s = m.viewList()
	}
	v := tea.NewView(s)
	v.AltScreen = true
	return v
}

func (m *model) viewList() string {
	var b strings.Builder
	n, size := m.selectedSize()

	title := sTitle.Render("steamsweep")
	if m.opts.DryRun {
		title += " " + sWarn.Render("[dry run]")
	}
	count := sMuted.Render(fmt.Sprintf("%d %s · %s", len(m.view), plural(len(m.view)), m.sortSummary()))
	b.WriteString(title + "  " + count + "\n")
	switch {
	case m.mode == modeFilter:
		b.WriteString(sKey.Render("/") + m.filter + sKey.Render("▏") + "\n\n")
	case m.filter != "":
		b.WriteString(sMuted.Render("filter: ") + m.filter + sMuted.Render("  (esc clears)") + "\n\n")
	default:
		b.WriteString(sMuted.Render("Mark games, then press enter to uninstall them.") + "\n\n")
	}

	var maxSize int64 = 1
	for _, idx := range m.view {
		if sz := m.all[idx].Size; sz > maxSize {
			maxSize = sz
		}
	}

	const barW, sizeW, playW = 12, 10, 10
	now := time.Now()
	libW := 0
	if m.multiLib {
		libW = 14
	}
	nameW := m.w - (4 + 2 + barW + 2 + sizeW + 2 + playW + 1) - libW
	if nameW < 10 {
		nameW = 10
	}

	if len(m.view) == 0 {
		b.WriteString(sMuted.Render("  No games match.") + "\n")
	}
	end := m.offset + m.rows()
	if end > len(m.view) {
		end = len(m.view)
	}
	printed := 0
	for i := m.offset; i < end; i++ {
		g := m.all[m.view[i]]
		on := m.selected[key(g)]
		here := i == m.cursor

		mark := sFaint.Render("○")
		name := lipgloss.NewStyle().Foreground(cText).Render(fit(g.Name, nameW))
		sz := sMuted.Render(fmt.Sprintf("%*s", sizeW, humanSize(g.Size)))
		if on {
			mark = sDanger.Render("●")
			name = lipgloss.NewStyle().Foreground(cDanger).Render(fit(g.Name, nameW))
			sz = sDanger.Render(fmt.Sprintf("%*s", sizeW, humanSize(g.Size)))
		}
		pointer := "  "
		if here {
			pointer = sKey.Render("▌ ")
			name = lipgloss.NewStyle().Bold(true).Render(name)
		}
		played := sFaint.Render(fmt.Sprintf("%*s", playW, ago(g.LastPlayed, now)))
		if g.LastPlayed > 0 {
			played = sMuted.Render(fmt.Sprintf("%*s", playW, ago(g.LastPlayed, now)))
		}
		line := pointer + mark + " " + name + "  " + bar(float64(g.Size)/float64(maxSize), barW) + "  " + sz + "  " + played
		if m.multiLib {
			line += " " + sFaint.Render(fit(shortLib(g.Library), libW-1))
		}
		if here {
			line = lipgloss.NewStyle().Background(lipgloss.Color("#313244")).Width(m.w).Render(line)
		}
		b.WriteString(line + "\n")
		printed++
	}
	for ; printed < m.rows(); printed++ {
		b.WriteString("\n")
	}

	b.WriteString("\n")
	status := sMuted.Render("nothing selected")
	if n > 0 {
		status = sDanger.Render(fmt.Sprintf("%d selected", n)) + sMuted.Render(" · frees ") + sOK.Render(humanSize(size))
	}
	if m.opts.SteamRunning {
		status += "   " + sWarn.Render("⚠ Steam is running")
	}
	b.WriteString(status + "\n")
	b.WriteString(help([][2]string{{"↑↓", "move"}, {"space", "mark"}, {"a", "all"}, {"i", "invert"}, {"/", "filter"},
		{"s", "sort"}, {"r", "reverse"}, {"t", "tools"}, {"enter", "uninstall"}, {"q", "quit"}}))
	return b.String()
}

func ago(unix int64, now time.Time) string {
	if unix <= 0 {
		return "never"
	}
	d := now.Sub(time.Unix(unix, 0))
	switch {
	case d < 24*time.Hour:
		return "today"
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(d.Hours()/24/30))
	}
	return fmt.Sprintf("%dy ago", int(d.Hours()/24/365))
}

func (m *model) sortSummary() string {
	arrow := "↑" // oldest / A-Z first
	if m.sortMode == sortSize {
		arrow = "↓" // biggest first
	}
	if m.reverse {
		arrow = map[string]string{"↑": "↓", "↓": "↑"}[arrow]
	}
	return "sort: " + sortLabels[m.sortMode] + " " + arrow
}

func shortLib(p string) string {
	parts := strings.Split(strings.ReplaceAll(p, `\`, "/"), "/")
	return parts[len(parts)-1]
}

func help(items [][2]string) string {
	parts := make([]string, len(items))
	for i, it := range items {
		parts[i] = sKey.Render(it[0]) + " " + sMuted.Render(it[1])
	}
	return strings.Join(parts, sFaint.Render(" · "))
}

func (m *model) box(content string) string {
	w := 64
	if m.w-4 < w {
		w = m.w - 4
	}
	if w < 20 {
		w = 20
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cDanger).Padding(1, 2).Width(w).Render(content)
	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, box)
}

func (m *model) viewConfirm() string {
	games := m.selectedGames()
	_, size := m.selectedSize()
	var b strings.Builder
	b.WriteString(sDanger.Render(fmt.Sprintf("Uninstall %d %s?", len(games), plural(len(games)))) + "\n")
	b.WriteString(sMuted.Render("Frees ") + sOK.Render(humanSize(size)) + sMuted.Render(". This deletes the files; it cannot be undone.") + "\n\n")
	for i, g := range games {
		if i == 8 {
			b.WriteString(sMuted.Render(fmt.Sprintf("  … and %d more\n", len(games)-8)))
			break
		}
		b.WriteString("  " + fit(g.Name, 40) + sMuted.Render(fmt.Sprintf("%10s", humanSize(g.Size))) + "\n")
	}
	pfx := sFaint.Render("○") + sMuted.Render(" keep Proton prefix (saves)")
	if m.prefix {
		pfx = sDanger.Render("●") + " also delete Proton prefix and shader cache"
	}
	b.WriteString("\n" + pfx + "\n")
	switch {
	case m.closing:
		b.WriteString("\n" + sWarn.Render("Closing Steam…") + "\n")
	case m.opts.SteamRunning && m.opts.CloseSteam:
		b.WriteString("\n" + sWarn.Render("⚠ Steam is running and will be closed first.") + "\n")
	case m.opts.SteamRunning:
		b.WriteString("\n" + sWarn.Render("⚠ Steam is running. Press c to close it, or it may restore the library state.") + "\n")
	}
	if m.closeErr != "" {
		b.WriteString(sDanger.Render("✗ "+m.closeErr) + "\n")
	}
	if m.opts.DryRun {
		b.WriteString("\n" + sWarn.Render("dry run: nothing will be deleted") + "\n")
	}
	keys := [][2]string{{"y", "uninstall"}, {"p", "prefix"}, {"esc", "cancel"}}
	if m.opts.SteamRunning {
		keys = append(keys[:2], [2]string{"c", "close Steam"}, keys[2])
	}
	b.WriteString("\n" + help(keys))
	return m.box(b.String())
}

func (m *model) viewProgress() string {
	total := len(m.queue)
	var b strings.Builder
	if m.mode == modeDone {
		b.WriteString(sOK.Render("Done") + "\n\n")
		b.WriteString(fmt.Sprintf("Uninstalled %d of %d %s, freed %s.\n", total-len(m.failed), total, plural(total), sOK.Render(humanSize(m.freed))))
		for _, f := range m.failed {
			b.WriteString(sDanger.Render("✗ ") + f + "\n")
		}
		b.WriteString("\n" + sMuted.Render("Press any key to exit."))
		return m.box(b.String())
	}
	if m.closing {
		b.WriteString(sTitle.Render("Closing Steam…") + "\n\n" + sMuted.Render("Waiting for the client to exit."))
		return m.box(b.String())
	}
	b.WriteString(sTitle.Render("Uninstalling…") + "\n\n")
	b.WriteString(fit(m.queue[m.qIdx].Name, 50) + "\n\n")
	b.WriteString(bar(float64(m.qIdx)/float64(total), 40) + sMuted.Render(fmt.Sprintf("  %d/%d", m.qIdx, total)))
	return m.box(b.String())
}

func plural(n int) string {
	if n == 1 {
		return "game"
	}
	return "games"
}
