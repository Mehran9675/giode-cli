package winget

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWrite(t *testing.T) {
	dir := t.TempDir()
	setup := filepath.Join(dir, "setup.exe")
	os.WriteFile(setup, []byte("MZ setup"), 0o644)
	out, err := Write(App{ID: "Example.PocketAgent", Name: "Pocket Agent", Version: "1.4.0", Publisher: "Example Ltd",
		Description: `An "agent"`, License: "MIT", ProductCode: "com.example.agent",
		Installer: setup, InstallerURL: "https://example.com/dl/PocketAgent-{version}-setup.exe", Arch: "amd64"}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "manifests", "e", "Example", "PocketAgent", "1.4.0"); out != want {
		t.Errorf("dir %s, want %s", out, want)
	}
	inst, _ := os.ReadFile(filepath.Join(out, "Example.PocketAgent.installer.yaml"))
	for _, w := range []string{"InstallerType: nullsoft", "Scope: user", `InstallerUrl: "https://example.com/dl/PocketAgent-1.4.0-setup.exe"`,
		"InstallerSha256: 57", "Architecture: x64", `ProductCode: "com.example.agent"`, "ManifestType: installer"} {
		if !strings.Contains(string(inst), w) {
			t.Errorf("installer manifest lacks %s:\n%s", w, inst)
		}
	}
	loc, _ := os.ReadFile(filepath.Join(out, "Example.PocketAgent.locale.en-US.yaml"))
	if !strings.Contains(string(loc), `ShortDescription: "An \"agent\""`) || !strings.Contains(string(loc), `License: "MIT"`) {
		t.Errorf("locale manifest:\n%s", loc)
	}
	if _, err := Write(App{ID: "NoDot", InstallerURL: "x", Installer: setup}, dir); err == nil {
		t.Error("bad ID accepted")
	}
}
