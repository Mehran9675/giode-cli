// Package giopatch builds apps against a patched copy of Gio, for fixes
// and hooks Gio hasn't released: on Android, rendering at the display's
// refresh rate and keeping held keys repeating; everywhere, the hook
// behind giode.App's Config.Underlay. The copy lives in the user cache directory;
// the app's go.mod is left alone: the build gets a temporary copy of it
// with a replace directive, passed with -modfile to the go build and go
// list commands run in the app's module. (Not through GOFLAGS: go
// refuses -modfile in commands that run outside module mode.)
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

// patch replaces old with new in a file of the Gio module, or, with
// old empty, adds the file with new as its content.
type patch struct {
	file, old, new string
}

// revision changes whenever the patches do, so cached copies made with
// older patches aren't used.
const revision = "4"

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

// androidKeyRepeat keeps a held key repeating. After every edit Gio
// compares the snippet of text the keyboard was given with the
// editor's, and restarts the keyboard's input connection when they
// differ - which they do after any edit, because the editor moves its
// snippet window along with the text. Restarting cancels the
// keyboard's key repeat, so holding backspace deleted one character.
// The keyboard reads text on demand, so it only needs a restart when
// the text changed under it: unless one snippet lies within the other
// with the same text (the editor just moved its window).
var androidKeyRepeat = []patch{
	{
		file: "app/os_android.go",
		old: `		if old.Snippet != new.Snippet {
			callVoidMethod(env, w.view, gioView.restartInput)
			return
		}
`,
		new: `		if old.Snippet != new.Snippet && !snippetsAgree(old.Snippet, new.Snippet) {
			// giode: only when the text changed under the keyboard;
			// a restart cancels its key repeat.
			callVoidMethod(env, w.view, gioView.restartInput)
			return
		}
`,
	},
	{
		file: "app/os_android.go",
		old: `func (w *window) ShowTextInput(show bool) {
`,
		new: `// snippetsAgree reports whether one snippet of the editor's text lies
// within the other with the same text: the editor only moved its
// window over text the keyboard already knows (giode).
func snippetsAgree(a, b key.Snippet) bool {
	ar, br := []rune(a.Text), []rune(b.Text)
	if (len(ar) == 0) != (len(br) == 0) {
		return false
	}
	as, bs := min(a.Start, a.End), min(b.Start, b.End)
	start, end := max(as, bs), min(as+len(ar), bs+len(br))
	if end-start != min(len(ar), len(br)) {
		return false
	}
	return string(ar[start-as:end-as]) == string(br[start-bs:end-bs])
}

func (w *window) ShowTextInput(show bool) {
`,
	},
}

