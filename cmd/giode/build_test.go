package main

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"github.com/mehran9675/giode-cli/internal/config"
)

func TestAddTag(t *testing.T) {
	for _, c := range []struct{ tags, tag, want string }{
		{"", "giode_gio", "giode_gio"},
		{"foo bar", "giode_gio", "foo,bar,giode_gio"},
		{"foo,giode_gio", "giode_gio", "foo,giode_gio"},
	} {
		if got := addTag(c.tags, c.tag); got != c.want {
			t.Errorf("addTag(%q, %q) = %q", c.tags, c.tag, got)
		}
	}
}

var placeholder = regexp.MustCompile(`\{\{[A-Z_]+\}\}`)

func TestWorkflowSetup(t *testing.T) {
	plain := workflow(&config.Config{})
	if placeholder.MatchString(plain) || strings.Contains(plain, "msys2") || !strings.Contains(plain, "giode build $win windows-installer msix") {
		t.Errorf("default workflow:\n%s", plain)
	}
	c := &config.Config{CI: config.CI{WindowsMSYS2: true, Setup: config.CISetup{
		MacOS: "brew install mpv", Linux: "sudo apt-get install -y libmpv-dev", Windows: "pacman -S --noconfirm mingw-w64-x86_64-mpv"}}}
	wf := workflow(c)
	for _, w := range []string{"brew install mpv", "libmpv-dev", "mingw-w64-x86_64-mpv", "runs-on: windows-latest", "needs: [linux, macos, windows]", "shell: msys2 {0}"} {
		if !strings.Contains(wf, w) {
			t.Errorf("lacks %q", w)
		}
	}
	if placeholder.MatchString(wf) || strings.Count(wf, "windows-installer msix") != 1 {
		t.Errorf("windows built on two runners, or a placeholder is left:\n%s", wf)
	}
	if py, err := exec.LookPath("python3"); err == nil {
		for _, s := range []string{plain, wf} {
			cmd := exec.Command(py, "-c", "import sys, yaml; yaml.safe_load(sys.stdin)")
			cmd.Stdin = strings.NewReader(s)
			if out, err := cmd.CombinedOutput(); err != nil && !strings.Contains(string(out), "No module named") {
				t.Errorf("not YAML: %v\n%s", err, out)
			}
		}
	}
}
