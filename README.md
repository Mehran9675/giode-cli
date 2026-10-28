# giode CLI

The command-line tool for [giode](https://github.com/mehran9675/giode) apps: create a
project, run it on your computer, a phone, a simulator or the browser, and build signed
packages and installers for Android, iOS, macOS, Windows, Linux and the web.

```sh
giode new counter          # a new app with the latest giode
cd counter
giode run                  # on this computer
giode run android          # on a connected phone: build, install, launch, follow the log
giode run windows          # build the Windows app and run it (with Wine off Windows)
giode build android windows-installer web
giode doctor               # what's installed, and what's missing for each target
```

It builds on [gogio](https://gioui.org/doc/install), Gio's packaging tool, whose code it
includes, and adds what gogio can't do: Android manifest entries (services, any
permission, raw XML), Info.plist keys (permission texts, background modes), Windows
installers, Authenticode signing, Linux desktop packaging, a project file instead of
long flag lists, and the run, doctor and device commands.

## Install

**macOS and Linux:**

```sh
curl -fsSL https://raw.githubusercontent.com/Mehran9675/giode-cli/main/install.sh | sh
```

**Windows** (PowerShell):

```powershell
irm https://raw.githubusercontent.com/Mehran9675/giode-cli/main/install.ps1 | iex
```

Both download the latest release for your system and put `giode` on your PATH
(`/usr/local/bin` or `~/.local/bin`; `%LOCALAPPDATA%\Programs\giode` on Windows). Set
`GIODE_VERSION=v0.2.0` for a specific release. Or, with Go installed:

```sh
go install github.com/mehran9675/giode-cli/cmd/giode@latest
```

which puts `giode` in `$(go env GOPATH)/bin`. `giode self-update` updates a release
install. Building apps needs Go 1.26 or later; `giode doctor` lists what else each
target needs.

## Commands

| Command | What it does |
| --- | --- |
| `giode new [-id com.example.app] <dir>` | Creates a project: a counter app in `main.go`, `go.mod` with the latest giode, `giode.toml` with the settings to fill in, placeholder assets (`assets/icon.png`, `assets/README.md`), `.gitignore` and a README. |
| `giode init` | Adds `giode.toml` to an existing Go module. |
| `giode build <target>...` | Builds into `dist/<platform>`. Targets: `android` (APK), `aab` (Play bundle), `ios` (IPA), `ios-sim`, `macos` (universal), `dmg`, `windows`, `windows-installer` (setup program), `msix`, `winget`, `linux`, `deb`, `rpm`, `appimage`, `flatpak`, `web`, `desktop` (this computer) and `all` (everything this computer can build). `-release` signs with your certificates. |
| `giode run [target]` | Builds for a target and runs it: `desktop` (default, with `go run`), `linux`, `windows` (Wine off Windows), `windows-installer`, `macos`, `android` (builds the device's architecture, installs, launches, follows the log), `ios-sim`, `web` (serves `dist/web`). `-mcp` starts giode's MCP server so an AI agent can drive the app. |
| `giode devices` | Lists Android devices and booted iOS simulators. |
| `giode doctor` | Checks Go, the Android SDK, build tools, platform, NDK and JDK, Xcode, and Gio's Linux libraries, with install commands for what's missing. |
| `giode keystore` | Creates an Android release keystore and points `giode.toml` at it. |
| `giode cert` | Creates a self-signed Windows code signing certificate for testing and points `giode.toml` at it. |
| `giode sign <file>...` | Signs Windows executables and installers with the configured certificate. |
| `giode links` | Writes `.well-known/assetlinks.json` and `apple-app-site-association` for each host in `app.links`, to upload to your site (fingerprints from `android.keystore` and `android.fingerprints`, team from `ios.team_id`). |
| `giode upgrade [version]` | Updates the project's giode dependency and tidies `go.mod`. |
| `giode manifest [android\|ios\|macos]` | Prints what `giode.toml` adds to the manifest and Info.plist. |
| `giode clean` | Removes `dist`. |
| `giode self-update` | Updates the CLI to the latest release. |

Every command takes `-C dir` to run on a project elsewhere, and build commands take `-x`
to print what they run. Targets have short names too: `apk`, `bundle`, `win`, `mac`,
`sim`, `installer`, `browser`.

### What builds where

| Target | Builds on | Needs |
| --- | --- | --- |
| `android`, `aab` | Linux, macOS, Windows | Android SDK with build tools, a platform and the NDK; a JDK |
| `ios`, `ios-sim`, `macos`, `dmg` | macOS | Xcode; for `ios`, a provisioning profile |
| `windows` | Linux, macOS, Windows | nothing extra: Gio doesn't need cgo on Windows |
| `msix`, `winget` | Linux, macOS, Windows | nothing extra for `msix` (giode writes it); `winget` builds the setup program, so NSIS |
| `windows-installer` | Linux, macOS, Windows | [NSIS](https://nsis.sourceforge.io) (`winget install NSIS.NSIS`, `brew install makensis`, `apt install nsis`) |
| `linux` | Linux | gcc and Gio's Wayland/X11/EGL development packages |
| `deb`, `rpm` | Linux | as `linux`; packages are written by giode itself |
| `appimage` | Linux | as `linux`, plus [appimagetool](https://github.com/AppImage/appimagetool/releases) on PATH |
| `flatpak` | Linux | as `linux`; with `flatpak-builder` a `.flatpak` bundle, without it the manifest |
| `web` | anywhere | nothing extra |

`giode doctor` checks all of these.

## giode.toml

The project file. Only `app.name` and `app.id` are required.

```toml
[app]
name = "Pocket Agent"         # display name
id = "com.example.agent"      # Android package name and Apple bundle ID
version = "1.4.0"
build = 12                    # Android versionCode, CFBundleVersion: raise per upload
icon = "assets/icon.png"      # square PNG, 1024x1024 for the stores
package = "."                 # main package
publisher = "Example Ltd"     # shown by installers and Windows' app list
description = "An AI agent in your pocket."
website = "https://example.com"
schemes = ["agent"]           # URL schemes the app opens
links = ["https://example.com/app/*"]   # universal links / App Links; see giode links
tags = ""                     # Go build tags
ldflags = ""                  # extra linker flags
output = "dist"

[android]
min_sdk = 21
target_sdk = 35
arch = ["arm64", "amd64"]     # default: arm, arm64, 386, amd64
permissions = ["network", "camera", "microphone", "android.permission.VIBRATE"]
keep_alive = true             # declares giode's foreground service for App.KeepAlive
gio_patches = true            # build against Gio with giode's Android fixes (default true)
notifications = true          # POST_NOTIFICATIONS for App.Notify (default true)
queries = ["com.android.chrome"]
manifest = '<uses-feature android:name="android.hardware.camera" android:required="false"/>'
application = '<meta-data android:name="example" android:value="1"/>'
keystore = "release.keystore" # used by giode build -release
keystore_pass_env = "GIODE_KEYSTORE_PASS"

[ios]
min_os = 15
profile = "Agent.mobileprovision"   # for device builds

[ios.usage]                   # shown when iOS asks for a permission
camera = "To attach photos you take."
microphone = "To dictate messages."
speech = "To dictate messages."

[ios.plist]                   # any other Info.plist keys
UIBackgroundModes = ["audio"]
ITSAppUsesNonExemptEncryption = false

[macos]
min_os = 11
identity = "Developer ID Application: Example Ltd (ABCDE12345)"  # signs -release builds
notary_id = "you@example.com"
notary_team_id = "ABCDE12345"
notary_pass_env = "GIODE_NOTARY_PASS"

[macos.usage]
microphone = "To dictate messages."

[windows]
arch = "amd64"

[windows.installer]
per_machine = false           # true: all users, Program Files, asks for admin
desktop_shortcut = true
license = "assets/license.txt"

[windows.signing]
certificate = "codesign.pfx"  # password in $GIODE_WINDOWS_CERT_PASS
password_env = "GIODE_WINDOWS_CERT_PASS"
timestamp = "http://timestamp.digicert.com"   # "none" to skip
# command = ["AzureSignTool", "sign", "-kvu", "https://vault.vault.azure.net", "-kvc", "cert", "-kvm", "-tr", "http://timestamp.digicert.com", "{file}"]

[windows.msix]                       # giode build msix
publisher = "CN=Example Ltd"         # the signing certificate's subject (or Partner Center's ID)
identity_name = "ExampleLtd.PocketAgent"   # reserved in Partner Center for the Store

[windows.winget]                     # giode build winget
id = "Example.PocketAgent"
installer_url = "https://example.com/download/PocketAgent-{version}-setup.exe"
license = "MIT"

[linux]
categories = ["Utility", "Office"]
description = "Longer text for package managers and software centers."
maintainer = "Jane Doe <jane@example.com>"   # .deb/.rpm metadata
license = "MIT"                              # SPDX expression
# flatpak_permissions = ["--share=ipc", "--socket=wayland", "--device=dri"]

[web]
port = 8080
pwa = true                    # installable, works offline (default)
short_name = "Agent"          # under the home screen icon
theme_color = "#111827"
background_color = "#111827"

# The privacy manifest (PrivacyInfo.xcprivacy) App Store apps need. giode always
# declares the system boot time and file timestamp APIs Go itself uses.
[privacy]
tracking = false
[[privacy.collected]]
type = "crash_data"           # Apple's data types, by short or full name
linked = false
purposes = ["app_functionality"]
[privacy.api]
user_defaults = ["CA92.1"]    # more required-reason APIs your code uses

# Entries in the file manager's right-click menu that open the app with the
# chosen paths (giode.Config.OnContextAction receives them).
[[context_actions]]
id = "ask"
label = "Ask Pocket Agent"
extensions = [".md", ".txt"]  # or files = true for every file

[[context_actions]]
id = "here"
label = "Open Pocket Agent here"
folders = true
background = true             # the empty space of an open folder
```

**Android permissions** take gogio's short names (`network`, `networkstate`,
`bluetooth`, `camera`, `storage`, `wakelock`, `microphone`), giode's (`notifications`,
`foreground_service`, `vibrate`, `biometric`, `location`, `coarse_location`,
`contacts`, `calendar`, `boot`) or full `android.permission.X` names. Importing
`gioui.org/app/permission/...` packages still adds theirs, as with gogio.

**iOS usage texts** take short names: `camera`, `microphone`, `speech`, `photos`,
`photos_add`, `location`, `location_always`, `contacts`, `calendar`, `reminders`,
`bluetooth`, `faceid`, `motion`, `health`, `health_update`, `tracking`,
`local_network`. iOS ends an app that uses the camera, microphone or speech recognition
without one; giode's `PickPhoto` and `Dictate` check for them and return an error instead.

**Context actions** go into the build (the giode library reads them, and
`App.InstallContextActions` registers them for the current user), the Windows installer
(Explorer's context menu, removed on uninstall; on Windows 11 under "Show more options"),
and the macOS app's `NSServices` (Finder's Quick Actions, once the app is in
`/Applications` and has run once).

## Signing

`giode build -release` builds for distribution and signs everything it can with the
certificates in `giode.toml`; without `-release`, Android uses the debug key and the
rest are unsigned.

| Platform | What signs it | Set up |
| --- | --- | --- |
| Android | `android.keystore` | `GIODE_KEYSTORE_PASS=... giode keystore` creates one. Keep it and its password safe: Google Play only accepts updates signed with it (with Play App Signing, it's your upload key). |
| Windows | `windows.signing`: an Authenticode `.pfx`, or a `command` | Signs the executable and the setup program, with a timestamp so signatures outlive the certificate. Uses `signtool` on Windows and `osslsigncode` elsewhere. For keys in a hardware token or a cloud service (Azure Trusted Signing, a KMS), give `command` with `{file}` where the file goes. `giode cert` makes a self-signed certificate to test with; for public releases, use one from a certificate authority so SmartScreen doesn't warn. |
| macOS | `macos.identity` (or `macos.profile`) | A "Developer ID Application" identity in your keychain. With `macos.notary_id` and the app-specific password in `$GIODE_NOTARY_PASS`, the app is also notarized, so Gatekeeper opens it without warnings. |
| iOS | `ios.profile` | Device builds always need a provisioning profile; the matching identity must be in your keychain. |

`giode sign <file>...` signs other Windows files the same way.

## Windows installer

`giode build windows-installer` makes `dist/windows/<App>-<version>-setup.exe` with NSIS:
a wizard (welcome, optional license, folder, progress, finish with a "Run" box), a Start
menu shortcut (and a desktop one with `desktop_shortcut`), an entry in Settings > Apps
with publisher, version and size, and an uninstaller. By default it installs for the
current user into `%LOCALAPPDATA%\Programs`, without an administrator prompt;
`per_machine = true` installs for everyone into Program Files. It runs silently with
`/S`. The uninstaller isn't signed yet.

## Patched Gio for Android

`giode build android` (and `aab`, and `giode run android`) builds against a copy of your Gio
version with two Android fixes Gio hasn't released:

- **Full refresh rate.** Gio asks Android's Choreographer for the next frame only after
  presenting the current one. Presenting waits for a vsync, so the request misses the next
  one, and a 120Hz display gets 60 frames a second while anything animates. The patched
  copy asks before drawing, and drops a frame callback that arrives after the animation
  stopped instead of drawing a frame for nothing.
- **Held keys repeat.** After every edit Gio restarts the soft keyboard's input connection
  if the editor's snippet of text differs from the keyboard's - which it does after any
  edit, since the editor's snippet window moves with the text. The restart cancels the
  keyboard's key repeat, so holding backspace deleted one character. The patched copy
  restarts only when the text changed under the keyboard (an app clearing or replacing the
  field), not when the editor just moved its window over text the keyboard already has.

Your `go.mod` isn't touched. The copy is made once per Gio version in the user cache
directory (`giode/gio/<version>-giode<n>`, made again when the patches change), and the build gets a temporary `go.mod` with
`replace gioui.org => <copy>`, passed with `-modfile` to the `go build` and `go list`
commands run in your module (`GOFLAGS` is left alone). The patch is skipped, with a note,
when go isn't in module mode, or your module already replaces `gioui.org`, vendors its
dependencies or is in a Go workspace, or when your Gio version doesn't have the code the patch expects. Set
`gio_patches = false` under `[android]` to build against plain Gio.

## License

Apache License 2.0; see [LICENSE](LICENSE). `internal/gogio` is derived from gioui.org's
gogio and stays under its Unlicense/MIT terms; see [NOTICE](NOTICE).
