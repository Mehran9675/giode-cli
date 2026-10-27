package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/mehran9675/giode-cli/internal/config"
)

const runUsage = `
Usage: giode run [options] [target] [-- app arguments]

Builds and runs the app in giode.toml:

	desktop   on this computer, with go run (default)
	android   on a connected device or emulator: builds for its
	          architecture, installs, launches and shows the app's log
	ios-sim   in the booted iOS simulator, showing its output (macOS)
	web       serves dist/web and prints the address to open

Options:

	-device   the Android device serial or simulator UDID (giode devices)
	-mcp      start giode's MCP server so an AI agent can drive the app

Example:

	giode run android
	giode run -mcp
`

func cmdRun(args []string) error {
	fs := flags("run", runUsage)
	dir := fs.String("C", ".", "project directory")
	device := fs.String("device", "", "Android device serial or simulator UDID")
	mcp := fs.Bool("mcp", false, "start giode's MCP server (GIODE_MCP=1)")
	release := fs.Bool("release", false, "run a release-signed Android build")
	fs.BoolVar(&verbose, "x", false, "print the commands run")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	target, appArgs := "desktop", []string(nil)
	if len(pos) > 0 {
		target, appArgs = pos[0], pos[1:]
	}
	c, err := loadProject(*dir)
	if err != nil {
		return err
	}
	if *mcp {
		os.Setenv("GIODE_MCP", "1")
	}
	switch target {
	case "desktop", "linux", "windows", "macos":
		if target != "desktop" && target != hostTarget() {
			return fmt.Errorf("%s apps run on %s; build one with giode build %s", target, target, target)
		}
		cmd := command(c, "go", append([]string{"run", "-tags", c.App.Tags, pkgPath(c)}, appArgs...)...)
		return run(cmd)
	case "android":
		return runAndroid(c, *device, buildOpts{release: *release, args: appArgs})
	case "ios-sim", "ios":
		return runIOSSim(c, *device, appArgs)
	case "web":
		return runWeb(c)
	}
	return fmt.Errorf("unknown target %q (desktop, android, ios-sim, web)", target)
}

// abiArch maps Android ABIs to GOARCHs.
var abiArch = map[string]string{
	"arm64-v8a":   "arm64",
	"armeabi-v7a": "arm",
	"x86_64":      "amd64",
	"x86":         "386",
}

func runAndroid(c *config.Config, device string, o buildOpts) error {
	adbPath, err := adb()
	if err != nil {
		return err
	}
	if device != "" {
		os.Setenv("ANDROID_SERIAL", device)
	}
	if device == "" {
		devs, err := androidDevices(adbPath)
		if err != nil {
			return err
		}
		switch len(devs) {
		case 0:
			return errors.New("no Android device is connected: plug one in with USB debugging on, or start an emulator")
		case 1:
		default:
			return fmt.Errorf("several Android devices are connected; pick one with -device (%s)", strings.Join(devs, ", "))
		}
	}
	// Build only the device's architecture: a quarter of the work.
	abi, err := output(adbPath, "shell", "getprop", "ro.product.cpu.abi")
	if err != nil {
		return err
	}
	o.arch = abiArch[abi]
	if o.arch == "" {
		return fmt.Errorf("unknown device ABI %q", abi)
	}
	fmt.Printf("Building for %s (%s)…\n", abi, o.arch)
	apk, err := buildTarget(c, "android", o)
	if err != nil {
		return err
	}
	fmt.Println("Installing…")
	if err := run(command(c, adbPath, "install", "-r", "-d", apk)); err != nil {
		return err
	}
	// Clear old log lines, start the app and follow its log.
	run(command(c, adbPath, "logcat", "-c"))
	if err := run(command(c, adbPath, "shell", "monkey", "-p", c.App.ID, "-c", "android.intent.category.LAUNCHER", "1")); err != nil {
		return err
	}
	var pid string
	for i := 0; i < 50 && pid == ""; i++ {
		pid, _ = output(adbPath, "shell", "pidof", c.App.ID)
		if pid == "" {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if pid == "" {
		return errors.New("the app didn't start; check adb logcat for a crash")
	}
	fmt.Printf("Running %s (pid %s). Press Ctrl+C to stop following the log.\n\n", c.App.ID, pid)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	cmd := exec.CommandContext(ctx, adbPath, "logcat", "-v", "brief", "--pid="+strings.Fields(pid)[0])
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}

// androidDevices lists the serials of connected, authorized devices.
func androidDevices(adbPath string) ([]string, error) {
	out, err := output(adbPath, "devices")
	if err != nil {
		return nil, err
	}
	var devs []string
	for _, line := range strings.Split(out, "\n")[1:] {
		f := strings.Fields(line)
		if len(f) >= 2 && f[1] == "device" {
			devs = append(devs, f[0])
		}
	}
	return devs, nil
}

func runIOSSim(c *config.Config, udid string, appArgs []string) error {
	if runtime.GOOS != "darwin" {
		return errors.New("the iOS simulator needs macOS and Xcode")
	}
	if udid == "" {
		udid = "booted"
		if out, _ := output("xcrun", "simctl", "list", "devices", "booted"); !strings.Contains(out, "Booted") {
			return errors.New("no simulator is booted: open Simulator.app, or run xcrun simctl boot <device>")
		}
	}
	app, err := buildTarget(c, "ios-sim", buildOpts{})
	if err != nil {
		return err
	}
	if err := run(command(c, "xcrun", "simctl", "install", udid, app)); err != nil {
		return err
	}
	args := append([]string{"simctl", "launch", "--console-pty", "--terminate-running-process", udid, c.App.ID}, appArgs...)
	return run(command(c, "xcrun", args...))
}

func runWeb(c *config.Config) error {
	d, err := buildTarget(c, "web", buildOpts{})
	if err != nil {
		return err
	}
	addr := "127.0.0.1:" + strconv.Itoa(c.Web.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("serving on %s: %w (set web.port in giode.toml)", addr, err)
	}
	fmt.Printf("Serving %s at http://%s/ - press Ctrl+C to stop.\n", relPath(d), addr)
	srv := &http.Server{Handler: noCache(http.FileServer(http.Dir(filepath.Clean(d))))}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		srv.Close()
	}()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// noCache makes browsers fetch a rebuilt main.wasm.
func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}
