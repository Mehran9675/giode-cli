package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mehran9675/giode-cli/internal/config"
)

func TestParse_FlagsAfterArgs(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	rel := fs.Bool("release", false, "")
	pos, err := parse(fs, []string{"android", "-release", "windows", "--", "-x"})
	if err != nil || !*rel || strings.Join(pos, " ") != "android windows -x" {
		t.Errorf("pos %v, release %v, err %v", pos, *rel, err)
	}
}

func TestGoVersion(t *testing.T) {
	for v, want := range map[string][2]int{"go1.26.6": {1, 26}, "go1.27rc1": {1, 27}, "devel": {0, 0}} {
		if maj, min := goVersion(v); maj != want[0] || min != want[1] {
			t.Errorf("goVersion(%q) = %d.%d", v, maj, min)
		}
	}
}

func TestBinaryName(t *testing.T) {
	c := &config.Config{App: config.App{Name: "My Notes: Pro"}}
	if got := binaryName(c); got != "MyNotesPro" {
		t.Errorf("binaryName = %q", got)
	}
}

func TestSetKeystore(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "giode.toml")
	os.WriteFile(p, []byte("[app]\nname = \"A\"\n\n[android]\nmin_sdk = 21\n"), 0o644)
	if err := setKeystore(p, "release.keystore"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "[android]\nkeystore = \"release.keystore\"\nmin_sdk = 21") {
		t.Errorf("giode.toml:\n%s", b)
	}
	os.WriteFile(p, []byte("[app]\nname = \"A\"\n"), 0o644)
	setKeystore(p, "k.keystore")
	b, _ = os.ReadFile(p)
	if !strings.HasSuffix(string(b), "\n\n[android]\nkeystore = \"k.keystore\"\n") {
		t.Errorf("without [android]:\n%s", b)
	}
}

func TestGogioOptions_Windows(t *testing.T) {
	dir := t.TempDir()
	c := &config.Config{Dir: dir, App: config.App{Name: "My App", ID: "com.example.app", Version: "1.2.3", Build: 7, Output: "dist"}}
	g, err := gogioOptions(c, "windows", buildOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if g.Target != "windows" || g.Version != "1.2.3.7" || g.Arch != "amd64" || g.Output != filepath.Join(dir, "dist", "windows", "MyApp.exe") {
		t.Errorf("options %+v", g)
	}
}
