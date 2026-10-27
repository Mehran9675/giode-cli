# giode CLI

The command-line tool for [giode](https://github.com/mehran9675/giode) apps: create a
project, run it on your computer, a phone, a simulator or the browser, and build
packages for Android, iOS, macOS, Windows, Linux and the web.

```sh
go install github.com/mehran9675/giode-cli/cmd/giode@latest

giode new notes        # a new app with the latest giode
cd notes
giode run              # on this computer
giode run android      # on a connected phone: build, install, launch, follow the log
giode build android windows web
giode doctor           # what's installed, and what's missing for each target
```

It builds on [gogio](https://gioui.org/doc/install), Gio's packaging tool, whose code it
includes, and adds what gogio can't do: Android manifest entries (services, any
permission, raw XML), Info.plist keys (permission texts, background modes), Linux
desktop packaging, a project file instead of long flag lists, and the run, doctor and
device commands.

## Commands

| Command | What it does |
| --- | --- |
| `giode new [-template app\|minimal] [-id com.example.app] <dir>` | Creates a project: `main.go`, `go.mod` with the latest giode, `giode.toml`, a placeholder icon, `.gitignore` and a README. |
| `giode init` | Adds `giode.toml` to an existing Go module. |
| `giode build <target>...` | Builds into `dist/<platform>`. Targets: `android` (APK), `aab` (Play bundle), `ios` (IPA), `ios-sim`, `macos`, `windows`, `linux`, `web`, `desktop` (this computer) and `all` (everything this computer can build). `-release` signs Android builds with your keystore. |
| `giode run [desktop\|android\|ios-sim\|web]` | Builds and runs. On Android it builds only the device's architecture, installs, launches and follows the app's log; `web` serves `dist/web`. `-mcp` starts giode's MCP server so an AI agent can drive the app. |
| `giode devices` | Lists Android devices and booted iOS simulators. |
| `giode doctor` | Checks Go, the Android SDK, build tools, platform, NDK and JDK, Xcode, and Gio's Linux libraries, with install commands for what's missing. |
| `giode keystore` | Creates an Android release keystore and points `giode.toml` at it. |
| `giode upgrade [version]` | Updates the project's giode dependency and tidies `go.mod`. |
| `giode manifest [android\|ios\|macos]` | Prints what `giode.toml` adds to the manifest and Info.plist. |
| `giode clean` | Removes `dist`. |

Every command takes `-C dir` to run on a project elsewhere, and build commands take `-x`
to print what they run.

### What builds where

| Target | Builds on | Needs |
| --- | --- | --- |
| `android`, `aab` | Linux, macOS, Windows | Android SDK with build tools, a platform and the NDK; a JDK |
| `ios`, `ios-sim`, `macos` | macOS | Xcode; for `ios`, a provisioning profile |
| `windows` | Linux, macOS, Windows | nothing extra: Gio doesn't need cgo on Windows |
| `linux` | Linux | gcc and Gio's Wayland/X11/EGL development packages |
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
schemes = ["agent"]           # URL schemes the app opens
tags = ""                     # Go build tags
ldflags = ""                  # extra linker flags
output = "dist"

[android]
min_sdk = 21
target_sdk = 35
arch = ["arm64", "amd64"]     # default: arm, arm64, 386, amd64
permissions = ["network", "camera", "microphone", "android.permission.VIBRATE"]
keep_alive = true             # declares giode's foreground service for App.KeepAlive
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
notary_id = "you@example.com"
notary_team_id = "ABCDE12345"
notary_pass_env = "GIODE_NOTARY_PASS"

[macos.usage]
microphone = "To dictate messages."

[windows]
arch = "amd64"

[linux]
categories = ["Utility", "Office"]

[web]
port = 8080
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

## Signing

- **Android debug builds** use the Android debug key, created on first use.
- **Android release builds**: `GIODE_KEYSTORE_PASS=... giode keystore` once, then
  `GIODE_KEYSTORE_PASS=... giode build -release aab` for Google Play. Keep the keystore
  and its password safe; with Play App Signing it's your upload key.
- **iOS devices**: set `ios.profile` to a provisioning profile; the matching signing
  identity must be in your keychain.
- **macOS**: set `macos.profile` to sign, and the notary settings to notarize.

## License

Apache License 2.0; see [LICENSE](LICENSE). `internal/gogio` is derived from gioui.org's
gogio and stays under its Unlicense/MIT terms; see [NOTICE](NOTICE).
