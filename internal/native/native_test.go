package native

import (
	"debug/pe"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func run(t *testing.T, dir string, env []string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

// TestCollectLinux builds a cgo program linking libfoo, which links
// libbar, and finds both - and not the C library.
func TestCollectLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("builds Linux libraries")
	}
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("no gcc")
	}
	lib, src := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(lib, "bar.c"), []byte("int bar(void) { return 2; }\n"), 0o644)
	os.WriteFile(filepath.Join(lib, "foo.c"), []byte("int bar(void);\nint foo(void) { return bar() + 1; }\n"), 0o644)
	run(t, lib, nil, "gcc", "-shared", "-fPIC", "-o", "libbar.so.1", "-Wl,-soname,libbar.so.1", "bar.c")
	run(t, lib, nil, "ln", "-s", "libbar.so.1", "libbar.so")
	run(t, lib, nil, "gcc", "-shared", "-fPIC", "-o", "libfoo.so.2", "-Wl,-soname,libfoo.so.2", "foo.c", "-L.", "-lbar")
	run(t, lib, nil, "ln", "-s", "libfoo.so.2", "libfoo.so")
	os.WriteFile(filepath.Join(src, "go.mod"), []byte("module example.com/p\n\ngo 1.24\n"), 0o644)
	os.WriteFile(filepath.Join(src, "main.go"), []byte("package main\n\n// #cgo LDFLAGS: -L"+lib+" -Wl,-rpath-link,"+lib+" -lfoo\n// int foo(void);\nimport \"C\"\n\nfunc main() { println(C.foo()) }\n"), 0o644)
	run(t, src, []string{"CGO_ENABLED=1"}, "go", "build", "-o", "app", ".")

	r, err := Collect(filepath.Join(src, "app"), Options{GOOS: "linux", Search: []string{lib}})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, l := range r.Libs {
		names = append(names, l.Name)
		if filepath.Dir(l.Path) != lib {
			t.Errorf("%s found at %s", l.Name, l.Path)
		}
	}
	if !reflect.DeepEqual(names, []string{"libfoo.so.2", "libbar.so.1"}) || len(r.Missing) != 0 {
		t.Errorf("libs %v, missing %v", names, r.Missing)
	}

	// Without the directory, they're missing, not silently dropped.
	r, _ = Collect(filepath.Join(src, "app"), Options{GOOS: "linux"})
	if len(r.Libs) != 0 || !reflect.DeepEqual(r.Missing, []string{"libfoo.so.2"}) {
		t.Errorf("unsearched: %v %v", r.Libs, r.Missing)
	}
	// Excluded by name.
	r, _ = Collect(filepath.Join(src, "app"), Options{GOOS: "linux", Search: []string{lib}, Exclude: []string{"libbar*"}})
	if len(r.Libs) != 1 {
		t.Errorf("exclude: %v", r.Libs)
	}
}

// TestCollectSystemOnly checks Go programs for Windows and macOS, which
// import only system libraries, need nothing bundled.
func TestCollectSystemOnly(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "go.mod"), []byte("module example.com/p\n\ngo 1.24\n"), 0o644)
	os.WriteFile(filepath.Join(src, "main.go"), []byte("package main\n\nimport \"os\"\n\nfunc main() { os.Exit(0) }\n"), 0o644)
	for _, goos := range []string{"windows", "darwin"} {
		run(t, src, []string{"GOOS=" + goos, "GOARCH=amd64", "CGO_ENABLED=0"}, "go", "build", "-o", "app-"+goos, ".")
		r, err := Collect(filepath.Join(src, "app-"+goos), Options{GOOS: goos})
		if err != nil {
			t.Fatal(goos, err)
		}
		if len(r.Libs) != 0 || len(r.Missing) != 0 {
			t.Errorf("%s: libs %v missing %v", goos, r.Libs, r.Missing)
		}
	}
}

func TestLookupFoldsCaseOnWindows(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "LIBMPV-2.DLL"), nil, 0o644)
	// On a case-insensitive file system (macOS, Windows) the name asked
	// for finds the file as it's spelled.
	if p := lookup(dir, "libmpv-2.dll", true); !strings.EqualFold(filepath.Base(p), "LIBMPV-2.DLL") {
		t.Errorf("windows lookup: %q", p)
	}
	// On a case-insensitive file system (macOS, Windows) the plain
	// lookup finds it too.
	if _, err := os.Stat(filepath.Join(dir, "libmpv-2.dll")); err != nil {
		if p := lookup(dir, "libmpv-2.dll", false); p != "" {
			t.Errorf("case-sensitive lookup found %q", p)
		}
	}
	c := &collector{o: Options{GOOS: "windows"}}
	if !c.system("KERNEL32.dll", "KERNEL32.dll") || !c.system("api-ms-win-crt-runtime-l1-1-0.dll", "api-ms-win-crt-runtime-l1-1-0.dll") || c.system("libmpv-2.dll", "libmpv-2.dll") {
		t.Error("windows system DLLs")
	}
}

