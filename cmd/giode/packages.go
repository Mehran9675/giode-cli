package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mehran9675/giode-cli/internal/config"
	"github.com/mehran9675/giode-cli/internal/linuxpkg"
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
