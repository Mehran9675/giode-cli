package linuxpkg

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/google/rpmpack"
)

// rpmArch maps GOARCH to RPM architectures.
var rpmArch = map[string]string{"amd64": "x86_64", "arm64": "aarch64", "386": "i686", "arm": "armv7hl", "riscv64": "riscv64", "ppc64le": "ppc64le"}

// rpmRequires are the libraries Gio programs link, as sonames, which
// every RPM distribution resolves to its own packages.
var rpmRequires = []string{"libX11.so.6", "libX11-xcb.so.1", "libXcursor.so.1", "libXfixes.so.3", "libxkbcommon.so.0",
	"libxkbcommon-x11.so.0", "libEGL.so.1", "libwayland-client.so.0", "libwayland-cursor.so.0", "libwayland-egl.so.1"}

// RPMArch returns the RPM architecture for a GOARCH.
func RPMArch(goarch string) string {
	if a, ok := rpmArch[goarch]; ok {
		return a
	}
	return goarch
}

// RPM writes an RPM package installing the app under /usr.
func RPM(a App, out string) error {
	arch, ok := rpmArch[a.Arch]
	if !ok {
		return fmt.Errorf("no RPM architecture for GOARCH %s", a.Arch)
	}
	files, err := Files(a, "/usr")
	if err != nil {
		return err
	}
	summary := a.Summary
	if summary == "" {
		summary = a.Name
	}
	desc := a.Description
	if desc == "" {
		desc = summary
	}
	license := a.License
	if license == "" {
		license = "Proprietary"
	}
	var requires rpmpack.Relations
	suffix := ""
	if arch == "x86_64" || arch == "aarch64" || arch == "riscv64" || arch == "ppc64le" {
		suffix = "(64bit)"
	}
	for _, so := range rpmRequires {
		requires.Set(so + "()" + suffix)
	}
	r, err := rpmpack.NewRPM(rpmpack.RPMMetaData{
		Name: packageName(a.Command), Summary: summary, Description: desc,
		Version: a.Version, Release: strconv.Itoa(max(a.Build, 1)), Arch: arch, OS: "linux",
		Vendor: a.Publisher, URL: a.Website, Packager: a.maintainer(), Group: "Applications/Productivity",
		Licence: license, Compressor: "gzip", BuildTime: time.Now(), Requires: requires,
	})
	if err != nil {
		return err
	}
	// Only files: the directories belong to the system's packages
	// (filesystem, hicolor-icon-theme, ...).
	mtime := uint32(time.Now().Unix())
	for _, f := range files {
		r.AddFile(rpmpack.RPMFile{Name: f.Path, Body: f.Body, Mode: uint(f.Mode) | 0o100000, Owner: "root", Group: "root", MTime: mtime})
	}
	o, err := os.Create(out)
	if err != nil {
		return err
	}
	defer o.Close()
	return r.Write(o)
}