func TestPlanMac(t *testing.T) {
	libs := []Lib{
		{Name: "libmpv.2.dylib", Path: "/opt/homebrew/lib/libmpv.2.dylib", Refs: []string{"/opt/homebrew/opt/mpv/lib/libmpv.2.dylib"}},
		{Name: "libavcodec.61.dylib", Path: "/opt/homebrew/lib/libavcodec.61.dylib", Refs: []string{"/opt/homebrew/opt/ffmpeg/lib/libavcodec.61.dylib", "@rpath/libavcodec.61.dylib"}},
	}
	p := PlanMac(libs)
	if p.Copies["/opt/homebrew/lib/libmpv.2.dylib"] != "libmpv.2.dylib" {
		t.Error(p.Copies)
	}
	args := p.InstallNameArgs("libmpv.2.dylib", "Frameworks/libmpv.2.dylib")
	want := []string{"-id", "@rpath/libmpv.2.dylib",
		"-change", "/opt/homebrew/opt/ffmpeg/lib/libavcodec.61.dylib", "@rpath/libavcodec.61.dylib",
		"-change", "/opt/homebrew/opt/mpv/lib/libmpv.2.dylib", "@rpath/libmpv.2.dylib",
		"-change", "@rpath/libavcodec.61.dylib", "@rpath/libavcodec.61.dylib",
		"Frameworks/libmpv.2.dylib"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("args\n%q\nwant\n%q", args, want)
	}
}

// TestCollectWindowsDLLs reads real Windows DLLs (Wine's) whose imports
// include ones Windows doesn't have.
func TestCollectWindowsDLLs(t *testing.T) {
	dir := "/usr/lib/x86_64-linux-gnu/wine/x86_64-windows"
	bin := filepath.Join(dir, "d3dx9_43.dll")
	if _, err := os.Stat(bin); err != nil {
		t.Skip("no Wine DLLs to read")
	}
	f, err := pe.Open(bin)
	if err != nil {
		t.Fatal(err)
	}
	imports := peImports(f)
	f.Close()
	if !containsFold(imports, "d3dxof.dll") || !containsFold(imports, "kernel32.dll") {
		t.Fatalf("imports %v", imports)
	}
	r, err := Collect(bin, Options{GOOS: "windows", Search: []string{dir}})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, l := range r.Libs {
		names = append(names, strings.ToLower(l.Name))
	}
	if !containsFold(names, "d3dxof.dll") {
		t.Errorf("bundled %v, want d3dxof.dll", names)
	}
	if containsFold(names, "kernel32.dll") {
		t.Errorf("bundled a system DLL: %v", names)
	}
	// Alone in a folder, without the search directory, the DLL is
	// reported missing (Windows also looks next to the program).
	alone := filepath.Join(t.TempDir(), "d3dx9_43.dll")
	b, _ := os.ReadFile(bin)
	os.WriteFile(alone, b, 0o644)
	r, _ = Collect(alone, Options{GOOS: "windows"})
	if !containsFold(r.Missing, "d3dxof.dll") {
		t.Errorf("missing %v", r.Missing)
	}
}

// TestCollectWindowsRebuild doesn't take the DLLs beside the program -
// the output folder, holding the last build's copies - over the search
// folders'.
func TestCollectWindowsRebuild(t *testing.T) {
	dir := "/usr/lib/x86_64-linux-gnu/wine/x86_64-windows"
	b, err := os.ReadFile(filepath.Join(dir, "d3dx9_43.dll"))
	if err != nil {
		t.Skip("no Wine DLLs to read")
	}
	out := t.TempDir()
	exe := filepath.Join(out, "app.exe")
	os.WriteFile(exe, b, 0o755)
	os.WriteFile(filepath.Join(out, "d3dxof.dll"), []byte("stale"), 0o644)
	r, err := Collect(exe, Options{GOOS: "windows", Search: []string{dir}})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range r.Libs {
		if strings.EqualFold(l.Name, "d3dxof.dll") && filepath.Dir(l.Path) != dir {
			t.Errorf("d3dxof.dll from %s, want the search folder", l.Path)
		}
	}
}

func containsFold(s []string, v string) bool {
	for _, x := range s {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}
