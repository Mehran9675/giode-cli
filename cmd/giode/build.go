package main

import (
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	_ "image/jpeg"

	"github.com/mehran9675/giode-cli/internal/config"
	"github.com/mehran9675/giode-cli/internal/giopatch"
	"github.com/mehran9675/giode-cli/internal/gogio"
	"github.com/mehran9675/giode-cli/internal/installer"
	"github.com/mehran9675/giode-cli/internal/sign"
)

const buildUsage = `
Usage: giode build [options] <target>...

Builds the app in giode.toml for each target, into dist/<platform>:

	android   an APK for phones and tablets (dist/android/<App>.apk)
	aab       an Android App Bundle for Google Play (dist/android/<App>.aab)
	ios       an IPA for devices, signed with ios.profile (macOS only)
	ios-sim   an app for the iOS simulator (macOS only)
	macos     a macOS app bundle (macOS only)
	windows   a Windows executable with icon and manifest (from any OS)
	windows-installer
	          a Windows setup program (<App>-<version>-setup.exe) with Start
	          menu shortcut and uninstaller, made with NSIS (from any OS)
	linux     a Linux executable with a .desktop file and icon (Linux only)
	web       a WebAssembly site (dist/web)
	desktop   this computer's platform
	all       every target this computer can build

Android and iOS builds add what giode.toml asks for to the manifest and
Info.plist: permissions, giode's background service (android.keep_alive),
usage texts, raw XML and plist keys.

-release builds for distribution, signed with the certificates in
giode.toml: Android with android.keystore, Windows executables and
installers with windows.signing (Authenticode), macOS with
macos.identity (and notarized with macos.notary_id). iOS device builds are
always signed with ios.profile. Without -release, Android uses the debug
key and the rest are unsigned.

Short names work too: apk, bundle, win, mac, sim, installer.

Example:

	giode build android windows
	giode build -release aab windows-installer
`

// buildOpts are the options shared by build and run.
type buildOpts struct {
	release bool
	arch    string
	output  string
	work    bool
	args    []string
}

func cmdBuild(args []string) error {
	fs := flags("build", buildUsage)
	var o buildOpts
	dir := fs.String("C", ".", "project directory")
	fs.BoolVar(&o.release, "release", false, "build for distribution, signed with the certificates in giode.toml")
	fs.StringVar(&o.arch, "arch", "", "comma separated GOARCHs to build (default: the target's set)")
	fs.StringVar(&o.output, "o", "", "output file or directory (one target only)")
	fs.BoolVar(&o.work, "work", false, "keep and print gogio's work directory")
	fs.BoolVar(&verbose, "x", false, "print the commands run")
	targets, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		fs.Usage()
		return errors.New("name a target")
	}
	if o.output != "" && len(targets) > 1 {
		return errors.New("-o works with one target only")
	}
	c, err := loadProject(*dir)
	if err != nil {
		return err
	}
	targets = expandTargets(targets)
	var failed []string
	for _, t := range targets {
		fmt.Printf("Building %s…\n", t)
		out, err := buildTarget(c, t, o)
		if err != nil {
			fmt.Fprintf(os.Stderr, "giode build %s: %v\n", t, err)
			failed = append(failed, t)
			continue
		}
		fmt.Printf("  %s\n", relPath(out))
	}
	if len(failed) > 0 {
		return fmt.Errorf("failed: %s", strings.Join(failed, ", "))
	}
	return nil
}

// targetNames are the targets build accepts, besides desktop and all.
var targetNames = []string{"android", "aab", "ios", "ios-sim", "macos", "windows", "windows-installer", "linux", "web"}

// targetAliases are short and alternative target names.
var targetAliases = map[string]string{
	"apk":           "android",
	"bundle":        "aab",
	"mac":           "macos",
	"osx":           "macos",
	"darwin":        "macos",
	"win":           "windows",
	"exe":           "windows",
	"installer":     "windows-installer",
	"setup":         "windows-installer",
	"msi":           "windows-installer",
	"sim":           "ios-sim",
	"simulator":     "ios-sim",
	"ios-simulator": "ios-sim",
	"iphone":        "ios",
	"wasm":          "web",
	"js":            "web",
	"browser":       "web",
}

// canonical resolves a target alias.
func canonical(t string) string {
	t = strings.ToLower(t)
	if a, ok := targetAliases[t]; ok {
		return a
	}
	return t
}

