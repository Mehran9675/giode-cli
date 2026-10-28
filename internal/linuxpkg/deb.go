package linuxpkg

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// debArch maps GOARCH to Debian architectures.
var debArch = map[string]string{"amd64": "amd64", "arm64": "arm64", "386": "i386", "arm": "armhf", "riscv64": "riscv64", "ppc64le": "ppc64el", "loong64": "loong64"}

// debDepends are the Debian packages of the libraries Gio programs link.
const debDepends = "libc6, libx11-6, libx11-xcb1, libxcursor1, libxfixes3, libxkbcommon0, libxkbcommon-x11-0, libegl1, libwayland-client0, libwayland-cursor0, libwayland-egl1"

// Deb writes a Debian package (.deb) installing the app under /usr.
func Deb(a App, out string) error {
	arch, ok := debArch[a.Arch]
	if !ok {
		return fmt.Errorf("no Debian architecture for GOARCH %s", a.Arch)
	}
	files, err := Files(a, "/usr")
	if err != nil {
		return err
	}
	sortFiles(files)
	mtime := time.Now()

	var data bytes.Buffer
	var md5sums strings.Builder
	size := int64(0)
	if err := writeTarGz(&data, func(tw *tar.Writer) error {
		for _, d := range dirsOf(files) {
			if err := tw.WriteHeader(&tar.Header{Name: "." + d + "/", Typeflag: tar.TypeDir, Mode: 0o755, ModTime: mtime}); err != nil {
				return err
			}
		}
		for _, f := range files {
			if err := tw.WriteHeader(&tar.Header{Name: "." + f.Path, Mode: int64(f.Mode), Size: int64(len(f.Body)), ModTime: mtime}); err != nil {
				return err
			}
			if _, err := tw.Write(f.Body); err != nil {
				return err
			}
			size += int64(len(f.Body))
			fmt.Fprintf(&md5sums, "%x  %s\n", md5.Sum(f.Body), strings.TrimPrefix(f.Path, "/"))
		}
		return nil
	}); err != nil {
		return err
	}

	summary := a.Summary
	if summary == "" {
		summary = a.Name
	}
	var control strings.Builder
	fmt.Fprintf(&control, "Package: %s\nVersion: %s-%d\nArchitecture: %s\nMaintainer: %s\n", packageName(a.Command), a.Version, max(a.Build, 1), arch, a.maintainer())
	fmt.Fprintf(&control, "Installed-Size: %d\nDepends: %s\nSection: utils\nPriority: optional\n", (size+1023)/1024, debDepends)
	if a.Website != "" {
		fmt.Fprintf(&control, "Homepage: %s\n", a.Website)
	}
	fmt.Fprintf(&control, "Description: %s\n", summary)
	if a.Description != "" && a.Description != summary {
		for _, line := range strings.Split(a.Description, "\n") {
			if strings.TrimSpace(line) == "" {
				line = "."
			}
			fmt.Fprintf(&control, " %s\n", line)
		}
	}
	var ctl bytes.Buffer
	if err := writeTarGz(&ctl, func(tw *tar.Writer) error {
		for _, f := range []File{{Path: "control", Body: []byte(control.String())}, {Path: "md5sums", Body: []byte(md5sums.String())}} {
			if err := tw.WriteHeader(&tar.Header{Name: "./" + f.Path, Mode: 0o644, Size: int64(len(f.Body)), ModTime: mtime}); err != nil {
				return err
			}
			if _, err := tw.Write(f.Body); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	o, err := os.Create(out)
	if err != nil {
		return err
	}
	defer o.Close()
	return writeAr(o, mtime, []File{
		{Path: "debian-binary", Body: []byte("2.0\n")},
		{Path: "control.tar.gz", Body: ctl.Bytes()},
		{Path: "data.tar.gz", Body: data.Bytes()},
	})
}

func writeTarGz(w io.Writer, fill func(*tar.Writer) error) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	if err := fill(tw); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

// writeAr writes an ar archive, the container of .deb packages.
func writeAr(w io.Writer, mtime time.Time, files []File) error {
	if _, err := io.WriteString(w, "!<arch>\n"); err != nil {
		return err
	}
	for _, f := range files {
		hdr := fmt.Sprintf("%-16s%-12d%-6d%-6d%-8s%-10d`\n", f.Path, mtime.Unix(), 0, 0, "100644", len(f.Body))
		if _, err := io.WriteString(w, hdr); err != nil {
			return err
		}
		if _, err := w.Write(f.Body); err != nil {
			return err
		}
		if len(f.Body)%2 == 1 {
			if _, err := w.Write([]byte{'\n'}); err != nil {
				return err
			}
		}
	}
	return nil
}
