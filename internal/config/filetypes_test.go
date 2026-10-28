package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileTypes(t *testing.T) {
	c := &Config{App: App{ID: "com.example.player"}, FileTypes: []FileType{
		{Name: "MKV video", Extensions: []string{"mkv", "mk3d"}, MIMETypes: []string{"video/x-matroska"}},
		{Name: "MP4 video", Extensions: []string{"mp4"}, MIMETypes: []string{"video/mp4"}, UTI: "public.mpeg-4"},
		{Name: "Player playlist", Extensions: []string{"gpl"}, Role: "editor"},
	}}
	plist, err := c.MacOSPlist()
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{
		"<key>CFBundleDocumentTypes</key>",
		"<string>com.example.player.mkv</string>",
		"<string>public.mpeg-4</string>",
		"<key>UTImportedTypeDeclarations</key>",
		"<string>public.movie</string>",
		"<string>Editor</string>",
	} {
		if !strings.Contains(plist, w) {
			t.Errorf("plist lacks %s:\n%s", w, plist)
		}
	}
	if strings.Count(plist, "<key>UTTypeIdentifier</key>") != 2 {
		t.Errorf("declares a type the system knows (public.mpeg-4):\n%s", plist)
	}

	types, xml := c.LinuxMIMETypes("player")
	if strings.Join(types, ";") != "video/x-matroska;video/mp4;application/x-player-gpl" {
		t.Errorf("mime types %v", types)
	}
	if !strings.Contains(xml, `<mime-type type="application/x-player-gpl">`) || !strings.Contains(xml, `<glob pattern="*.gpl"/>`) || strings.Contains(xml, "matroska") {
		t.Errorf("mime xml:\n%s", xml)
	}

	w := c.WindowsFileTypes()
	if len(w) != 3 || w[0].ProgID != "com.example.player.mkv" || strings.Join(w[0].Extensions, ",") != ".mk3d,.mkv" {
		t.Errorf("windows %+v", w)
	}

	c.FileTypes = append(c.FileTypes, FileType{Name: "bad", Extensions: []string{".dot"}})
	if errs := c.validateFileTypes(); len(errs) != 1 {
		t.Errorf("validate: %v", errs)
	}
}

func TestNativePaths(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"angle/bin/libEGL.dll", "angle/bin/libGLESv2.dll", "msys/mingw64/bin/zlib1.dll"} {
		p := filepath.Join(dir, filepath.FromSlash(f))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, nil, 0o644)
	}
	t.Setenv("ANGLE_DIR", filepath.Join(dir, "angle"))
	t.Setenv("MSYSTEM_PREFIX", filepath.Join(dir, "msys", "mingw64"))
	c := &Config{Dir: dir, Native: Native{Windows: NativePlatform{
		Search:    []string{"$MSYSTEM_PREFIX/bin", "msys/*/bin", "nowhere/*"},
		Libraries: []string{"${ANGLE_DIR}/bin/lib*.dll", "%MSYSTEM_PREFIX%/bin/zlib1.dll"},
	}}}
	search, libs, _, err := c.NativeOptions("windows")
	if err != nil {
		t.Fatal(err)
	}
	if len(search) != 2 || len(libs) != 3 {
		t.Errorf("search %v\nlibs %v", search, libs)
	}
	c.Native.Windows.Libraries = []string{"$NOT_SET_ANYWHERE/libmpv-2.dll"}
	if _, _, _, err := c.NativeOptions("windows"); err == nil || !strings.Contains(err.Error(), "libmpv-2.dll") {
		t.Errorf("missing library: %v", err)
	}
}
