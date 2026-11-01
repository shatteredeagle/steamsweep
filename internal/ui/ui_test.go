package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"steamsweep/internal/steam"
)

func testModel() *model {
	games := []steam.Game{
		{AppID: "1", Name: "Big Game", Size: 40e9, Library: "/lib", LastPlayed: 1_700_000_000},
		{AppID: "2", Name: "Medium Game", Size: 5e9, Library: "/lib", LastPlayed: 0},
		{AppID: "3", Name: "Proton 9.0", Size: 1e9, Library: "/lib", Tool: true},
	}
	m := &model{all: games, selected: map[string]bool{}, w: 90, h: 20}
	m.rebuild()
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
	default:
		k = tea.KeyPressMsg{Code: rune(s[0]), Text: s}
	}
	m.Update(k)
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
	if m.cursor != 1 {
		t.Errorf("cursor=%d (clamped at end)", m.cursor)
	}
}

func TestFilter(t *testing.T) {
	m := testModel()
	press(m, "/")
	for _, r := range "med" {
		press(m, string(r))
	}
	if len(m.view) != 1 || m.all[m.view[0]].Name != "Medium Game" {
		t.Fatalf("filter failed: %v", m.view)
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

func TestViewRenders(t *testing.T) {
	m := testModel()
	press(m, "space")
	out := m.viewList()
	for _, want := range []string{"steamsweep", "Big Game", "1 selected"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
	if testing.Verbose() {
		t.Log("\n" + out)
		press(m, "enter")
		t.Log("\n" + m.viewConfirm())
	}
}

func TestCloseFailureReturnsToConfirm(t *testing.T) {
	m := testModel()
	m.opts.SteamRunning = true
	m.mode, m.closing = modeDeleting, true
	m.queue = m.all[:1]
	m.Update(closedMsg{err: errTest})
	if m.mode != modeConfirm || m.closeErr == "" {
		t.Fatalf("mode=%v closeErr=%q", m.mode, m.closeErr)
	}
}

var errTest = errString("still running")

type errString string

func (e errString) Error() string { return string(e) }

func names(m *model) []string {
	var out []string
	for _, i := range m.view {
		out = append(out, m.all[i].Name)
	}
	return out
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
	if m.sortMode != sortName || m.sortSummary() != "sort: name ↓" {
		t.Fatalf("summary %q", m.sortSummary())
	}
}

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
