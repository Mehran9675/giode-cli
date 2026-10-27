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
	for tmpl := range Templates {
		dir := filepath.Join(t.TempDir(), "proj")
		err := Create(Project{Dir: dir, Name: "My Notes", Module: "example.com/notes", AppID: "com.example.notes", Template: tmpl})
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
	if err := Create(Project{Dir: dir, Name: "A", Module: "a", AppID: "com.example.a", Template: "minimal"}); err == nil {
		t.Error("wrote into a non-empty directory")
	}
	if _, err := os.Stat(filepath.Join(dir, "x")); err != nil {
		t.Error("removed a directory it didn't create")
	}
}
