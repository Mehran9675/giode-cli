// Package native finds the shared libraries a cgo program loads that
// aren't the system's - libmpv and the dozens it brings (FFmpeg,
// libass, ...) - so packages can bundle them: next to a Windows
// executable, in a macOS app's Frameworks, in a Linux package's lib
// directory. It reads the binaries' imports itself (PE, ELF and Mach-O),
// so it works when cross-building.
package native

import (
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// Options configures Collect.
type Options struct {
	// GOOS is the binary's platform: windows, darwin or linux.
	GOOS string
	// Search are directories libraries are looked up in, before the
	// platform's own (Linux's library directories; @rpath entries on
	// macOS). Windows has no default: a cross-built program's DLLs are
	// wherever its toolchain put them (MSYS2's mingw64/bin, a MinGW
	// sysroot).
	Search []string
	// Libraries are libraries to bundle besides the imported ones
	// (plugins loaded with dlopen), with their own dependencies.
	Libraries []string
	// Exclude are names (globs: "libvulkan*") never bundled, besides the
	// system's.
	Exclude []string
}

// Lib is a library to bundle.
type Lib struct {
	// Name is the file name it's loaded by: the DLL name, the soname,
	// the install name's base name.
	Name string
	// Path is where it was found.
	Path string
	// Refs are the names binaries refer to it by (Mach-O install names,
	// rewritten when bundling).
	Refs []string
}

// Result is what Collect found.
type Result struct {
	Libs []Lib
	// Missing are imported libraries that aren't system ones and
	// weren't found in the search directories.
	Missing []string
}

// Collect returns the libraries bin needs, recursively, leaving out the
// system's.
func Collect(bin string, o Options) (Result, error) {
	c := &collector{o: o, seen: map[string]*Lib{}, missing: map[string]bool{}}
	if err := c.visit(bin, true); err != nil {
		return Result{}, err
	}
	for _, l := range o.Libraries {
		name := filepath.Base(l)
		if _, ok := c.seen[c.key(name)]; ok {
			continue
		}
		lib := &Lib{Name: name, Path: l}
		c.seen[c.key(name)] = lib
		c.order = append(c.order, c.key(name))
		if err := c.visit(l, false); err != nil {
			return Result{}, err
		}
	}
	var r Result
	for _, k := range c.order {
		r.Libs = append(r.Libs, *c.seen[k])
	}
	for m := range c.missing {
		r.Missing = append(r.Missing, m)
	}
	sort.Strings(r.Missing)
	return r, nil
}

type collector struct {
	o       Options
	seen    map[string]*Lib
	order   []string
	missing map[string]bool
	// rpaths are the Mach-O run paths of the binaries visited so far.
	rpaths []string
	exe    string
}

// key folds case for Windows, whose file names don't have it.
func (c *collector) key(name string) string {
	if c.o.GOOS == "windows" {
		return strings.ToLower(name)
	}
	return name
}

func (c *collector) visit(file string, main bool) error {
	imports, rpaths, err := c.imports(file)
	if err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(file), err)
	}
	if main {
		c.exe = file
	}
	c.rpaths = append(c.rpaths, rpaths...)
	for _, ref := range imports {
		name := path.Base(filepath.ToSlash(ref))
		if c.system(ref, name) || c.excluded(name) {
			continue
		}
		k := c.key(name)
		if l, ok := c.seen[k]; ok {
			if !contains(l.Refs, ref) {
				l.Refs = append(l.Refs, ref)
			}
			continue
		}
		p := c.find(ref, name, file)
		if p == "" {
			if !(c.o.GOOS == "windows" && (windowsRedist[strings.TrimSuffix(strings.ToLower(name), ".dll")] || inSystem32(name))) {
				c.missing[name] = true
			}
			continue
		}
		c.seen[k] = &Lib{Name: name, Path: p, Refs: []string{ref}}
		c.order = append(c.order, k)
		if err := c.visit(p, false); err != nil {
			return err
		}
	}
	return nil
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// imports returns the libraries file imports and, for Mach-O, its run
// paths.
func (c *collector) imports(file string) ([]string, []string, error) {
	switch c.o.GOOS {
	case "windows":
		f, err := pe.Open(file)
		if err != nil {
			return nil, nil, err
		}
		defer f.Close()
		return peImports(f), nil, nil
	case "linux", "freebsd":
		f, err := elf.Open(file)
		if err != nil {
			return nil, nil, err
		}
		defer f.Close()
		libs, err := f.ImportedLibraries()
		if err != nil {
			return nil, nil, err
		}
		var rpaths []string
		for _, tag := range []elf.DynTag{elf.DT_RUNPATH, elf.DT_RPATH} {
			if vs, err := f.DynString(tag); err == nil {
				for _, v := range vs {
					for _, p := range strings.Split(v, ":") {
						rpaths = append(rpaths, strings.ReplaceAll(p, "$ORIGIN", filepath.Dir(file)))
					}
				}
			}
		}
		return libs, rpaths, nil
	case "darwin":
		f, err := openMacho(file)
		if err != nil {
			return nil, nil, err
		}
		defer f.Close()
		libs, err := f.ImportedLibraries()
		if err != nil {
			return nil, nil, err
		}
		var rpaths []string
		for _, l := range f.Loads {
			if r, ok := l.(*macho.Rpath); ok {
				rpaths = append(rpaths, r.Path)
			}
		}
		return libs, rpaths, nil
	}
	return nil, nil, fmt.Errorf("native: no library bundling for %s", c.o.GOOS)
}

