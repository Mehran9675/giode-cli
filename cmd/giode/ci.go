package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/mehran9675/giode-cli/internal/config"
)

const bumpUsage = `
Usage: giode bump [major|minor|patch|build]

Raises app.version in giode.toml - major (1.4.2 to 2.0.0), minor (1.5.0)
or patch (1.4.3) - and always app.build, the build number stores require
to grow with every upload. "build" raises only the build number.

In CI, $GIODE_VERSION (a tag like v1.4.2) and $GIODE_BUILD (the run
number) override both without editing the file.
`

func cmdBump(args []string) error {
	fs := flags("bump", bumpUsage)
	dir := fs.String("C", ".", "project directory")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	part := "build"
	if len(pos) > 0 {
		part = pos[0]
	}
	c, err := loadProject(*dir)
	if err != nil {
		return err
	}
	v := strings.Split(c.App.Version, ".")
	for len(v) < 3 {
		v = append(v, "0")
	}
	n := make([]int, 3)
	for i := range n {
		n[i], _ = strconv.Atoi(v[i])
	}
	switch part {
	case "major":
		n = []int{n[0] + 1, 0, 0}
	case "minor":
		n = []int{n[0], n[1] + 1, 0}
	case "patch":
		n[2]++
	case "build":
	default:
		return fmt.Errorf("bump %q: say major, minor, patch or build", part)
	}
	version := fmt.Sprintf("%d.%d.%d", n[0], n[1], n[2])
	build := c.App.Build + 1
	path := filepath.Join(c.Dir, config.FileName)
	if err := setAppValues(path, version, build); err != nil {
		return err
	}
	fmt.Printf("app.version %s, app.build %d\n", version, build)
	return nil
}

var (
	versionLine = regexp.MustCompile(`(?m)^version\s*=\s*"[^"]*"`)
	buildLine   = regexp.MustCompile(`(?m)^build\s*=\s*\d+`)
)

// setAppValues sets version and build in giode.toml's [app] table.
func setAppValues(path, version string, build int) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	s := string(b)
	start := strings.Index(s, "[app]\n")
	if start < 0 {
		return errors.New("giode.toml has no [app] table")
	}
	end := len(s)
	if i := strings.Index(s[start+6:], "\n["); i >= 0 {
		end = start + 6 + i + 1
	}
	table := s[start:end]
	if versionLine.MatchString(table) {
		table = versionLine.ReplaceAllString(table, fmt.Sprintf("version = %q", version))
	} else {
		table = strings.Replace(table, "[app]\n", fmt.Sprintf("[app]\nversion = %q\n", version), 1)
	}
	if buildLine.MatchString(table) {
		table = buildLine.ReplaceAllString(table, fmt.Sprintf("build = %d", build))
	} else {
		table = versionLine.ReplaceAllStringFunc(table, func(m string) string { return m + fmt.Sprintf("\nbuild = %d", build) })
	}
	return os.WriteFile(path, []byte(s[:start]+table+s[end:]), 0o644)
}

const ciUsage = `
Usage: giode ci

Writes .github/workflows/release.yml: on a version tag (v1.4.2) it builds
the app for Linux (deb, rpm, web), Windows (setup program, MSIX), Android
(APK, AAB) and macOS (DMG), signing with what your repository's secrets
provide, and attaches everything to a GitHub release. The tag sets the
version and the run number the build number.

In giode.toml, [ci] setup runs commands before building on each runner
(installing the C libraries a cgo app links), and ci.windows_msys2 builds
the Windows packages on a Windows runner with MSYS2's MINGW64 toolchain:

	[ci]
	windows_msys2 = true
	[ci.setup]
	macos = "brew install mpv"
	linux = "sudo apt-get install -y libmpv-dev"
	windows = "pacman -S --noconfirm mingw-w64-x86_64-mpv"

Secrets it uses when set (Settings > Secrets and variables > Actions):

	ANDROID_KEYSTORE_BASE64, GIODE_KEYSTORE_PASS   Android release keystore
	WINDOWS_CERT_BASE64, GIODE_WINDOWS_CERT_PASS   Windows .pfx certificate
	MACOS_CERT_BASE64, MACOS_CERT_PASS             macOS Developer ID .p12
	GIODE_NOTARY_PASS                              notarization password
	GIODE_UPDATE_KEY                               update feed key (giode release)
`