// underlay lets giode.App draw caller-rendered GPU content (video, 3D)
// under the UI: the window calls a hook with the frame's render target
// before drawing the UI over it without clearing, with Gio's OpenGL
// backend restoring the GL state it changes, and can be told to prefer
// OpenGL where it would pick Direct3D. giode uses app.GiodeSetUnderlay
// behind the giode_gio build tag, which builds against this copy set.
var underlay = []patch{
	{
		file: "app/window.go",
		old: `func (w *Window) frame(frame *op.Ops, viewport image.Point) error {
`,
		new: `func (w *Window) frame(frame *op.Ops, viewport image.Point) error {
	// giode: an underlay draws first, then the UI over it, uncleared.
	if f := giodeUnderlay; f != nil {
		target, err := w.ctx.RenderTarget()
		if err != nil {
			return err
		}
		f(giodeTarget(target, viewport))
		return w.gpu.Frame(frame, target, viewport)
	}
`,
	},
	{
		file: "app/window.go",
		old:  `gpu, err := gpu.New(w.ctx.API())`,
		new:  `gpu, err := gpu.New(giodeAPI(w.ctx.API()))`,
	},
	{
		file: "app/d3d11_windows.go",
		old: `		initializer: func(w *window) (context, error) {
			hwnd, _, _ := w.HWND()
`,
		new: `		initializer: func(w *window) (context, error) {
			if giodeForceGL {
				return nil, fmt.Errorf("giode: OpenGL requested")
			}
			hwnd, _, _ := w.HWND()
`,
	},
	{
		file: "gpu/internal/opengl/opengl.go",
		old: `	savedState glState
	sharedCtx  bool
`,
		new: `	savedState glState
	sharedCtx  bool
	// giode: the capabilities foreign GL code may leave on that Gio
	// assumes are off, and their state before the frame.
	giodeCaps [4]bool
`,
	},
	{
		file: "gpu/internal/opengl/opengl.go",
		old: `	if b.sharedCtx {
		b.glstate = b.queryState()
		b.savedState = b.glstate
	}
`,
		new: `	if b.sharedCtx {
		b.glstate = b.queryState()
		b.savedState = b.glstate
		for i, c := range giodeCapEnums {
			b.giodeCaps[i] = b.funcs.IsEnabled(c)
			if b.giodeCaps[i] {
				b.funcs.Disable(c)
			}
		}
	}
`,
	},
	{
		file: "gpu/internal/opengl/opengl.go",
		old: `	if b.sharedCtx {
		b.restoreState(b.savedState)
	}`,
		new: `	if b.sharedCtx {
		b.restoreState(b.savedState)
		for i, c := range giodeCapEnums {
			if b.giodeCaps[i] {
				b.funcs.Enable(c)
			}
		}
	}`,
	},
	{
		file: "gpu/internal/opengl/giode_caps.go",
		new: `// Added by giode's Gio patches.

package opengl

import "gioui.org/internal/gl"

// giodeCapEnums are GL_SCISSOR_TEST, GL_DEPTH_TEST, GL_STENCIL_TEST
// and GL_CULL_FACE: Gio never enables them, so a shared context's
// other user leaving one on would clip or drop Gio's drawing.
var giodeCapEnums = [4]gl.Enum{0x0C11, 0x0B71, 0x0B90, 0x0B44}
`,
	},
	{
		file: "app/giode_underlay.go",
		new: `// Added by giode's Gio patches: the hooks behind giode.App's
// Config.Underlay and Config.Renderer.

package app

import "gioui.org/gpu"

// GiodeTarget describes a frame's render target to an underlay.
type GiodeTarget struct {
	// API is 1 for OpenGL (ES), 2 Metal, 3 Direct3D 11, 4 Vulkan.
	API int
	// Framebuffer is the OpenGL framebuffer object, 0 for the window's.
	Framebuffer uint32
	// Handle is the MTLTexture, the ID3D11RenderTargetView or the VkImage.
	Handle        uintptr
	Width, Height int
}

var (
	giodeUnderlay func(GiodeTarget)
	giodeForceGL  bool
)

// GiodeSetUnderlay sets the function drawing under the UI each frame,
// with the GPU context current, and whether to prefer OpenGL over
// Direct3D. Call it before creating windows.
func GiodeSetUnderlay(f func(GiodeTarget), forceGL bool) {
	giodeUnderlay, giodeForceGL = f, forceGL
}

// giodeAPI makes Gio's OpenGL backend save and restore the GL state
// around its frames when an underlay shares the context.
func giodeAPI(api gpu.API) gpu.API {
	if gl, ok := api.(gpu.OpenGL); ok && giodeUnderlay != nil {
		gl.Shared = true
		return gl
	}
	return api
}
`,
	},
	{
		file: "app/giode_target.go",
		new: `//go:build !js

// Added by giode's Gio patches.

package app

import (
	"image"
	"unsafe"

	"gioui.org/gpu"
)

func giodeTarget(t gpu.RenderTarget, size image.Point) GiodeTarget {
	g := GiodeTarget{Width: size.X, Height: size.Y}
	switch t := t.(type) {
	case gpu.OpenGLRenderTarget:
		g.API, g.Framebuffer = 1, uint32(t.V)
	case gpu.MetalRenderTarget:
		g.API, g.Handle = 2, t.Texture
	case gpu.Direct3D11RenderTarget:
		g.API, g.Handle = 3, uintptr(unsafe.Pointer(t.RenderTarget))
	case gpu.VulkanRenderTarget:
		g.API, g.Handle = 4, uintptr(t.Image)
	}
	return g
}
`,
	},
	{
		file: "app/giode_target_js.go",
		new: `// Added by giode's Gio patches.

package app

import (
	"image"

	"gioui.org/gpu"
)

func giodeTarget(t gpu.RenderTarget, size image.Point) GiodeTarget {
	return GiodeTarget{API: 1, Width: size.X, Height: size.Y}
}
`,
	},
}

// Android is the set of patches for Android builds.
var Android = append(append([]patch{}, androidPacing...), androidKeyRepeat...)

// All is every patch, for builds of any target: the Android ones only
// touch Android's files. Builds against it have the giode_gio tag (see
// Tag).
var All = append(append([]patch{}, Android...), underlay...)

// Tag is the build tag of builds against patched Gio, which giode uses
// to reach the patches' hooks.
const Tag = "giode_gio"

// ErrSkipped is returned, wrapped, when a module can't be patched:
// it replaces or vendors Gio itself, or uses a workspace.
var ErrSkipped = errors.New("not patched")

type module struct {
	Path, Version, Dir string
	Replace            *module
}

// Prepare makes a patched copy of the Gio module the app in dir uses
// and returns the go.mod to build with (go build -modfile), and a
// cleanup function removing it.
func Prepare(dir string, patches []patch) (modfile string, cleanup func(), err error) {
	if _, err := os.Stat(filepath.Join(dir, "vendor")); err == nil {
		return "", nil, fmt.Errorf("%w: the module vendors its dependencies", ErrSkipped)
	}
	if out, err := goCmd(dir, "env", "GOWORK"); err == nil && out != "" && out != "off" {
		return "", nil, fmt.Errorf("%w: the module is in a workspace (%s)", ErrSkipped, out)
	}
	gomodPath, err := goCmd(dir, "env", "GOMOD")
	if err != nil {
		return "", nil, err
	}
	if gomodPath == "" || gomodPath == os.DevNull {
		return "", nil, fmt.Errorf("%w: %s isn't built in module mode", ErrSkipped, dir)
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
	// The module root may be above dir, where giode.toml is.
	gomod, err := os.ReadFile(gomodPath)
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
	if sum, err := os.ReadFile(filepath.Join(filepath.Dir(gomodPath), "go.sum")); err == nil {
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
		if p.old == "" {
			if _, err := os.Stat(file); err == nil {
				return fmt.Errorf("%w: %s exists already", ErrSkipped, p.file)
			}
			if err := os.WriteFile(file, []byte(p.new), 0o644); err != nil {
				return err
			}
			continue
		}
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
