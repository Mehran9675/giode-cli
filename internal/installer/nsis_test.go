package installer

import (
	"encoding/binary"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScript(t *testing.T) {
	o := Options{Name: `My "App"`, ID: "com.example.app", Version: "1.2.3", Publisher: "Acme $ Co",
		Exe: `C:\b\MyApp.exe`, Output: `C:\b\setup.exe`, DesktopShortcut: true}
	s, err := Script(o)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`!define APPNAME "My $\"App$\""`,
		`RequestExecutionLevel user`,
		`InstallDir "$LOCALAPPDATA\Programs\${APPNAME}"`,
		`"Publisher" "Acme $$ Co"`,
		`CreateShortCut "$DESKTOP\${APPNAME}.lnk"`,
		`VIProductVersion "1.2.3.0"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("script lacks %s", want)
		}
	}
	if strings.Contains(s, "MUI_PAGE_LICENSE") {
		t.Error("license page without a license")
	}
	o.PerMachine, o.License = true, "license.txt"
	s, _ = Script(o)
	for _, want := range []string{"RequestExecutionLevel admin", `$PROGRAMFILES64`, "!define REGROOT HKLM", "MUI_PAGE_LICENSE"} {
		if !strings.Contains(s, want) {
			t.Errorf("per-machine script lacks %s", want)
		}
	}
}

func TestWriteICO(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "icon.png")
	f, _ := os.Create(src)
	png.Encode(f, image.NewNRGBA(image.Rect(0, 0, 512, 512)))
	f.Close()
	dst := filepath.Join(dir, "app.ico")
	if err := writeICO(src, dst); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(dst)
	if len(b) < 6 || binary.LittleEndian.Uint16(b[2:]) != 1 || binary.LittleEndian.Uint16(b[4:]) != 7 {
		t.Fatalf("bad ICO header % x", b[:6])
	}
	// The last entry's offset and size reach the end of the file.
	e := b[6+16*6:]
	size, off := binary.LittleEndian.Uint32(e[8:]), binary.LittleEndian.Uint32(e[12:])
	if int(size+off) != len(b) || e[0] != 0 {
		t.Errorf("256px entry: size %d offset %d width byte %d, file %d", size, off, e[0], len(b))
	}
}

func TestBuild(t *testing.T) {
	if _, err := findMakensis(); err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "App.exe")
	os.WriteFile(exe, []byte("MZ"), 0o755)
	out := filepath.Join(dir, "out", "App-setup.exe")
	if err := Build(Options{Name: "App", ID: "com.example.app", Version: "1.0.0", Publisher: "Acme", Exe: exe, Output: out}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil || len(b) < 2 || string(b[:2]) != "MZ" {
		t.Errorf("setup program not written: %v", err)
	}
}

func TestScript_ContextMenu(t *testing.T) {
	o := Options{Name: "App", ID: "com.example.app", Version: "1.0.0", Exe: `C:\b\App.exe`, Output: `C:\b\setup.exe`,
		ContextMenu: []Verb{{Key: `*\shell\com-example-app.ask`, Label: "Ask App", ID: "ask", Arg: "%1"}}}
	s, err := Script(o)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`WriteRegStr ${REGROOT} "Software\Classes\*\shell\com-example-app.ask" "MUIVerb" "Ask App"`,
		`WriteRegStr ${REGROOT} "Software\Classes\*\shell\com-example-app.ask\command" "" '"$INSTDIR\${EXE}" --giode-action=ask "%1"'`,
		`DeleteRegKey ${REGROOT} "Software\Classes\*\shell\com-example-app.ask"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("script lacks %s", want)
		}
	}
}

// TestBuild_ContextMenuUnderWine installs and uninstalls a setup
// program with a context menu entry under Wine, checking the registry.
func TestBuild_ContextMenuUnderWine(t *testing.T) {
	if _, err := findMakensis(); err != nil {
		t.Skip(err)
	}
	wine, err := exec.LookPath("wine")
	if err != nil {
		t.Skip("no wine")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "CtxApp.exe")
	os.WriteFile(exe, []byte("MZ"), 0o755)
	out := filepath.Join(dir, "setup.exe")
	key := `HKCU\Software\Classes\*\shell\com-example-ctxapp.ask`
	o := Options{Name: "CtxApp", ID: "com.example.ctxapp", Version: "1.0.0", Publisher: "Acme", Exe: exe, Output: out,
		ContextMenu: []Verb{{Key: `*\shell\com-example-ctxapp.ask`, Label: "Ask CtxApp", ID: "ask", Arg: "%1"}}}
	if err := Build(o); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) string {
		cmd := exec.Command(wine, args...)
		cmd.Env = append(os.Environ(), "WINEDEBUG=-all")
		b, _ := cmd.CombinedOutput()
		return string(b)
	}
	run(out, "/S")
	defer run("reg", "delete", `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\com.example.ctxapp`, "/f")
	if got := run("reg", "query", key+`\command`); !strings.Contains(got, `--giode-action=ask "%1"`) {
		t.Fatalf("command after install:\n%s", got)
	}
	if got := run("reg", "query", key, "/v", "MUIVerb"); !strings.Contains(got, "Ask CtxApp") {
		t.Errorf("label after install:\n%s", got)
	}
	// Uninstall: the uninstaller re-runs itself from a temp copy, so
	// wait for the key to go.
	uninst := strings.TrimSpace(run("cmd", "/c", `echo %LOCALAPPDATA%`)) + `\Programs\CtxApp\Uninstall.exe`
	run(uninst, "/S")
	for i := 0; i < 50; i++ {
		if got := run("reg", "query", key); !strings.Contains(got, "MUIVerb") {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Error("context menu key left after uninstalling")
}
