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
	wf := strings.NewReplacer("{{KEYSTORE}}", orDefault(c.Android.Keystore, "release.keystore"),
		"{{CERT}}", orDefault(c.Windows.Signing.Certificate, "codesign.pfx")).Replace(releaseWorkflow)
	if err := os.WriteFile(path, []byte(wf), 0o644); err != nil {
		return err
	}
	fmt.Printf("Wrote %s. Push a tag to release: git tag v%s && git push --tags\n", relPath(path), c.App.Version)
	return nil
}

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
          # Signed when the repository has the certificate or keystore.
          win=""; if [ -n "$GIODE_WINDOWS_CERT_PASS" ]; then win="-release"; fi
          giode build $win windows-installer msix
          apk=""; if [ -n "$GIODE_KEYSTORE_PASS" ]; then apk="-release"; fi
          giode build $apk android aab
      - uses: actions/upload-artifact@v4
        with:
          name: linux-windows-android
          path: |
            dist/linux/*.deb
            dist/linux/*.rpm
            dist/windows/*.exe
            dist/windows/*.msix
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

  publish:
    needs: [linux, macos]
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
