package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ShatteredEagle/steamsweep/internal/clean"
	"github.com/ShatteredEagle/steamsweep/internal/steam"
)

func testModel() *model {
	games := []steam.Game{
		{AppID: "1", Name: "Big Game", Size: 40e9, Library: "/lib", LastPlayed: 1_700_000_000},
		{AppID: "2", Name: "Medium Game", Size: 5e9, Library: "/lib", LastPlayed: 0},
		{AppID: "3", Name: "Proton 9.0", Size: 1e9, Library: "/lib", Tool: true},
	}
	m := newModel(games, Options{Root: "/lib", Libs: []string{"/lib"}})
	m.w, m.h = 90, 20
	return m
}

func press(m *model, s string) {
	var k tea.KeyPressMsg
	switch s {
	case "space":
		k = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "down":
		k = tea.KeyPressMsg{Code: tea.KeyDown}
	case "enter":
		k = tea.KeyPressMsg{Code: tea.KeyEnter}
	case "tab":
		k = tea.KeyPressMsg{Code: tea.KeyTab}
	default:
		k = tea.KeyPressMsg{Code: rune(s[0]), Text: s}
	}
	m.Update(k)
}

func names(m *model) []string {
	var out []string
	for _, e := range m.view {
		out = append(out, e.name)
	}
	return out
}

func withCleanup(m *model) {
	press(m, "tab") // starts the scan; feed the result by hand instead of running it
	items := []clean.Item{
		{Key: "a", Name: "Old recording", Kind: clean.KindRecording, Size: 9e9, Paths: []string{"/lib/x"}},
		{Key: "b", Name: "Web cache", Kind: clean.KindCache, Size: 1e9, Safe: true, Paths: []string{"/lib/y"}},
		{Key: "c", Name: "Leftover", Kind: clean.KindLeftover, Size: 2e9, Safe: true, Paths: []string{"/lib/z"}},
	}
	m.Update(scannedMsg{clean.Result{Items: items, Warnings: []string{"Library not found"}}})
}

func TestToolsHiddenByDefault(t *testing.T) {
	m := testModel()
	if len(m.view) != 2 {
		t.Fatalf("view=%d, want tools hidden", len(m.view))
	}
	press(m, "t")
	if len(m.view) != 3 {
		t.Fatalf("view=%d after t", len(m.view))
	}
}

func TestSpaceMarksAndAdvances(t *testing.T) {
	m := testModel()
	press(m, "space")
	press(m, "space")
	n, size := m.selectedSize()
	if n != 2 || size != 45e9 {
		t.Fatalf("n=%d size=%d", n, size)
	}
}

func TestFilter(t *testing.T) {
	m := testModel()
	press(m, "/")
	for _, r := range "med" {
		press(m, string(r))
	}
	if got := names(m); len(got) != 1 || got[0] != "Medium Game" {
		t.Fatalf("filter failed: %v", got)
	}
}

func TestEnterNeedsSelection(t *testing.T) {
	m := testModel()
	press(m, "enter")
	if m.mode != modeList {
		t.Fatal("confirm opened with nothing selected")
	}
	press(m, "space")
	press(m, "enter")
	if m.mode != modeConfirm {
		t.Fatal("confirm not opened")
	}
}

func TestSortModes(t *testing.T) {
	m := testModel()
	if got := names(m); got[0] != "Big Game" {
		t.Fatalf("size sort: %v", got)
	}
	press(m, "s") // last played: never-played first
	if got := names(m); got[0] != "Medium Game" {
		t.Fatalf("played sort: %v", got)
	}
	press(m, "r")
	if got := names(m); got[0] != "Big Game" {
		t.Fatalf("reversed played sort: %v", got)
	}
	press(m, "s")
	press(m, "s") // name; reverse is still on from above
	if m.kind() != sortName || m.sortSummary() != "sort: name ↓" {
		t.Fatalf("summary %q", m.sortSummary())
	}
}

