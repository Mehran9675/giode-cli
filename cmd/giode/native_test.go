package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
}