// openMacho opens a Mach-O file, or the first architecture of a
// universal one (they import the same libraries).
func openMacho(file string) (*macho.File, error) {
	f, err := macho.Open(file)
	if err == nil {
		return f, nil
	}
	ff, ferr := macho.OpenFat(file)
	if ferr != nil || len(ff.Arches) == 0 {
		return nil, err
	}
	return ff.Arches[0].File, nil
}

// find locates an imported library.
func (c *collector) find(ref, name, from string) string {
	var dirs []string
	switch c.o.GOOS {
	case "darwin":
		switch {
		case strings.HasPrefix(ref, "@rpath/"):
			rel := strings.TrimPrefix(ref, "@rpath/")
			for _, rp := range c.rpaths {
				rp = strings.ReplaceAll(rp, "@loader_path", filepath.Dir(from))
				rp = strings.ReplaceAll(rp, "@executable_path", filepath.Dir(c.exe))
				if p := filepath.Join(rp, rel); exists(p) {
					return p
				}
			}
		case strings.HasPrefix(ref, "@loader_path/"):
			if p := filepath.Join(filepath.Dir(from), strings.TrimPrefix(ref, "@loader_path/")); exists(p) {
				return p
			}
		case strings.HasPrefix(ref, "@executable_path/"):
			if p := filepath.Join(filepath.Dir(c.exe), strings.TrimPrefix(ref, "@executable_path/")); exists(p) {
				return p
			}
		case filepath.IsAbs(ref):
			if exists(ref) {
				return ref
			}
		}
		dirs = c.o.Search
	case "windows":
		dirs = c.o.Search
		if from != c.exe {
			// Windows looks beside a DLL for its imports. Not beside the
			// program, though: that's the output folder, holding the
			// last build's copies, which would win over the search
			// folders (and be copied onto themselves).
			dirs = append([]string{filepath.Dir(from)}, dirs...)
		}
	default:
		dirs = append(append(append([]string{}, c.o.Search...), c.rpaths...), linuxDirs...)
	}
	for _, d := range dirs {
		if p := lookup(d, name, c.o.GOOS == "windows"); p != "" {
			return p
		}
	}
	return ""
}

