// Command giode creates, builds and runs giode apps on desktop, mobile
// and the web.
//
//	giode new notes             # a new project from a template
//	giode run                   # run on this computer
//	giode run android           # build, install and run on a device
//	giode build android windows # packages in dist/
//	giode doctor                # check the toolchains
//
// Builds read giode.toml, the project file, for the app's name, ID,
// version, icon and platform settings - including the Android manifest
// and Info.plist entries gogio can't add.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"sort"
	"strings"
)

// subcommand is a giode command.
type subcommand struct {
	name    string
	summary string
	usage   string
	run     func(args []string) error
}

var commands []subcommand

func init() {
	commands = []subcommand{
		{"new", "create a project from a template", newUsage, cmdNew},
		{"init", "add giode.toml to an existing project", initUsage, cmdInit},
		{"build", "build packages for one or more platforms", buildUsage, cmdBuild},
		{"run", "build and run on this computer, a device, a simulator or the browser", runUsage, cmdRun},
		{"devices", "list connected Android devices and iOS simulators", devicesUsage, cmdDevices},
		{"doctor", "check Go, the Android SDK and NDK, Java, Xcode and system libraries", doctorUsage, cmdDoctor},
		{"keystore", "create an Android release keystore", keystoreUsage, cmdKeystore},
		{"cert", "create a self-signed Windows code signing certificate for testing", certUsage, cmdCert},
		{"sign", "sign Windows executables and installers", signUsage, cmdSign},
		{"links", "write the files a website serves for the app's universal links and App Links", linksUsage, cmdLinks},
		{"update-key", "create the key pair that signs update feeds", updateKeyUsage, cmdUpdateKey},
		{"release", "build desktop targets and write a signed update feed", releaseUsage, cmdRelease},
		{"upgrade", "update the project to the latest giode", upgradeUsage, cmdUpgrade},
		{"manifest", "print the Android manifest and Info.plist additions from giode.toml", manifestUsage, cmdManifest},
		{"clean", "remove build output", cleanUsage, cmdClean},
		{"self-update", "update the giode CLI to the latest release", selfUpdateUsage, cmdSelfUpdate},
		{"version", "print the giode CLI version", "giode version", cmdVersion},
	}
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	name, args := os.Args[1], os.Args[2:]
	if name == "help" || name == "-h" || name == "--help" {
		if len(args) > 0 {
			if c, ok := find(args[0]); ok {
				fmt.Println(strings.TrimSpace(c.usage))
				return
			}
		}
		usage()
		return
	}
	c, ok := find(name)
	if !ok {
		fmt.Fprintf(os.Stderr, "giode: unknown command %q\n\n", name)
		usage()
		os.Exit(2)
	}
	if err := c.run(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintf(os.Stderr, "giode %s: %v\n", c.name, err)
		os.Exit(1)
	}
}

func find(name string) (subcommand, bool) {
	for _, c := range commands {
		if c.name == name {
			return c, true
		}
	}
	return subcommand{}, false
}

func usage() {
	fmt.Fprintln(os.Stderr, "giode creates, builds and runs giode apps.\n\nUsage:\n\n\tgiode <command> [arguments]\n\nCommands:")
	for _, c := range commands {
		fmt.Fprintf(os.Stderr, "\t%-12s %s\n", c.name, c.summary)
	}
	fmt.Fprintln(os.Stderr, "\nRun giode help <command> for a command's options.")
}

// flags returns a flag set for a command that prints its usage text.
func flags(name, usageText string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), strings.TrimSpace(usageText))
		fmt.Fprintln(fs.Output(), "\nOptions:")
		fs.PrintDefaults()
	}
	return fs
}

// parse parses args, allowing flags after positional arguments
// (giode build android -release).
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		if args[0] == "--" {
			return append(pos, args[1:]...), nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

// version is set by release builds (-ldflags "-X main.version=v0.2.0").
var version string

// currentVersion is the release, or the module version go install
// recorded.
func currentVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		return bi.Main.Version
	}
	return "(devel)"
}

func cmdVersion(args []string) error {
	fmt.Println("giode", currentVersion())
	return nil
}

func sortedNames[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