// hostTarget is the desktop target for this computer.
func hostTarget() string {
	switch runtime.GOOS {
	case "darwin":
		return "macos"
	case "windows":
		return "windows"
	default:
		return "linux"
	}
}

func expandTargets(ts []string) []string {
	var out []string
	for _, t := range ts {
		switch t = canonical(t); t {
		case "desktop":
			out = append(out, hostTarget())
		case "all":
			out = append(out, "web", "windows")
			if _, err := exec.LookPath("makensis"); err == nil {
				out = append(out, "windows-installer")
			}
			if runtime.GOOS == "linux" {
				out = append(out, "linux")
			}
			if runtime.GOOS == "darwin" {
				out = append(out, "macos", "ios-sim")
			}
			if androidHome() != "" {
				out = append(out, "android")
			}
		default:
			out = append(out, t)
		}
	}
	return out
}

// buildTarget builds one target and returns the output path.
func buildTarget(c *config.Config, target string, o buildOpts) (string, error) {
	switch target {
	case "linux":
		return buildLinux(c, o)
	case "windows-installer":
		return buildWindowsInstaller(c, o)
	case "android", "aab", "ios", "ios-sim", "macos", "windows", "web":
	default:
		return "", fmt.Errorf("unknown target %q; targets: %s, desktop, all", target, strings.Join(targetNames, ", "))
	}
	g, err := gogioOptions(c, target, o)
	if err != nil {
		return "", err
	}
	// gogio runs go in the working directory.
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if err := os.Chdir(c.Dir); err != nil {
		return "", err
	}
	defer os.Chdir(wd)
	if (target == "android" || target == "aab") && (c.Android.GioPatches == nil || *c.Android.GioPatches) {
		modfile, cleanup, err := giopatch.Prepare(c.Dir, giopatch.Android)
		switch {
		case err == nil:
			defer cleanup()
			if verbose {
				fmt.Fprintf(os.Stderr, "  building against patched Gio (%s)\n", modfile)
			}
			g.ModFile = modfile
		case errors.Is(err, giopatch.ErrSkipped):
			fmt.Fprintf(os.Stderr, "  note: Gio's Android frame pacing fix not applied (%v)\n", err)
		default:
			return "", fmt.Errorf("patching Gio: %w (set android.gio_patches = false to skip)", err)
		}
	}
	if err := gogio.Build(pkgPath(c), g); err != nil {
		return "", err
	}
	if target == "windows" && o.release {
		if err := signWindows(c, g.Output); err != nil {
			return "", err
		}
	}
	return g.Output, nil
}

// signWindows signs a release executable or installer when
// windows.signing is set up, and says so when it isn't.
func signWindows(c *config.Config, file string) error {
	if !c.Windows.Signing.Configured() {
		fmt.Fprintf(os.Stderr, "  note: %s is unsigned; set windows.signing in giode.toml so Windows doesn't warn about it\n", filepath.Base(file))
		return nil
	}
	sign.Verbose = verbose
	if err := sign.Windows(c, file, c.App.Name); err != nil {
		return fmt.Errorf("signing %s: %w", filepath.Base(file), err)
	}
	fmt.Printf("  signed %s\n", filepath.Base(file))
	return nil
}

// buildWindowsInstaller builds the Windows executable and a setup
// program installing it.
func buildWindowsInstaller(c *config.Config, o buildOpts) (string, error) {
	exeOpts := o
	exeOpts.output = ""
	exe, err := buildTarget(c, "windows", exeOpts)
	if err != nil {
		return "", err
	}
	out := filepath.Join(filepath.Dir(exe), fmt.Sprintf("%s-%s-setup.exe", binaryName(c), c.App.Version))
	if o.output != "" {
		out, _ = filepath.Abs(o.output)
	}
	inst := c.Windows.Installer
	opts := installer.Options{
		Name: c.App.Name, ID: c.App.ID, Version: c.App.Version,
		Publisher: c.App.Publisher, Website: c.App.Website,
		Exe: exe, Icon: c.Path(c.App.Icon), License: c.Path(inst.License),
		PerMachine: inst.PerMachine, DesktopShortcut: inst.DesktopShortcut,
		Output: out, Verbose: verbose,
	}
	for _, v := range c.WindowsVerbs() {
		opts.ContextMenu = append(opts.ContextMenu, installer.Verb{Key: v.Key, Label: v.Label, ID: v.ID, Arg: v.Arg})
	}
	if err := installer.Build(opts); err != nil {
		return "", err
	}
	if o.release {
		if err := signWindows(c, out); err != nil {
			return "", err
		}
	}
	return out, nil
}