func cmdCI(args []string) error {
	fs := flags("ci", ciUsage)
	dir := fs.String("C", ".", "project directory")
	force := fs.Bool("f", false, "overwrite an existing workflow")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	c, err := loadProject(*dir)
	if err != nil {
		return err
	}
	path := filepath.Join(c.Dir, ".github", "workflows", "release.yml")
	if _, err := os.Stat(path); err == nil && !*force {
		return fmt.Errorf("%s exists; use -f to overwrite it", relPath(path))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(workflow(c)), 0o644); err != nil {
		return err
	}
	fmt.Printf("Wrote %s. Push a tag to release: git tag v%s && git push --tags\n", relPath(path), c.App.Version)
	return nil
}

// workflow returns the release workflow for the project: with its
// [ci] setup commands, and the Windows build on a Windows runner with
// MSYS2 when ci.windows_msys2 is set (cgo apps linking C libraries).
func workflow(c *config.Config) string {
	step := func(cmds string, indent string) string {
		if strings.TrimSpace(cmds) == "" {
			return ""
		}
		var b strings.Builder
		b.WriteString(indent + "- name: Project setup (giode.toml ci.setup)\n" + indent + "  run: |\n")
		for _, l := range strings.Split(strings.TrimRight(cmds, "\n"), "\n") {
			b.WriteString(indent + "    " + l + "\n")
		}
		return b.String()
	}
	linuxWindows := "          # Signed when the repository has the certificate or keystore.\n" +
		"          win=\"\"; if [ -n \"$GIODE_WINDOWS_CERT_PASS\" ]; then win=\"-release\"; fi\n" +
		"          giode build $win windows-installer msix\n"
	windowsPaths := "            dist/windows/*.exe\n            dist/windows/*.msix\n"
	windowsJob, needs := "", "linux, macos"
	if c.CI.WindowsMSYS2 {
		linuxWindows, windowsPaths, needs = "", "", "linux, macos, windows"
		windowsJob = strings.NewReplacer("{{SETUP}}", step(c.CI.Setup.Windows, "      "),
			"{{CERT}}", orDefault(c.Windows.Signing.Certificate, "codesign.pfx")).Replace(windowsMSYS2Job)
	}
	return strings.NewReplacer(
		"{{KEYSTORE}}", orDefault(c.Android.Keystore, "release.keystore"),
		"{{CERT}}", orDefault(c.Windows.Signing.Certificate, "codesign.pfx"),
		"{{LINUX_SETUP}}\n", step(c.CI.Setup.Linux, "      "),
		"{{MACOS_SETUP}}\n", step(c.CI.Setup.MacOS, "      "),
		"{{LINUX_WINDOWS}}\n", linuxWindows,
		"{{WINDOWS_PATHS}}\n", windowsPaths,
		"{{WINDOWS_JOB}}\n", windowsJob,
		"{{NEEDS}}", needs,
	).Replace(releaseWorkflow)
}