// lookup finds name in dir, ignoring case when fold is set.
func lookup(dir, name string, fold bool) string {
	if p := filepath.Join(dir, name); exists(p) {
		return p
	}
	if !fold {
		return ""
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if strings.EqualFold(e.Name(), name) {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

func exists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

var linuxDirs = []string{
	"/lib/x86_64-linux-gnu", "/usr/lib/x86_64-linux-gnu", "/lib/aarch64-linux-gnu", "/usr/lib/aarch64-linux-gnu",
	"/usr/local/lib", "/usr/lib64", "/lib64", "/usr/lib", "/lib",
}

func (c *collector) excluded(name string) bool {
	for _, g := range c.o.Exclude {
		if ok, _ := path.Match(strings.ToLower(g), strings.ToLower(name)); ok {
			return true
		}
	}
	return false
}

// system reports a library the platform provides.
func (c *collector) system(ref, name string) bool {
	lower := strings.ToLower(name)
	switch c.o.GOOS {
	case "windows":
		if strings.HasPrefix(lower, "api-ms-win-") || strings.HasPrefix(lower, "ext-ms-") {
			return true
		}
		return windowsSystem[strings.TrimSuffix(lower, ".dll")]
	case "darwin":
		return strings.HasPrefix(ref, "/usr/lib/") || strings.HasPrefix(ref, "/System/")
	default:
		for _, p := range linuxSystem {
			if ok, _ := path.Match(p, name); ok {
				return true
			}
		}
	}
	return false
}

// windowsSystem are DLLs Windows has (in System32), by lower-case name.
var windowsSystem = map[string]bool{}

func init() {
	for _, n := range strings.Fields(`advapi32 bcrypt cfgmgr32 comctl32 comdlg32 crypt32 d2d1 d3d9 d3d11
		d3d12 d3dcompiler_47 dbghelp dcomp dnsapi dsound dwmapi dwrite dxgi dxva2 gdi32 gdiplus hid imm32
		iphlpapi kernel32 kernelbase mf mfplat mfreadwrite mfuuid mmdevapi msimg32 msvcrt mswsock ncrypt
		netapi32 normaliz ntdll ole32 oleaut32 opengl32 powrprof propsys psapi rpcrt4 secur32 setupapi
		shcore shell32 shlwapi user32 userenv usp10 uxtheme version winhttp wininet winmm winspool ws2_32
		wsock32 wtsapi32 xinput1_4 avrt ucrtbase dinput8 dxcore mscms avicap32 avifil32 wldap32
		bcryptprimitives cryptbase cryptnet cabinet d3d10 d3d10_1 d3d10core d3d11on12 dhcpcsvc dinput
		evr glu32 hhctrl imagehlp ksuser mfcore mpr msacm32 msvfw32 netutils nsi ntmarta oleacc pdh
		profapi rasapi32 rstrtmgr sechost sspicli urlmon wer wevtapi windowscodecs winmmbase winsta
		wintrust winusb wlanapi wmvcore xinput9_1_0 dbgeng fwpuclnt mfsensorgroup cryptui comsvcs
		credui dpapi esent faultrep wkscli srvcli devobj
		vulkan-1 opencl nvcuda`) {
		// vulkan-1, opencl and nvcuda come with the graphics driver.
		windowsSystem[n] = true
	}
}

// inSystem32 reports whether name is in Windows' System32, when giode
// runs on Windows: a DLL of Windows' the list above doesn't name.
// It's consulted only for DLLs not found in the search folders, so a
// library some installer dropped into System32 is still bundled when
// the search folders have it.
func inSystem32(name string) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return exists(filepath.Join(root, "System32", name))
}

// windowsRedist are Microsoft's C++ runtime DLLs: on most Windows PCs
// but not all, so they're bundled when found in the search directories,
// and not reported when they aren't.
var windowsRedist = map[string]bool{
	"vcruntime140": true, "vcruntime140_1": true, "msvcp140": true, "msvcp140_1": true,
	"msvcp140_2": true, "concrt140": true, "vcomp140": true,
}

// linuxSystem are libraries a Linux desktop has and a package mustn't
// bundle: the C library, and the graphics, windowing and sound stacks
// that go with the system's drivers and servers (as AppImage's
// excludelist).
var linuxSystem = []string{
	"libc.so.*", "libm.so.*", "libdl.so.*", "libpthread.so.*", "librt.so.*", "libresolv.so.*", "libutil.so.*",
	"ld-linux*", "libgcc_s.so.*", "libstdc++.so.*", "libanl.so.*",
	"libGL.so.*", "libEGL.so.*", "libGLX*", "libGLdispatch.so.*", "libGLESv2.so.*", "libOpenGL.so.*",
	"libvulkan.so.*", "libdrm.so.*", "libgbm.so.*", "libva*.so.*", "libvdpau.so.*",
	"libX11.so.*", "libX11-xcb.so.*", "libxcb*.so.*", "libXext.so.*", "libXrender.so.*", "libXfixes.so.*",
	"libXcursor.so.*", "libXi.so.*", "libXrandr.so.*", "libXinerama.so.*", "libXss.so.*", "libXau.so.*", "libXdmcp.so.*",
	"libxkbcommon*.so.*", "libwayland-*.so.*",
	"libasound.so.*", "libpulse*.so.*", "libpipewire*.so.*", "libjack.so.*",
	"libfontconfig.so.*", "libfreetype.so.*", "libdbus-1.so.*", "libsystemd.so.*", "libudev.so.*",
	"libexpat.so.*", "libz.so.*", "libuuid.so.*",
}

// MacPlan is what bundling a macOS app's libraries does: each library
// copied into Contents/Frameworks, renamed @rpath/<name>, and every
// reference to it rewritten.
type MacPlan struct {
	// Copies maps a library's path to its name in Frameworks.
	Copies map[string]string
	// Changes maps an install name binaries use to its new one.
	Changes map[string]string
}

// PlanMac returns the rewrite plan for libs.
func PlanMac(libs []Lib) MacPlan {
	p := MacPlan{Copies: map[string]string{}, Changes: map[string]string{}}
	for _, l := range libs {
		p.Copies[l.Path] = l.Name
		for _, r := range l.Refs {
			p.Changes[r] = "@rpath/" + l.Name
		}
	}
	return p
}

// InstallNameArgs returns install_name_tool's arguments for a file in
// the bundle: id its new install name for a library ("" for the
// executable), with every reference in changes rewritten.
func (p MacPlan) InstallNameArgs(id string, file string) []string {
	var args []string
	if id != "" {
		args = append(args, "-id", "@rpath/"+id)
	}
	refs := make([]string, 0, len(p.Changes))
	for r := range p.Changes {
		refs = append(refs, r)
	}
	sort.Strings(refs)
	for _, r := range refs {
		args = append(args, "-change", r, p.Changes[r])
	}
	return append(args, file)
}

// ErrNotFound is wrapped by errors for missing libraries.
var ErrNotFound = errors.New("not found")
