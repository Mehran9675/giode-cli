// Package config reads giode.toml, the project file of a giode app, and
// turns it into build settings: the app's identity, and the Android
// manifest and Info.plist entries gogio can't add on its own.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// FileName is the project file giode looks for.
const FileName = "giode.toml"

// Config is the content of giode.toml.
type Config struct {
	App     App     `toml:"app"`
	Android Android `toml:"android"`
	IOS     IOS     `toml:"ios"`
	MacOS   MacOS   `toml:"macos"`
	Windows Windows `toml:"windows"`
	Linux   Linux   `toml:"linux"`
	Web     Web     `toml:"web"`

	// Dir is the directory holding giode.toml; paths in it are
	// relative to Dir.
	Dir string `toml:"-"`
}

// App is the app's identity, shared by every platform.
type App struct {
	// Name is the display name.
	Name string `toml:"name"`
	// ID is the reverse-DNS app ID (com.example.notes): the Android
	// package name and the Apple bundle ID.
	ID string `toml:"id"`
	// Version is major.minor.patch; Build is the build number (Android
	// versionCode, CFBundleVersion), which stores require to grow with
	// every upload.
	Version string `toml:"version"`
	Build   int    `toml:"build"`
	// Icon is a square PNG, at least 512x512 for the stores.
	Icon string `toml:"icon"`
	// Package is the main Go package, "." by default.
	Package string `toml:"package"`
	// Schemes are URL schemes the app opens (myapp://...).
	Schemes []string `toml:"schemes"`
	// Tags are Go build tags; Ldflags extra linker flags.
	Tags    string `toml:"tags"`
	Ldflags string `toml:"ldflags"`
	// Output is where builds go, "dist" by default.
	Output string `toml:"output"`
}

// Android configures Android builds.
type Android struct {
	MinSDK    int `toml:"min_sdk"`
	TargetSDK int `toml:"target_sdk"`
	// Arch limits the architectures built (arm, arm64, 386, amd64);
	// all four by default.
	Arch []string `toml:"arch"`
	// Permissions are added to the manifest: gogio's short names
	// (network, camera, microphone, storage, bluetooth, wakelock,
	// networkstate), giode's (notifications, foreground_service,
	// vibrate, biometric, location, coarse_location, contacts,
	// calendar, boot) or full names (android.permission.X).
	Permissions []string `toml:"permissions"`
	// KeepAlive declares giode's foreground service, so App.KeepAlive
	// keeps work running after the app leaves the screen. It adds the
	// service and its permissions.
	KeepAlive bool `toml:"keep_alive"`
	// Notifications adds POST_NOTIFICATIONS, which App.Notify needs on
	// Android 13 and later. True by default.
	Notifications *bool `toml:"notifications"`
	// Queries are package names the app may look up (Android 11 package
	// visibility).
	Queries []string `toml:"queries"`
	// Manifest is raw XML added inside <manifest>; Application raw XML
	// added inside <application>.
	Manifest    string `toml:"manifest"`
	Application string `toml:"application"`
	// Keystore signs release builds (giode build -release);
	// KeystorePassEnv names the environment variable holding its
	// password, GIODE_KEYSTORE_PASS by default. Debug builds use the
	// Android debug key.
	Keystore        string `toml:"keystore"`
	KeystorePassEnv string `toml:"keystore_pass_env"`
}

// IOS configures iOS builds.
type IOS struct {
	MinOS int `toml:"min_os"`
	// Usage holds the texts iOS shows when asking for a permission, by
	// short name: camera, microphone, speech, photos, photos_add,
	// location, location_always, contacts, calendar, reminders,
	// bluetooth, faceid, motion, health, health_update, tracking,
	// local_network. iOS ends an app that uses one without its text.
	Usage map[string]string `toml:"usage"`
	// Plist adds raw Info.plist keys: strings, booleans, integers and
	// lists of strings.
	Plist map[string]any `toml:"plist"`
	// Profile is the provisioning profile signing device builds.
	Profile string `toml:"profile"`
}

