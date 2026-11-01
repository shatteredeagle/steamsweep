package steam

import (
	"fmt"
	"os/exec"
	"runtime"
	"time"
)

func waitGone(d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if !Running() {
			return true
		}
		time.Sleep(250 * time.Millisecond)
	}
	return !Running()
}

// Close shuts the Steam client down, escalating until it is gone:
// a polite shutdown request first, then SIGTERM / taskkill, then SIGKILL / taskkill /F.
func Close() error {
	if !Running() {
		return nil
	}
	type step struct {
		cmds [][]string
		wait time.Duration
	}
	var steps []step
	switch runtime.GOOS {
	case "windows":
		steps = []step{
			{[][]string{{"taskkill", "/IM", "steam.exe"}}, 6 * time.Second},
			{[][]string{{"taskkill", "/F", "/IM", "steam.exe"}, {"taskkill", "/F", "/IM", "steamwebhelper.exe"}}, 4 * time.Second},
		}
	case "darwin":
		steps = []step{
			{[][]string{{"pkill", "-x", "steam_osx"}}, 6 * time.Second},
			{[][]string{{"pkill", "-9", "-x", "steam_osx"}}, 4 * time.Second},
		}
	default:
		steps = []step{
			{[][]string{{"steam", "-shutdown"}}, 8 * time.Second},
			{[][]string{{"pkill", "-x", "steam"}}, 5 * time.Second},
			{[][]string{{"pkill", "-9", "-x", "steam"}, {"pkill", "-9", "-x", "steamwebhelper"}}, 3 * time.Second},
		}
	}
	for _, s := range steps {
		for _, c := range s.cmds {
			_ = exec.Command(c[0], c[1:]...).Run() // best effort; Running() decides success
		}
		if waitGone(s.wait) {
			return nil
		}
	}
	return fmt.Errorf("Steam is still running")
}
