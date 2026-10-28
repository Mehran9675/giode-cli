package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestFindANGLE finds libEGL.dll and libGLESv2.dll for the program's
// CPU: x64 DLLs (Wine's, renamed) serve an amd64 program, not an arm64
// one.
func TestFindANGLE(t *testing.T) {
	src := "/usr/lib/x86_64-linux-gnu/wine/x86_64-windows/opengl32.dll"
	b, err := os.ReadFile(src)
	if err != nil {
		t.Skip("no x64 Windows DLL to stand in for ANGLE")
	}
	angle := t.TempDir()
	os.WriteFile(filepath.Join(angle, "libEGL.dll"), b, 0o644)
	os.WriteFile(filepath.Join(angle, "libGLESv2.dll"), b, 0o644)

	prog := t.TempDir()
	os.WriteFile(filepath.Join(prog, "go.mod"), []byte("module example.com/p\n\ngo 1.24\n"), 0o644)
	os.WriteFile(filepath.Join(prog, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644)
	build := func(arch string) string {
		out := filepath.Join(prog, arch+".exe")
		cmd := exec.Command("go", "build", "-o", out, ".")
		cmd.Dir = prog
		cmd.Env = append(os.Environ(), "GOOS=windows", "GOARCH="+arch, "CGO_ENABLED=0")
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, b)
		}
		return out
	}
	t.Setenv("MSYSTEM_PREFIX", "")
	t.Setenv("LOCALAPPDATA", "")
	found, err := findANGLE(build("amd64"), []string{t.TempDir(), angle})
	if err != nil || len(found) != 2 || filepath.Dir(found[0]) != angle {
		t.Fatalf("amd64: %v %v", found, err)
	}
	if _, err := findANGLE(build("arm64"), []string{angle}); err == nil || !strings.Contains(err.Error(), "angleproject") {
		t.Errorf("arm64 took x64 ANGLE: %v", err)
	}

	// VS Code keeps ANGLE in a folder named after its build; after an
	// update there are two, and the newer one is taken.
	local := t.TempDir()
	code := filepath.Join(local, "Programs", "Microsoft VS Code")
	for i, build := range []string{"0f0d87fa9e", "6d62b1d5da"} {
		d := filepath.Join(code, build)
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "libEGL.dll"), b, 0o644)
		os.WriteFile(filepath.Join(d, "libGLESv2.dll"), b, 0o644)
		when := time.Now().Add(time.Duration(i-2) * time.Hour)
		os.Chtimes(d, when, when)
	}
	t.Setenv("LOCALAPPDATA", local)
	found, err = findANGLE(build("amd64"), nil)
	if err != nil || len(found) != 2 || filepath.Base(filepath.Dir(found[0])) != "6d62b1d5da" {
		t.Errorf("VS Code: %v %v", found, err)
	}
}

// TestCopyLibOntoItself leaves a library alone when it's copied onto
// itself, as a rebuild once did: truncating the destination emptied it.
func TestCopyLibOntoItself(t *testing.T) {
	p := filepath.Join(t.TempDir(), "libmpv-2.dll")
	os.WriteFile(p, []byte("not empty"), 0o644)
	if err := copyLib(p, p); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "not empty" {
		t.Errorf("copied onto itself: %q", b)
	}
}