// gogioOptions turns giode.toml into gogio options for a target.
func gogioOptions(c *config.Config, target string, o buildOpts) (gogio.Options, error) {
	g := gogio.Options{
		AppID:         c.App.ID,
		Name:          binaryName(c),
		Version:       c.GogioVersion(),
		Tags:          c.App.Tags,
		Ldflags:       strings.TrimSpace(c.App.Ldflags + " " + c.ContextActionsLdflag()),
		Schemes:       c.App.Schemes,
		Arch:          o.arch,
		PrintCommands: verbose,
		KeepWorkdir:   o.work,
		Args:          o.args,
	}
	if icon := c.Path(c.App.Icon); icon != "" {
		if err := checkIcon(icon); err != nil {
			return g, err
		}
		g.Icon = icon
	}
	var platform, file string
	switch target {
	case "android", "aab":
		if err := ensureAndroidHome(); err != nil {
			return g, err
		}
		g.Target = "android"
		g.MinSDK, g.TargetSDK = c.Android.MinSDK, c.Android.TargetSDK
		g.Queries = c.Android.Queries
		if g.Arch == "" && len(c.Android.Arch) > 0 {
			g.Arch = strings.Join(c.Android.Arch, ",")
		}
		perms, manifest, app := c.AndroidExtras()
		g.Extras = gogio.Extras{AndroidPermissions: perms, AndroidManifest: manifest, AndroidApplication: app}
		if o.release {
			if c.Android.Keystore == "" {
				return g, errors.New("-release needs android.keystore in giode.toml (giode keystore creates one)")
			}
			g.SignKey = c.Path(c.Android.Keystore)
			g.SignPass = os.Getenv(c.Android.KeystorePassEnv)
			if g.SignPass == "" {
				return g, fmt.Errorf("-release needs the keystore password in $%s", c.Android.KeystorePassEnv)
			}
		}
		platform, file = "android", binaryName(c)+".apk"
		if target == "aab" {
			file = binaryName(c) + ".aab"
		}
	case "ios", "ios-sim":
		if runtime.GOOS != "darwin" {
			return g, errors.New("iOS builds need macOS and Xcode")
		}
		g.Target = "ios"
		g.MinSDK = c.IOS.MinOS
		if g.MinSDK == 0 {
			g.MinSDK = 15
		}
		// Go's cgo build of the iOS runtime needs an explicit deployment
		// target, or clang gets -miphoneos-version-min=0.0.
		if os.Getenv("IPHONEOS_DEPLOYMENT_TARGET") == "" {
			os.Setenv("IPHONEOS_DEPLOYMENT_TARGET", strconv.Itoa(g.MinSDK)+".0")
		}
		plist, err := c.IOSPlist()
		if err != nil {
			return g, err
		}
		g.Extras = gogio.Extras{InfoPlist: plist}
		platform, file = "ios", binaryName(c)+".ipa"
		if target == "ios-sim" {
			// gogio builds simulator apps for amd64; Xcode runs them on
			// Apple silicon too.
			if g.Arch == "" {
				g.Arch = "amd64"
			}
			file = binaryName(c) + ".app"
		} else {
			if c.IOS.Profile == "" {
				return g, errors.New("device builds need ios.profile, a provisioning profile, in giode.toml; use ios-sim for the simulator")
			}
			g.SignKey = c.Path(c.IOS.Profile)
		}
	case "macos":
		if runtime.GOOS != "darwin" {
			return g, errors.New("macOS app bundles need macOS")
		}
		g.Target = "macos"
		g.MinSDK = c.MacOS.MinOS
		plist, err := c.MacOSPlist()
		if err != nil {
			return g, err
		}
		g.Extras = gogio.Extras{InfoPlist: plist}
		if o.release {
			switch {
			case c.MacOS.Profile != "":
				g.SignKey = c.Path(c.MacOS.Profile)
			case c.MacOS.Identity != "":
				g.SignKey = c.MacOS.Identity
			default:
				fmt.Fprintln(os.Stderr, "  note: the app is unsigned; set macos.identity in giode.toml so Gatekeeper lets it open")
			}
			if c.MacOS.NotaryID != "" {
				if g.SignKey == "" {
					return g, errors.New("notarizing needs a signed app: set macos.identity")
				}
				g.NotaryID, g.NotaryTeamID = c.MacOS.NotaryID, c.MacOS.NotaryTeamID
				g.NotaryPass = os.Getenv(c.MacOS.NotaryPassEnv)
			}
		}
		platform, file = "macos", binaryName(c)+".app"
	case "windows":
		g.Target = "windows"
		if g.Arch == "" {
			g.Arch = c.Windows.Arch
		}
		if g.Arch == "" {
			g.Arch = "amd64"
		}
		platform, file = "windows", binaryName(c)+".exe"
	case "web":
		g.Target = "js"
		platform, file = "web", ""
	}
	if o.output != "" {
		g.Output = o.output
		if !filepath.IsAbs(g.Output) {
			wd, _ := os.Getwd()
			g.Output = filepath.Join(wd, g.Output)
		}
		return g, nil
	}
	d, err := outDir(c, platform)
	if err != nil {
		return g, err
	}
	g.Output = filepath.Join(d, file)
	if target == "windows" && g.Arch != "amd64" {
		g.Output = filepath.Join(d, binaryName(c)+"-"+g.Arch+".exe")
	}
	if target == "web" {
		g.Output = d
	}
	return g, nil
}

