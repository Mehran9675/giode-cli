package giopatch

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestPrepare patches the Gio version giode builds with and checks the
// build resolves gioui.org to the patched copy. It needs the module in
// the module cache or a reachable proxy.
func TestPrepare(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go command")
	}
	app := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("GOFLAGS", "-mod=mod")
	write := func(name, s string) {
		if err := os.WriteFile(filepath.Join(app, name), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/app\n\ngo 1.24\n\nrequire gioui.org v0.10.2\n")
	write("main.go", "package main\n\nimport _ \"gioui.org/app\"\n\nfunc main() {}\n")
	if out, err := exec.Command("sh", "-c", "cd "+app+" && go mod tidy").CombinedOutput(); err != nil {
		t.Skipf("can't fetch Gio: %v\n%s", err, out)
	}
	t.Setenv("GOFLAGS", "")

	modfile, cleanup, err := Prepare(app, Android)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	var dir string
	err = WithModfile(modfile, func() error {
		cmd := exec.Command("go", "list", "-f", "{{.Dir}}", "gioui.org/app")
		cmd.Dir = app
		out, err := cmd.Output()
		dir = strings.TrimSpace(string(out))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dir, filepath.Join("giode", "gio")) {
		t.Fatalf("gioui.org/app resolved to %s, not the patched copy", dir)
	}
	src, err := os.ReadFile(filepath.Join(dir, "os_android.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "giode: ask for the next frame") {
		t.Error("os_android.go isn't patched")
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "os_android.go", src, 0); err != nil {
		t.Errorf("patched os_android.go doesn't parse: %v", err)
	}
	if os.Getenv("GOFLAGS") != "" {
		t.Error("GOFLAGS not restored")
	}

	// A second build reuses the copy.
	m2, c2, err := Prepare(app, Android)
	if err != nil {
		t.Fatal(err)
	}
	c2()
	_ = m2
}

func TestApply_MismatchSkips(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "app"), 0o755)
	os.WriteFile(filepath.Join(dir, "app", "os_android.go"), []byte("package app\n"), 0o644)
	err := apply(dir, Android)
	if err == nil || !strings.Contains(err.Error(), ErrSkipped.Error()) {
		t.Errorf("apply to other code: %v, want ErrSkipped", err)
	}
}
