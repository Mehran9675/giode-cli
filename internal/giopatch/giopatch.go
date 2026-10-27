// Package giopatch builds apps against a patched copy of Gio, for fixes
// Gio hasn't released yet. The copy lives in the user cache directory;
// the app's go.mod is left alone: the build gets a temporary copy of it
// with a replace directive, through GOFLAGS=-modfile.
package giopatch

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// patch replaces old with new in a file of the Gio module.
type patch struct {
	file, old, new string
}

// revision changes whenever the patches do, so cached copies made with
// older patches aren't used.
const revision = "1"

// androidPacing makes Android render at the display's refresh rate.
// Gio asks the Choreographer for the next frame only after presenting
// the current one; presenting blocks until a vsync, so the request
// misses the next one and a 120Hz display gets 60 frames a second.
// Asking before drawing catches it. A callback that arrives once the
// animation stopped is dropped instead of drawing a frame nobody asked
// for.
var androidPacing = []patch{
	{
		file: "app/os_android.go",
		old: `func (w *window) draw(env *C.JNIEnv, sync bool) {
	if !w.visible {
		return
	}
`,
		new: `func (w *window) draw(env *C.JNIEnv, sync bool) {
	if !w.visible {
		return
	}
	// giode: ask for the next frame before drawing this one, so the
	// request makes the next vsync.
	if !sync && !w.animating {
		return
	}
	wasAnimating := w.animating
	if wasAnimating {
		callVoidMethod(env, w.view, gioView.postFrameCallback)
	}
`,
	},
	{
		file: "app/os_android.go",
		old: `		Sync: sync,
	})
	if w.animating {
		callVoidMethod(env, w.view, gioView.postFrameCallback)
	}
`,
		new: `		Sync: sync,
	})
	if w.animating && !wasAnimating {
		callVoidMethod(env, w.view, gioView.postFrameCallback)
	}
`,
	},
}

// Android is the set of patches for Android builds.
var Android = androidPacing

// ErrSkipped is returned, wrapped, when a module can't be patched:
// it replaces or vendors Gio itself, or uses a workspace.
var ErrSkipped = errors.New("not patched")

type module struct {
	Path, Version, Dir string
	Replace            *module
}

// Prepare makes a patched copy of the Gio module the app in dir uses
// and returns the -modfile to build with, and a cleanup function
// removing it.
func Prepare(dir string, patches []patch) (modfile string, cleanup func(), err error) {
	if _, err := os.Stat(filepath.Join(dir, "vendor")); err == nil {
		return "", nil, fmt.Errorf("%w: the module vendors its dependencies", ErrSkipped)
	}
	if out, err := goCmd(dir, "env", "GOWORK"); err == nil && out != "" && out != "off" {
		return "", nil, fmt.Errorf("%w: the module is in a workspace (%s)", ErrSkipped, out)
	}
	out, err := goCmd(dir, "list", "-m", "-json", "gioui.org")
	if err != nil {
		return "", nil, err
	}
	var m module
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		return "", nil, fmt.Errorf("reading go list output: %w", err)
	}
	if m.Replace != nil {
		return "", nil, fmt.Errorf("%w: go.mod replaces gioui.org", ErrSkipped)
	}
	if m.Dir == "" {
		if _, err := goCmd(dir, "mod", "download", "gioui.org"); err != nil {
			return "", nil, err
		}
		if out, err = goCmd(dir, "list", "-m", "-json", "gioui.org"); err != nil {
			return "", nil, err
		}
		if err := json.Unmarshal([]byte(out), &m); err != nil || m.Dir == "" {
			return "", nil, errors.New("can't find the Gio module's source")
		}
	}
	patched, err := patchedCopy(m, patches)
	if err != nil {
		return "", nil, err
	}

	tmp, err := os.MkdirTemp("", "giode-mod-")
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { os.RemoveAll(tmp) }
	gomod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		cleanup()
		return "", nil, err
	}
	gomod = append(gomod, fmt.Sprintf("\n// Added by giode for this build.\nreplace gioui.org => %s\n", quote(patched))...)
	modfile = filepath.Join(tmp, "go.mod")
	if err := os.WriteFile(modfile, gomod, 0o644); err != nil {
		cleanup()
		return "", nil, err
	}
	if sum, err := os.ReadFile(filepath.Join(dir, "go.sum")); err == nil {
		if err := os.WriteFile(filepath.Join(tmp, "go.sum"), sum, 0o644); err != nil {
			cleanup()
			return "", nil, err
		}
	}
	return modfile, cleanup, nil
}

// patchedCopy returns the directory of a patched copy of m, making it
// the first time.
func patchedCopy(m module, patches []patch) (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dst := filepath.Join(cache, "giode", "gio", m.Version+"-giode"+revision)
	if _, err := os.Stat(filepath.Join(dst, "go.mod")); err == nil {
		return dst, nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(dst), "tmp-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	if err := copyTree(m.Dir, tmp); err != nil {
		return "", err
	}
	if err := apply(tmp, patches); err != nil {
		return "", fmt.Errorf("Gio %s: %w", m.Version, err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		if _, serr := os.Stat(filepath.Join(dst, "go.mod")); serr == nil {
			return dst, nil // another build made it first
		}
		return "", err
	}
	return dst, nil
}

// apply applies patches to the module in dir.
func apply(dir string, patches []patch) error {
	for _, p := range patches {
		file := filepath.Join(dir, filepath.FromSlash(p.file))
		b, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		s := string(b)
		if strings.Count(s, p.old) != 1 {
			return fmt.Errorf("%w: %s doesn't have the code the patch expects", ErrSkipped, p.file)
		}
		s = strings.Replace(s, p.old, p.new, 1)
		if err := os.WriteFile(file, []byte(s), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// copyTree copies the files of src into dst, writable (the module
// cache is read-only).
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
}

func quote(path string) string {
	if strings.ContainsAny(path, " \t\"'`") {
		return fmt.Sprintf("%q", path)
	}
	return path
}

func goCmd(dir string, args ...string) (string, error) {
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("go %s: %s", strings.Join(args, " "), strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("go %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// WithModfile runs fn with GOFLAGS set to build with modfile, then
// restores GOFLAGS.
func WithModfile(modfile string, fn func() error) error {
	old, had := os.LookupEnv("GOFLAGS")
	flag := "-modfile=" + modfile
	if strings.ContainsAny(flag, " \t") {
		// GOFLAGS splits at spaces outside quotes.
		flag = "'" + flag + "'"
	}
	flags := strings.TrimSpace(old + " " + flag)
	os.Setenv("GOFLAGS", flags)
	defer func() {
		if had {
			os.Setenv("GOFLAGS", old)
		} else {
			os.Unsetenv("GOFLAGS")
		}
	}()
	return fn()
}