func TestCleanupTab(t *testing.T) {
	m := testModel()
	withCleanup(m)
	if !m.scanning && !m.scanned {
		t.Fatal("scan never started")
	}
	if got := names(m); len(got) != 3 || got[0] != "Old recording" {
		t.Fatalf("cleanup view: %v", got)
	}
	press(m, "a") // marks only safe items; the recording holds personal data
	n, size := m.selectedSize()
	if n != 2 || size != 3e9 {
		t.Fatalf("mark-all picked n=%d size=%d, want the 2 safe items", n, size)
	}
	press(m, "tab") // back to games; selection survives across tabs
	press(m, "space")
	if n, _ := m.selectedSize(); n != 3 {
		t.Fatalf("selection across tabs: %d", n)
	}
	press(m, "enter")
	if !strings.Contains(m.viewConfirm(), "1 game and 2 cleanup items") {
		t.Errorf("confirm summary wrong:\n%s", m.viewConfirm())
	}
}

func TestCleanupSortCyclesItsOwnModes(t *testing.T) {
	m := testModel()
	withCleanup(m)
	press(m, "s")
	if m.kind() != sortKindTag {
		t.Fatalf("kind=%v", m.kind())
	}
	press(m, "s")
	press(m, "s")
	if m.kind() != sortSize {
		t.Fatalf("cleanup cycle should wrap after 3 modes, kind=%v", m.kind())
	}
}

func TestCloseFailureReturnsToConfirm(t *testing.T) {
	m := testModel()
	m.opts.SteamRunning = true
	m.mode, m.closing = modeDeleting, true
	m.queue = m.tabs[tabGames][:1]
	m.Update(closedMsg{err: errString("still running")})
	if m.mode != modeConfirm || m.closeErr == "" {
		t.Fatalf("mode=%v closeErr=%q", m.mode, m.closeErr)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestAgo(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	cases := map[int64]string{0: "never", 2_000_000_000 - 3600: "today", 2_000_000_000 - 5*86400: "5d ago",
		2_000_000_000 - 90*86400: "3mo ago", 2_000_000_000 - 800*86400: "2y ago"}
	for in, want := range cases {
		if got := ago(in, now); got != want {
			t.Errorf("ago(%d)=%q want %q", in, got, want)
		}
	}
}

func TestViewsRender(t *testing.T) {
	m := testModel()
	press(m, "space")
	for _, want := range []string{"steamsweep", "Games", "Cleanup", "Big Game", "1 selected"} {
		if out := m.viewList(); !strings.Contains(out, want) {
			t.Errorf("games view missing %q", want)
		}
	}
	withCleanup(m)
	out := m.viewList()
	for _, want := range []string{"Old recording", "recording", "Library not found"} {
		if !strings.Contains(out, want) {
			t.Errorf("cleanup view missing %q", want)
		}
	}
	if testing.Verbose() {
		t.Log("\n" + out)
	}
}

func TestUpdateBanner(t *testing.T) {
	m := testModel()
	if strings.Contains(m.viewList(), "available") {
		t.Fatal("banner shown with no update")
	}
	press(m, "u")
	if m.updating {
		t.Fatal("u started an update with nothing available")
	}
	m.Update(updateAvailableMsg{tag: "v9.9.9"})
	out := m.viewList()
	if !strings.Contains(out, "v9.9.9 available") || !strings.Contains(out, "update") {
		t.Errorf("banner/help missing:\n%s", out)
	}
	m.Update(updatedMsg{tag: "v9.9.9"})
	if out := m.viewList(); !strings.Contains(out, "updated to v9.9.9") || strings.Contains(out, "available") {
		t.Errorf("post-update state wrong:\n%s", out)
	}
	m.updateTag = "v9.9.9"
	m.Update(updatedMsg{err: errString("permission denied")})
	if !strings.Contains(m.viewList(), "update failed: permission denied") {
		t.Error("failure not shown")
	}
}