// checkIcon reports an icon the stores and launchers would reject.
func checkIcon(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("app.icon: %w", err)
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return fmt.Errorf("app.icon %s: %w", path, err)
	}
	if cfg.Width != cfg.Height {
		return fmt.Errorf("app.icon %s is %dx%d; it must be square", path, cfg.Width, cfg.Height)
	}
	if cfg.Width < 512 {
		fmt.Fprintf(os.Stderr, "giode: app.icon %s is %dx%d; stores want at least 512x512 (1024x1024 for the App Store)\n", path, cfg.Width, cfg.Height)
	}
	return nil
}

// buildLinux builds a Linux executable with a .desktop file and icon.
// Gio needs cgo on Linux, so this builds on Linux only.
func buildLinux(c *config.Config, o buildOpts) (string, error) {
	if runtime.GOOS != "linux" {
		return "", errors.New("Linux builds need a Linux computer (Gio uses cgo for Wayland and X11); build in a Linux VM, container or CI")
	}
	d, err := outDir(c, "linux")
	if err != nil {
		return "", err
	}
	bin := filepath.Join(d, strings.ToLower(binaryName(c)))
	if o.output != "" {
		bin, _ = filepath.Abs(o.output)
		d = filepath.Dir(bin)
	}
	ldflags := strings.TrimSpace("-s -w -X gioui.org/app.ID=" + c.App.ID + " " + c.App.Ldflags + " " + c.ContextActionsLdflag())
	args := []string{"build", "-trimpath", "-ldflags", ldflags, "-o", bin}
	if c.App.Tags != "" {
		args = append(args, "-tags", c.App.Tags)
	}
	cmd := command(c, "go", append(args, pkgPath(c))...)
	if o.arch != "" {
		cmd.Env = append(os.Environ(), "GOARCH="+o.arch)
	}
	if err := run(cmd); err != nil {
		return "", err
	}
	if icon := c.Path(c.App.Icon); icon != "" {
		if err := writeLinuxIcon(icon, filepath.Join(d, c.App.ID+".png")); err != nil {
			return "", err
		}
	}
	cats := "Utility;"
	if len(c.Linux.Categories) > 0 {
		cats = strings.Join(c.Linux.Categories, ";") + ";"
	}
	desktop := fmt.Sprintf("[Desktop Entry]\nType=Application\nName=%s\nExec=%s\nIcon=%s\nCategories=%s\nTerminal=false\n",
		c.App.Name, filepath.Base(bin), c.App.ID, cats)
	if err := os.WriteFile(filepath.Join(d, c.App.ID+".desktop"), []byte(desktop), 0o644); err != nil {
		return "", err
	}
	return bin, nil
}

// writeLinuxIcon copies the icon as PNG, as desktop environments
// expect.
func writeLinuxIcon(src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	err = png.Encode(out, img)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}
