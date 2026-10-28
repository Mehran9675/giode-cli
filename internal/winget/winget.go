// Package winget writes the manifests that list an app in the Windows
// Package Manager (winget) repository: submit the directory it writes
// to github.com/microsoft/winget-pkgs, or use it with
// "winget install --manifest <dir>".
package winget

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// App describes the release.
type App struct {
	// ID is the package identifier, Publisher.App.
	ID string
	// Name, Version, Publisher, Description, License and Website
	// describe the app; ProductCode is its uninstall entry's key.
	Name, Version, Publisher, Description, License, Website, ProductCode string
	// Installer is the setup program; InstallerURL where users download
	// it ({version} and {arch} are replaced). PerMachine installs for every user.
	Installer, InstallerURL string
	PerMachine              bool
	// Arch is the GOARCH: amd64, arm64 or 386.
	Arch string
}

const manifestVersion = "1.6.0"

// Write writes the version, installer and locale manifests into
// dir/manifests/<first letter>/<publisher>/<app>/<version>, the
// repository's layout, and returns that directory.
func Write(a App, dir string) (string, error) {
	parts := strings.SplitN(a.ID, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("the winget ID %q must be Publisher.App", a.ID)
	}
	if a.InstallerURL == "" {
		return "", fmt.Errorf("winget manifests need windows.winget.installer_url: where users download the setup program")
	}
	sum, err := sha256File(a.Installer)
	if err != nil {
		return "", err
	}
	arch := map[string]string{"amd64": "x64", "arm64": "arm64", "386": "x86"}[a.Arch]
	if arch == "" {
		arch = "x64"
	}
	scope := "user"
	if a.PerMachine {
		scope = "machine"
	}
	url := strings.NewReplacer("{version}", a.Version, "{arch}", a.Arch).Replace(a.InstallerURL)
	out := filepath.Join(dir, "manifests", strings.ToLower(parts[0][:1]), parts[0], parts[1], a.Version)
	if err := os.MkdirAll(out, 0o755); err != nil {
		return "", err
	}
	q := func(s string) string { b, _ := json.Marshal(s); return string(b) }
	head := func(kind string) string {
		return fmt.Sprintf("# yaml-language-server: $schema=https://aka.ms/winget-manifest.%s.%s.schema.json\n\nPackageIdentifier: %s\nPackageVersion: %s\n", kind, manifestVersion, q(a.ID), q(a.Version))
	}
	tail := func(kind string) string {
		return fmt.Sprintf("ManifestType: %s\nManifestVersion: %s\n", kind, manifestVersion)
	}
	files := map[string]string{
		a.ID + ".yaml": head("version") + "DefaultLocale: en-US\n" + tail("version"),
		a.ID + ".installer.yaml": head("installer") +
			"InstallerType: nullsoft\nScope: " + scope + "\nInstallModes:\n- interactive\n- silent\n- silentWithProgress\nUpgradeBehavior: install\n" +
			"ProductCode: " + q(a.ProductCode) + "\nInstallers:\n- Architecture: " + arch + "\n  InstallerUrl: " + q(url) +
			"\n  InstallerSha256: " + strings.ToUpper(sum) + "\n" + tail("installer"),
	}
	locale := head("defaultLocale") + "PackageLocale: en-US\nPublisher: " + q(a.Publisher) + "\nPackageName: " + q(a.Name) + "\n"
	if a.Website != "" {
		locale += "PackageUrl: " + q(a.Website) + "\n"
	}
	license := a.License
	if license == "" {
		license = "Proprietary"
	}
	desc := a.Description
	if desc == "" {
		desc = a.Name
	}
	locale += "License: " + q(license) + "\nShortDescription: " + q(desc) + "\n" + tail("defaultLocale")
	files[a.ID+".locale.en-US.yaml"] = locale
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(out, name), []byte(body), 0o644); err != nil {
			return "", err
		}
	}
	return out, nil
}

func sha256File(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