// windowsMSYS2Job builds the Windows packages on a Windows runner, in
// MSYS2's MINGW64 environment, for cgo apps.
const windowsMSYS2Job = `  windows:
    runs-on: windows-latest
    defaults:
      run:
        shell: msys2 {0}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: stable
      - uses: msys2/setup-msys2@v2
        with:
          msystem: MINGW64
          path-type: inherit
          install: >-
            mingw-w64-x86_64-gcc
            mingw-w64-x86_64-pkgconf
            mingw-w64-x86_64-nsis
      - run: go install github.com/mehran9675/giode-cli/cmd/giode@latest
{{SETUP}}      - name: Signing certificate
        env:
          WINDOWS_CERT_BASE64: ${{ secrets.WINDOWS_CERT_BASE64 }}
        run: |
          if [ -n "$WINDOWS_CERT_BASE64" ]; then echo "$WINDOWS_CERT_BASE64" | base64 -d > {{CERT}}; fi
      - name: Build
        env:
          GIODE_WINDOWS_CERT_PASS: ${{ secrets.GIODE_WINDOWS_CERT_PASS }}
        run: |
          win=""; if [ -n "$GIODE_WINDOWS_CERT_PASS" ]; then win="-release"; fi
          "$(go env GOPATH)/bin/giode" build $win windows-installer msix
      - uses: actions/upload-artifact@v4
        with:
          name: windows
          path: |
            dist/windows/*.exe
            dist/windows/*.msix

`

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// releaseWorkflow is the GitHub Actions workflow giode ci writes.
const releaseWorkflow = `# Written by giode ci. Builds and publishes a release when a version tag is pushed.
name: release

on:
  push:
    tags: ["v*"]
  workflow_dispatch:

permissions:
  contents: write

env:
  GIODE_VERSION: ${{ github.ref_type == 'tag' && github.ref_name || '' }}
  GIODE_BUILD: ${{ github.run_number }}

jobs:
  linux:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: stable
      - name: Install build dependencies
        run: |
          sudo apt-get update
          sudo apt-get install -y gcc pkg-config libwayland-dev libx11-dev libx11-xcb-dev libxkbcommon-x11-dev libgles2-mesa-dev libegl1-mesa-dev libffi-dev libxcursor-dev libvulkan-dev nsis osslsigncode
          go install github.com/mehran9675/giode-cli/cmd/giode@latest
{{LINUX_SETUP}}
      - name: Signing keys
        env:
          ANDROID_KEYSTORE_BASE64: ${{ secrets.ANDROID_KEYSTORE_BASE64 }}
          WINDOWS_CERT_BASE64: ${{ secrets.WINDOWS_CERT_BASE64 }}
        run: |
          if [ -n "$ANDROID_KEYSTORE_BASE64" ]; then echo "$ANDROID_KEYSTORE_BASE64" | base64 -d > {{KEYSTORE}}; fi
          if [ -n "$WINDOWS_CERT_BASE64" ]; then echo "$WINDOWS_CERT_BASE64" | base64 -d > {{CERT}}; fi
      - name: Build
        env:
          GIODE_KEYSTORE_PASS: ${{ secrets.GIODE_KEYSTORE_PASS }}
          GIODE_WINDOWS_CERT_PASS: ${{ secrets.GIODE_WINDOWS_CERT_PASS }}
        run: |
          giode build deb rpm web
{{LINUX_WINDOWS}}
          apk=""; if [ -n "$GIODE_KEYSTORE_PASS" ]; then apk="-release"; fi
          giode build $apk android aab
      - uses: actions/upload-artifact@v4
        with:
          name: linux-windows-android
          path: |
            dist/linux/*.deb
            dist/linux/*.rpm
{{WINDOWS_PATHS}}
            dist/android/*.apk
            dist/android/*.aab
            dist/android/native-debug-symbols.zip

  macos:
    runs-on: macos-latest
    env:
      MACOS_CERT_BASE64: ${{ secrets.MACOS_CERT_BASE64 }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: stable
      - run: go install github.com/mehran9675/giode-cli/cmd/giode@latest
{{MACOS_SETUP}}
      - name: Signing certificate
        if: env.MACOS_CERT_BASE64 != ''
        env:
          MACOS_CERT_PASS: ${{ secrets.MACOS_CERT_PASS }}
        run: |
          echo "$MACOS_CERT_BASE64" | base64 -d > cert.p12
          security create-keychain -p ci build.keychain
          security default-keychain -s build.keychain
          security unlock-keychain -p ci build.keychain
          security import cert.p12 -k build.keychain -P "$MACOS_CERT_PASS" -T /usr/bin/codesign
          security set-key-partition-list -S apple-tool:,apple: -s -k ci build.keychain
          rm cert.p12
      - name: Build
        env:
          GIODE_NOTARY_PASS: ${{ secrets.GIODE_NOTARY_PASS }}
        run: |
          release=""
          if [ -n "$MACOS_CERT_BASE64" ]; then release="-release"; fi
          giode build $release dmg
      - uses: actions/upload-artifact@v4
        with:
          name: macos
          path: dist/macos/*.dmg

{{WINDOWS_JOB}}
  publish:
    needs: [{{NEEDS}}]
    if: github.ref_type == 'tag'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/download-artifact@v4
        with:
          path: artifacts
          merge-multiple: true
      - uses: softprops/action-gh-release@v2
        with:
          files: artifacts/**/*
          generate_release_notes: true
`
