// Package ui is the Bubble Tea front end: pick installed Steam games and Steam clutter, bulk delete.
package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/ShatteredEagle/steamsweep/internal/clean"
	"github.com/ShatteredEagle/steamsweep/internal/steam"
)

var (
	cAccent = lipgloss.Color("#66C0F4")
	cDanger = lipgloss.Color("#FF6B81")
	cOK     = lipgloss.Color("#7EE787")
	cWarn   = lipgloss.Color("#F9E2AF")
	cText   = lipgloss.Color("#CDD6F4")
	cMuted  = lipgloss.Color("#7F849C")
	cFaint  = lipgloss.Color("#45475A")

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

type tab int

const (
	tabGames tab = iota
	tabClean
)

type sortKind int

const (
	sortSize sortKind = iota
	sortPlayed
	sortUpdated
	sortName
	sortKindTag
)

var sortLabels = map[sortKind]string{
	sortSize: "size", sortPlayed: "last played", sortUpdated: "last updated", sortName: "name", sortKindTag: "type",
}

var sortCycles = [2][]sortKind{
	tabGames: {sortSize, sortPlayed, sortUpdated, sortName},
	tabClean: {sortSize, sortKindTag, sortName},
}

// entry is one selectable row: either an installed game or a cleanup item.
type entry struct {
	key, name, tag  string
	size            int64
	played, updated int64
	tool, safe      bool
	game            *steam.Game
	item            *clean.Item
}

// less orders a before b in the kind's natural direction: biggest first, and for dates oldest
// (or never) first, since those are the entries you most likely want gone.
func (k sortKind) less(a, b entry) bool {
	switch k {
	case sortPlayed:
		if a.played != b.played {
			return a.played < b.played
		}
	case sortUpdated:
		if a.updated != b.updated {
			return a.updated < b.updated
		}
	case sortKindTag:
		if a.tag != b.tag {
			return a.tag < b.tag
		}
	case sortName:
		return strings.ToLower(a.name) < strings.ToLower(b.name)
	}
	return a.size > b.size
}

type deletedMsg struct{ err error }
type closedMsg struct{ err error }
type scannedMsg struct{ res clean.Result }

// Options configures a UI session.
type Options struct {
	DryRun       bool
	SteamRunning bool
	Version      string
	CloseSteam   bool // close Steam automatically before deleting
	Root         string
	Libs         []string
}

type model struct {
	opts     Options
	tabs     [2][]entry
	multiLib bool
	tab      tab
	view     []entry
	cursor   int
	offset   int
	selected map[string]bool
	w, h     int
	mode     mode
	filter   string

	showTools bool
	sortIdx   [2]int
	reverse   [2]bool
	prefix    bool

	scanning, scanned bool
	warnings          []string

	closing  bool
	closeErr string
	queue    []entry
	qIdx     int
	freed    int64
	failed   []string
}

// Run starts the TUI.
func Run(games []steam.Game, o Options) error {
	_, err := tea.NewProgram(newModel(games, o)).Run()
	return err
}

func newModel(games []steam.Game, o Options) *model {
	libs := map[string]bool{}
	m := &model{opts: o, selected: map[string]bool{}, w: 80, h: 24}
	for i := range games {
		g := &games[i]
		libs[g.Library] = true
		m.tabs[tabGames] = append(m.tabs[tabGames], entry{
			key: g.Library + "\x00" + g.AppID, name: g.Name, size: g.Size, played: g.LastPlayed,
			updated: g.LastUpdated, tool: g.Tool, safe: true, game: g,
		})
	}
	m.multiLib = len(libs) > 1
	m.rebuild()
	return m
}

func (m *model) Init() tea.Cmd { return nil }

func (m *model) kind() sortKind { return sortCycles[m.tab][m.sortIdx[m.tab]%len(sortCycles[m.tab])] }

func (m *model) rebuild() {
	var cur string
	if m.cursor < len(m.view) {
		cur = m.view[m.cursor].key
	}
	q := strings.ToLower(m.filter)
	view := make([]entry, 0, len(m.tabs[m.tab]))
	for _, e := range m.tabs[m.tab] {
		if m.tab == tabGames && e.tool && !m.showTools {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(e.name), q) {
			continue
		}
		view = append(view, e)
	}
	k, rev := m.kind(), m.reverse[m.tab]
	sort.SliceStable(view, func(i, j int) bool {
		if rev {
			return k.less(view[j], view[i])
		}
		return k.less(view[i], view[j])
	})
	m.view = view
	m.cursor = 0
	for i, e := range m.view {
		if e.key == cur {
			m.cursor = i
		}
	}
	m.clamp()
}

func (m *model) rows() int {
	if r := m.h - 7; r > 3 {
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

func (m *model) selectedEntries() []entry {
	var out []entry
	for t := range m.tabs {
		for _, e := range m.tabs[t] {
			if m.selected[e.key] {
				out = append(out, e)
			}
		}
	}
	return out
}

func (m *model) selectedSize() (n int, size int64) {
	for _, e := range m.selectedEntries() {
		n++
		size += e.size
	}
	return
}

func (m *model) countSelected(t tab) (n int) {
	for _, e := range m.tabs[t] {
		if m.selected[e.key] {
			n++
		}
	}
	return
}

func (m *model) scanCmd() tea.Cmd {
	var games []steam.Game
	for _, e := range m.tabs[tabGames] {
		games = append(games, *e.game)
	}
	root, libs := m.opts.Root, m.opts.Libs
	return func() tea.Msg { return scannedMsg{clean.Scan(root, libs, games)} }
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.clamp()
	case scannedMsg:
		m.scanning, m.scanned = false, true
		m.warnings = msg.res.Warnings
		for i := range msg.res.Items {
			it := &msg.res.Items[i]
			m.tabs[tabClean] = append(m.tabs[tabClean], entry{key: "clean\x00" + it.Key, name: it.Name,
				size: it.Size, tag: string(it.Kind), safe: it.Safe, item: it})
		}
		m.rebuild()
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
			return m, m.runCmd()
		}
	case deletedMsg:
		e := m.queue[m.qIdx]
		if msg.err != nil {
			m.failed = append(m.failed, fmt.Sprintf("%s: %v", e.name, msg.err))
		} else {
			m.freed += e.size
		}
		m.qIdx++
		if m.qIdx >= len(m.queue) {
			m.mode = modeDone
			return m, nil
		}
		return m, m.runCmd()
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

func (m *model) switchTab() tea.Cmd {
	m.tab = 1 - m.tab
	m.filter, m.cursor, m.offset = "", 0, 0
	m.rebuild()
	if m.tab == tabClean && !m.scanned && !m.scanning {
		m.scanning = true
		return m.scanCmd()
	}
	return nil
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
	case "tab":
		return m, m.switchTab()
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
			k := m.view[m.cursor].key
			m.selected[k] = !m.selected[k]
			m.cursor++
		}
	case "a":
		// "all" skips entries that may hold personal data or progress (recordings, prefixes, Proton builds).
		allOn := true
		for _, e := range m.view {
			if e.safe && !m.selected[e.key] {
				allOn = false
				break
			}
		}
		for _, e := range m.view {
			if e.safe {
				m.selected[e.key] = !allOn
			}
		}
	case "i":
		for _, e := range m.view {
			m.selected[e.key] = !m.selected[e.key]
		}
	case "c":
		m.selected = map[string]bool{}
	case "/":
		m.mode = modeFilter
	case "s":
		m.sortIdx[m.tab] = (m.sortIdx[m.tab] + 1) % len(sortCycles[m.tab])
		m.rebuild()
	case "r":
		m.reverse[m.tab] = !m.reverse[m.tab]
		m.rebuild()
	case "t":
		if m.tab == tabGames {
			m.showTools = !m.showTools
			m.rebuild()
		}
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
		m.queue = m.selectedEntries()
		m.qIdx, m.freed, m.failed = 0, 0, nil
		m.mode = modeDeleting
		if m.opts.SteamRunning && m.opts.CloseSteam {
			m.closing = true
			return m, closeCmd()
		}
		return m, m.runCmd()
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

func (m *model) runCmd() tea.Cmd {
	e, o := m.queue[m.qIdx], m.opts
	prefix := m.prefix
	return func() tea.Msg {
		if e.game != nil {
			return deletedMsg{steam.Uninstall(*e.game, steam.UninstallOpts{Prefix: prefix, DryRun: o.DryRun})}
		}
		return deletedMsg{clean.Remove(*e.item, o.Root, o.Libs, o.DryRun)}
	}
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
	if m.kind() == sortSize {
		arrow = "↓" // biggest first
	}
	if m.reverse[m.tab] {
		arrow = map[string]string{"↑": "↓", "↓": "↑"}[arrow]
	}
	return "sort: " + sortLabels[m.kind()] + " " + arrow
}

func plural(n int) string {
	if n == 1 {
		return "game"
	}
	return "games"
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

func (m *model) tabBar() string {
	label := func(t tab, name string, n int) string {
		badge := ""
		if n > 0 {
			badge = sDanger.Render(fmt.Sprintf(" ●%d", n))
		}
		if m.tab == t {
			pill := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1E1E2E")).Background(cAccent).Padding(0, 1)
			return pill.Render(name) + badge
		}
		return sMuted.Render(" "+name) + badge
	}
	return label(tabGames, "Games", m.countSelected(tabGames)) + "  " + label(tabClean, "Cleanup", m.countSelected(tabClean))
}

func (m *model) viewList() string {
	var b strings.Builder
	n, size := m.selectedSize()

	title := sTitle.Render("steamsweep")
	if m.opts.DryRun {
		title += " " + sWarn.Render("[dry run]")
	}
	b.WriteString(title + "   " + m.tabBar() + "\n")
	count := fmt.Sprintf("%d %s", len(m.view), plural(len(m.view)))
	if m.tab == tabClean {
		count = fmt.Sprintf("%d items", len(m.view))
	}
	info := sMuted.Render(count + " · " + m.sortSummary())
	switch {
	case m.mode == modeFilter:
		b.WriteString(sKey.Render("/") + m.filter + sKey.Render("▏") + "\n")
	case m.filter != "":
		b.WriteString(info + sMuted.Render("   filter: ") + m.filter + sMuted.Render(" (esc clears)") + "\n")
	default:
		b.WriteString(info + "\n")
	}
	b.WriteString("\n")

	var maxSize int64 = 1
	for _, e := range m.view {
		if e.size > maxSize {
			maxSize = e.size
		}
	}

	const barW, sizeW, metaW = 12, 10, 10
	libW := 0
	if m.multiLib && m.tab == tabGames {
		libW = 14
	}
	nameW := m.w - (4 + 2 + barW + 2 + sizeW + 2 + metaW + 1) - libW
	if nameW < 10 {
		nameW = 10
	}

	printed := 0
	switch {
	case m.tab == tabClean && m.scanning:
		b.WriteString(sMuted.Render("  Scanning Steam folders…") + "\n")
		printed++
	case len(m.view) == 0 && m.tab == tabClean && m.scanned:
		b.WriteString(sMuted.Render("  Nothing to clean up.") + "\n")
		printed++
	case len(m.view) == 0:
		b.WriteString(sMuted.Render("  No games match.") + "\n")
		printed++
	}
	end := m.offset + m.rows()
	if end > len(m.view) {
		end = len(m.view)
	}
	now := time.Now()
	for i := m.offset; i < end; i++ {
		e := m.view[i]
		on := m.selected[e.key]
		here := i == m.cursor

		mark := sFaint.Render("○")
		name := lipgloss.NewStyle().Foreground(cText).Render(fit(e.name, nameW))
		sz := sMuted.Render(fmt.Sprintf("%*s", sizeW, humanSize(e.size)))
		if on {
			mark = sDanger.Render("●")
			name = lipgloss.NewStyle().Foreground(cDanger).Render(fit(e.name, nameW))
			sz = sDanger.Render(fmt.Sprintf("%*s", sizeW, humanSize(e.size)))
		}
		pointer := "  "
		if here {
			pointer = sKey.Render("▌ ")
			name = lipgloss.NewStyle().Bold(true).Render(name)
		}
		var meta string
		if e.game != nil {
			style := sMuted
			if e.played <= 0 {
				style = sFaint
			}
			meta = style.Render(fmt.Sprintf("%*s", metaW, ago(e.played, now)))
		} else {
			style := sMuted
			if !e.safe {
				style = sWarn // may hold personal data or progress; skipped by "mark all"
			}
			meta = style.Render(fmt.Sprintf("%*s", metaW, e.tag))
		}
		line := pointer + mark + " " + name + "  " + bar(float64(e.size)/float64(maxSize), barW) + "  " + sz + "  " + meta
		if libW > 0 && e.game != nil {
			line += " " + sFaint.Render(fit(shortLib(e.game.Library), libW-1))
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
	if m.tab == tabClean && len(m.warnings) > 0 {
		status += "   " + sWarn.Render("⚠ "+m.warnings[0])
	}
	b.WriteString(ansi.Truncate(status, m.w, "…") + "\n")
	other := "cleanup"
	if m.tab == tabClean {
		other = "games"
	}
	keys := [][2]string{{"↑↓", "move"}, {"space", "mark"}, {"a", "all"}, {"/", "filter"}, {"s", "sort"}, {"r", "reverse"}, {"tab", other}}
	if m.tab == tabGames {
		keys = append(keys, [2]string{"t", "tools"})
	}
	keys = append(keys, [2]string{"enter", "delete"}, [2]string{"q", "quit"})
	b.WriteString(help(keys))
	return b.String()
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

func (m *model) summary(entries []entry) string {
	var games, items int
	for _, e := range entries {
		if e.game != nil {
			games++
		} else {
			items++
		}
	}
	var parts []string
	if games > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", games, plural(games)))
	}
	if items > 0 {
		parts = append(parts, fmt.Sprintf("%d cleanup %s", items, map[bool]string{true: "item", false: "items"}[items == 1]))
	}
	return strings.Join(parts, " and ")
}

func (m *model) viewConfirm() string {
	entries := m.selectedEntries()
	_, size := m.selectedSize()
	hasGame := false
	var b strings.Builder
	b.WriteString(sDanger.Render("Delete "+m.summary(entries)+"?") + "\n")
	b.WriteString(sMuted.Render("Frees ") + sOK.Render(humanSize(size)) + sMuted.Render(". This deletes the files; it cannot be undone.") + "\n\n")
	for i, e := range entries {
		if e.game != nil {
			hasGame = true
		}
		if i < 8 {
			b.WriteString("  " + fit(e.name, 40) + sMuted.Render(fmt.Sprintf("%10s", humanSize(e.size))) + "\n")
		}
	}
	if len(entries) > 8 {
		b.WriteString(sMuted.Render(fmt.Sprintf("  … and %d more\n", len(entries)-8)))
	}
	if hasGame {
		pfx := sFaint.Render("○") + sMuted.Render(" keep Proton prefix (saves)")
		if m.prefix {
			pfx = sDanger.Render("●") + " also delete Proton prefix and shader cache"
		}
		b.WriteString("\n" + pfx + "\n")
	}
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
	keys := [][2]string{{"y", "delete"}}
	if hasGame {
		keys = append(keys, [2]string{"p", "prefix"})
	}
	if m.opts.SteamRunning {
		keys = append(keys, [2]string{"c", "close Steam"})
	}
	keys = append(keys, [2]string{"esc", "cancel"})
	b.WriteString("\n" + help(keys))
	return m.box(b.String())
}

func (m *model) viewProgress() string {
	total := len(m.queue)
	var b strings.Builder
	if m.mode == modeDone {
		b.WriteString(sOK.Render("Done") + "\n\n")
		b.WriteString(fmt.Sprintf("Deleted %d of %d, freed %s.\n", total-len(m.failed), total, sOK.Render(humanSize(m.freed))))
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
	b.WriteString(sTitle.Render("Deleting…") + "\n\n")
	b.WriteString(fit(m.queue[m.qIdx].name, 50) + "\n\n")
	b.WriteString(bar(float64(m.qIdx)/float64(total), 40) + sMuted.Render(fmt.Sprintf("  %d/%d", m.qIdx, total)))
	return m.box(b.String())
}
