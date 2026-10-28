package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mehran9675/giode-cli/internal/config"
	"github.com/mehran9675/giode-cli/internal/linuxpkg"
	"github.com/mehran9675/giode-cli/internal/msix"
	"github.com/mehran9675/giode-cli/internal/winget"
)

// linuxApp describes the built Linux binary bin for packaging.
func linuxApp(c *config.Config, bin, arch string) linuxpkg.App {
	if arch == "" {
		arch = runtime.GOARCH
	}
	return linuxpkg.App{
		Name: c.App.Name, ID: c.App.ID, Version: c.App.Version, Build: c.App.Build,
		Summary: c.App.Description, Description: c.Linux.Description,
		Publisher: c.App.Publisher, Maintainer: c.Linux.Maintainer,
		Website: c.App.Website, License: c.Linux.License, Categories: c.Linux.Categories,
		Binary: bin, Command: filepath.Base(bin), Icon: c.Path(c.App.Icon), Arch: arch,
		FlatpakPermissions: c.Linux.FlatpakPermissions,
	}
}

// buildLinuxPackage builds the Linux executable and packages it as a
// .deb, .rpm, AppImage or Flatpak.
func buildLinuxPackage(c *config.Config, kind string, o buildOpts) (string, error) {
	binOpts := o
	binOpts.output = ""
	bin, err := buildLinux(c, binOpts)
	if err != nil {
		return "", err
	}
	a := linuxApp(c, bin, o.arch)
	dir := filepath.Dir(bin)
	name := strings.ToLower(binaryName(c))
	var out string
	switch kind {
	case "deb":
		out = filepath.Join(dir, fmt.Sprintf("%s_%s-%d_%s.deb", name, c.App.Version, max(c.App.Build, 1), debArch(a.Arch)))
	case "rpm":
		out = filepath.Join(dir, fmt.Sprintf("%s-%s-%d.%s.rpm", name, c.App.Version, max(c.App.Build, 1), linuxpkg.RPMArch(a.Arch)))
	case "appimage":
		out = filepath.Join(dir, fmt.Sprintf("%s-%s-%s.AppImage", binaryName(c), c.App.Version, a.Arch))
	case "flatpak":
		out = filepath.Join(dir, fmt.Sprintf("%s-%s.flatpak", binaryName(c), c.App.Version))
	}
	if o.output != "" {
		out, _ = filepath.Abs(o.output)
	}
	switch kind {
	case "deb":
		err = linuxpkg.Deb(a, out)
	case "rpm":
		err = linuxpkg.RPM(a, out)
	case "appimage":
		work, werr := os.MkdirTemp("", "giode-appimage-")
		if werr != nil {
			return "", werr
		}
		defer os.RemoveAll(work)
		err = linuxpkg.AppImage(a, work, out, verbose)
	case "flatpak":
		manifestDir := filepath.Join(dir, "flatpak")
		built, ferr := linuxpkg.Flatpak(a, manifestDir, out, verbose)
		if ferr != nil {
			return "", ferr
		}
		if !built {
			fmt.Fprintf(os.Stderr, "  note: flatpak-builder isn't installed; wrote the manifest to %s (flatpak-builder --user --install-deps-from=flathub build %s.json)\n", manifestDir, c.App.ID)
			return filepath.Join(manifestDir, c.App.ID+".json"), nil
		}
	}
	if err != nil {
		return "", err
	}
	return out, nil
}

func debArch(goarch string) string {
	switch goarch {
	case "386":
		return "i386"
	case "arm":
		return "armhf"
	}
	return goarch
}

// buildMSIX builds the Windows executable and packages it as MSIX,
// signed with windows.signing when building for release.
func buildMSIX(c *config.Config, o buildOpts) (string, error) {
	exeOpts := o
	exeOpts.output = ""
	exeOpts.release = false // the package is signed, not the program inside
	exe, err := buildTarget(c, "windows", exeOpts)
	if err != nil {
		return "", err
	}
	arch := o.arch
	if arch == "" {
		arch = c.Windows.Arch
	}
	if arch == "" {
		arch = "amd64"
	}
	out := filepath.Join(filepath.Dir(exe), fmt.Sprintf("%s-%s-%s.msix", binaryName(c), c.App.Version, arch))
	if o.output != "" {
		out, _ = filepath.Abs(o.output)
	}
	m := c.Windows.MSIX
	name := m.IdentityName
	if name == "" {
		name = c.App.ID
	}
	display := m.PublisherDisplayName
	if display == "" {
		display = c.App.Publisher
	}
	var protocols []string
	protocols = append(protocols, c.App.Schemes...)
	if err := msix.Write(msix.App{
		Name: c.App.Name, IdentityName: name, Publisher: m.Publisher, PublisherDisplayName: display,
		Description: c.App.Description, Version: c.App.Version, Exe: exe, Icon: c.Path(c.App.Icon),
		Arch: arch, Protocols: protocols,
	}, out); err != nil {
		return "", err
	}
	if o.release {
		if err := signWindows(c, out); err != nil {
			return "", err
		}
	} else {
		fmt.Fprintln(os.Stderr, "  note: the package is unsigned; Windows installs only signed MSIX packages (use -release with windows.signing)")
	}
	return out, nil
}

// buildWinget builds the setup program and writes winget manifests for
// it.
func buildWinget(c *config.Config, o buildOpts) (string, error) {
	setupOpts := o
	setupOpts.output = ""
	setup, err := buildWindowsInstaller(c, setupOpts)
	if err != nil {
		return "", err
	}
	w := c.Windows.Winget
	id := w.ID
	if id == "" {
		id = wingetName(c.App.Publisher) + "." + wingetName(c.App.Name)
	}
	arch := o.arch
	if arch == "" {
		arch = c.Windows.Arch
	}
	dir := filepath.Join(filepath.Dir(setup), "winget")
	if o.output != "" {
		dir, _ = filepath.Abs(o.output)
	}
	return winget.Write(winget.App{
		ID: id, Name: c.App.Name, Version: c.App.Version, Publisher: c.App.Publisher,
		Description: c.App.Description, License: w.License, Website: c.App.Website, ProductCode: c.App.ID,
		Installer: setup, InstallerURL: w.InstallerURL, PerMachine: c.Windows.Installer.PerMachine, Arch: arch,
	}, dir)
}

// wingetName makes an identifier part from a name: letters and digits.
func wingetName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "App"
	}
	return b.String()
}
