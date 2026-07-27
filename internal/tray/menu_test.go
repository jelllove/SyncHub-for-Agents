package tray

import (
	"reflect"
	"testing"
)

func TestPauseTitle(t *testing.T) {
	if got := pauseTitle(false); got != "Pause" {
		t.Errorf("running -> %q, want Pause", got)
	}
	if got := pauseTitle(true); got != "Resume" {
		t.Errorf("paused -> %q, want Resume", got)
	}
}

func TestOpenCommand(t *testing.T) {
	cases := []struct {
		goos   string
		target string
		name   string
		args   []string
	}{
		{"windows", "http://x/", "rundll32", []string{"url.dll,FileProtocolHandler", "http://x/"}},
		{"darwin", "/logs", "open", []string{"/logs"}},
		{"linux", "/logs", "xdg-open", []string{"/logs"}},
	}
	for _, c := range cases {
		name, args := openCommand(c.goos, c.target)
		if name != c.name || !reflect.DeepEqual(args, c.args) {
			t.Errorf("openCommand(%q) = %q %v, want %q %v", c.goos, name, args, c.name, c.args)
		}
	}
}
