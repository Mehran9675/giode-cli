package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mehran9675/giode-cli/internal/config"
	"github.com/mehran9675/giode-cli/internal/native"
)

// nativeLibs returns the shared libraries bin loads that [native]
// bundles, warning about imported ones it couldn't find.
func nativeLibs(c *config.Config, goos, bin string) ([]native.Lib, error) {
	if !c.Native.Bundle {
		return nil, nil
	}
	search, libs, exclude := c.NativeOptions(goos)
	r, err := native.Collect(bin, native.Options{GOOS: goos, Search: search, Libraries: libs, Exclude: exclude})
	if err != nil {
		return nil, fmt.Errorf("native libraries: %w", err)
	}
	if len(r.Missing) > 0 {
		section := map[string]string{"windows": "native.windows", "darwin": "native.macos"}[goos]
		if section == "" {
			section = "native.linux"
		}
		fmt.Fprintf(os.Stderr, "  warning: %s needs %s, not found in %s.search; the package won't run where they're missing\n",
			filepath.Base(bin), strings.Join(r.Missing, ", "), section)
	}
	if verbose {
		for _, l := range r.Libs {
			fmt.Fprintf(os.Stderr, "  bundling %s (%s)\n", l.Name, l.Path)
		}
	}
	return r.Libs, nil
}

// copyLibs copies libs into dir and returns their new paths.
func copyLibs(libs []native.Lib, dir string) ([]string, error) {
	if len(libs) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var out []string
	for _, l := range libs {
		dst := filepath.Join(dir, l.Name)
		if err := copyLib(l.Path, dst); err != nil {
			return nil, err
		}
		out = append(out, dst)
	}
	return out, nil
}

// copyLib copies src (following links) to dst, writable.
func copyLib(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	os.Remove(dst)
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// windowsDLLs bundles the DLLs exe loads next to it and returns their
// paths.
func windowsDLLs(c *config.Config, exe string) ([]string, error) {
	libs, err := nativeLibs(c, "windows", exe)
	if err != nil {
		return nil, err
	}
	return copyLibs(libs, filepath.Dir(exe))
}

// linuxRpathLdflag is the linker flag giving a Linux binary the run
// path of its bundled libraries: lib next to it (dist/linux), and its
// package's private lib directory. It's a DT_RPATH, not the DT_RUNPATH
// linkers write by default, which applies only to the binary's own
// imports and not to the libraries those load.
func linuxRpathLdflag(c *config.Config, command string) string {
	if !c.Native.Bundle {
		return ""
	}
	return "-extldflags=-Wl,--disable-new-dtags,-rpath,$ORIGIN/lib:$ORIGIN/../lib/" + command
}

// macBundleLibs copies the libraries a macOS app's executable loads
// into Contents/Frameworks, as dylibbundler does: each renamed
// @rpath/<name>, every reference to it rewritten, the executable given
// the Frameworks run path, and the changed files signed ad hoc (the
// app's own signing, when there is one, signs them again).
func macBundleLibs(c *config.Config) func(app string) error {
	if !c.Native.Bundle {
		return nil
	}
	return func(app string) error {
		bins, _ := filepath.Glob(filepath.Join(app, "Contents", "MacOS", "*"))
		if len(bins) == 0 {
			return fmt.Errorf("no executable in %s", app)
		}
		bin := bins[0]
		libs, err := nativeLibs(c, "darwin", bin)
		if err != nil || len(libs) == 0 {
			return err
		}
		if runtime.GOOS != "darwin" {
			return fmt.Errorf("bundling macOS libraries needs install_name_tool, on macOS")
		}
		fw := filepath.Join(app, "Contents", "Frameworks")
		copied, err := copyLibs(libs, fw)
		if err != nil {
			return err
		}
		plan := native.PlanMac(libs)
		tool := func(args ...string) error {
			cmd := exec.Command("install_name_tool", args...)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("install_name_tool %s: %v\n%s", strings.Join(args, " "), err, out)
			}
			return nil
		}
		for i, l := range libs {
			if err := tool(plan.InstallNameArgs(l.Name, copied[i])...); err != nil {
				return err
			}
		}
		if err := tool(plan.InstallNameArgs("", bin)...); err != nil {
			return err
		}
		// Already there when the binary was built with it: ignore.
		exec.Command("install_name_tool", "-add_rpath", "@executable_path/../Frameworks", bin).Run()
		for _, f := range append(copied, bin) {
			if out, err := exec.Command("codesign", "--force", "--sign", "-", f).CombinedOutput(); err != nil {
				return fmt.Errorf("codesign %s: %v\n%s", filepath.Base(f), err, out)
			}
		}
		return nil
	}
}

// checkNativeTools checks the tools [native] bundling needs here.
func checkNativeTools(c *config.Config) []check {
	if !c.Native.Bundle || runtime.GOOS != "darwin" {
		return nil
	}
	var checks []check
	for _, t := range []string{"install_name_tool", "codesign"} {
		ch := check{area: "Native libraries (giode.toml [native])", name: t}
		if p, err := exec.LookPath(t); err == nil {
			ch.ok, ch.detail = true, p
		} else {
			ch.fix = "Install Xcode's command line tools: xcode-select --install"
		}
		checks = append(checks, ch)
	}
	return checks
}