// MacOS configures macOS app bundles.
type MacOS struct {
	MinOS   int               `toml:"min_os"`
	Usage   map[string]string `toml:"usage"`
	Plist   map[string]any    `toml:"plist"`
	Profile string            `toml:"profile"`
	// Notarization, with the app-specific password read from
	// NotaryPassEnv (GIODE_NOTARY_PASS by default).
	NotaryID      string `toml:"notary_id"`
	NotaryTeamID  string `toml:"notary_team_id"`
	NotaryPassEnv string `toml:"notary_pass_env"`
}

// Windows configures Windows builds.
type Windows struct {
	// Arch is the GOARCH, amd64 by default.
	Arch string `toml:"arch"`
}

// Linux configures Linux builds.
type Linux struct {
	// Categories go in the .desktop file (freedesktop menu categories).
	Categories []string `toml:"categories"`
}

// Web configures WebAssembly builds.
type Web struct {
	// Port is where giode run web serves the app, 8080 by default.
	Port int `toml:"port"`
}

// Find looks for giode.toml in dir and its parents.
func Find(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		p := filepath.Join(dir, FileName)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no %s here or in a parent directory (create one with giode init)", FileName)
		}
		dir = parent
	}
}

// Load reads and checks a giode.toml file.
func Load(path string) (*Config, error) {
	var c Config
	md, err := toml.DecodeFile(path, &c)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if und := md.Undecoded(); len(und) > 0 {
		keys := make([]string, len(und))
		for i, k := range und {
			keys[i] = k.String()
		}
		return nil, fmt.Errorf("%s: unknown settings: %s", path, strings.Join(keys, ", "))
	}
	c.Dir = filepath.Dir(path)
	c.setDefaults()
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &c, nil
}

func (c *Config) setDefaults() {
	if c.App.Version == "" {
		c.App.Version = "1.0.0"
	}
	if c.App.Build == 0 {
		c.App.Build = 1
	}
	if c.App.Package == "" {
		c.App.Package = "."
	}
	if c.App.Output == "" {
		c.App.Output = "dist"
	}
	if c.Android.KeystorePassEnv == "" {
		c.Android.KeystorePassEnv = "GIODE_KEYSTORE_PASS"
	}
	if c.MacOS.NotaryPassEnv == "" {
		c.MacOS.NotaryPassEnv = "GIODE_NOTARY_PASS"
	}
	if c.Web.Port == 0 {
		c.Web.Port = 8080
	}
}

var (
	appIDRE   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z][A-Za-z0-9_]*)+$`)
	versionRE = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
)

// Validate reports settings that would fail a build.
func (c *Config) Validate() error {
	var errs []error
	if c.App.Name == "" {
		errs = append(errs, errors.New("app.name is required"))
	}
	if !appIDRE.MatchString(c.App.ID) {
		errs = append(errs, fmt.Errorf("app.id %q must be a reverse-DNS ID like com.example.notes (letters, digits and _, at least two parts)", c.App.ID))
	}
	if !versionRE.MatchString(c.App.Version) {
		errs = append(errs, fmt.Errorf("app.version %q must be major.minor.patch", c.App.Version))
	}
	if c.App.Build < 1 {
		errs = append(errs, fmt.Errorf("app.build must be at least 1"))
	}
	for _, p := range c.Android.Permissions {
		if _, err := androidPermission(p); err != nil {
			errs = append(errs, err)
		}
	}
	for _, u := range []map[string]string{c.IOS.Usage, c.MacOS.Usage} {
		for k := range u {
			if _, ok := usageKeys[k]; !ok {
				errs = append(errs, fmt.Errorf("unknown usage %q; known: %s", k, strings.Join(sortedKeys(usageKeys), ", ")))
			}
		}
	}
	for _, p := range []map[string]any{c.IOS.Plist, c.MacOS.Plist} {
		if _, err := plistXML(p); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Path resolves a path from giode.toml against its directory.
func (c *Config) Path(p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(c.Dir, p)
}

// GogioVersion is the app version as gogio takes it:
// major.minor.patch.build.
func (c *Config) GogioVersion() string {
	return fmt.Sprintf("%s.%d", c.App.Version, c.App.Build)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
