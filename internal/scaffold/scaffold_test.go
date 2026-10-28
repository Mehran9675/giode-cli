package scaffold

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaults(t *testing.T) {
	name, module, id := Defaults("/work/my-notes_app")
	if name != "My Notes App" || module != "my-notes-app" || id != "com.example.mynotesapp" {
		t.Errorf("Defaults = %q, %q, %q", name, module, id)
	}
	if _, _, id := Defaults("2048"); id != "com.example.app2048" {
		t.Errorf("an ID part can't start with a digit: %q", id)
	}
}

func TestCreate(t *testing.T) {
	for _, tmpl := range []string{"app"} {
		dir := filepath.Join(t.TempDir(), "proj")
		err := Create(Project{Dir: dir, Name: "My Notes", Module: "example.com/notes", AppID: "com.example.notes"})
		if err != nil {
			t.Fatalf("%s: %v", tmpl, err)
		}
		main, _ := os.ReadFile(filepath.Join(dir, "main.go"))
		if !strings.Contains(string(main), `"My Notes"`) || strings.Contains(string(main), "[[") {
			t.Errorf("%s: main.go not filled in:\n%s", tmpl, main)
		}
		toml, _ := os.ReadFile(filepath.Join(dir, "giode.toml"))
		if !strings.Contains(string(toml), `id = "com.example.notes"`) {
			t.Errorf("%s: giode.toml:\n%s", tmpl, toml)
		}
		if _, err := os.Stat(filepath.Join(dir, "assets", "README.md")); err != nil {
			t.Error("no assets/README.md")
		}
		f, err := os.Open(filepath.Join(dir, "assets", "icon.png"))
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := png.DecodeConfig(f)
		f.Close()
		if err != nil || cfg.Width != 1024 || cfg.Height != 1024 {
			t.Errorf("%s: icon %dx%d, %v", tmpl, cfg.Width, cfg.Height, err)
		}
	}
}

func TestCreate_RefusesNonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "x"), nil, 0o644)
	if err := Create(Project{Dir: dir, Name: "A", Module: "a", AppID: "com.example.a"}); err == nil {
		t.Error("wrote into a non-empty directory")
	}
	if _, err := os.Stat(filepath.Join(dir, "x")); err != nil {
		t.Error("removed a directory it didn't create")
	}
}

func TestCreate_Platforms(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "proj")
	err := Create(Project{Dir: dir, Name: "Notes", Module: "example.com/notes", AppID: "com.acme.notes",
		Platforms: []string{"android", "ios"}, Version: "1.2.0", Publisher: "Acme", Description: "Quick notes"})
	if err != nil {
		t.Fatal(err)
	}
	toml, _ := os.ReadFile(filepath.Join(dir, "giode.toml"))
	s := string(toml)
	for _, w := range []string{`platforms = ["android", "ios"]`, `version = "1.2.0"`, `publisher = "Acme"`, "[android]", "[ios]"} {
		if !strings.Contains(s, w) {
			t.Errorf("giode.toml lacks %s:\n%s", w, s)
		}
	}
	for _, w := range []string{"[windows]", "[macos]", "[web]", "[linux]", "TODO: your name"} {
		if strings.Contains(s, w) {
			t.Errorf("giode.toml has %s for a mobile-only app:\n%s", w, s)
		}
	}
	readme, _ := os.ReadFile(filepath.Join(dir, "README.md"))
	if !strings.Contains(string(readme), "for Android and iOS") || !strings.Contains(string(readme), "giode run ios-sim") || strings.Contains(string(readme), "windows-installer") {
		t.Errorf("README:\n%s", readme)
	}
}
