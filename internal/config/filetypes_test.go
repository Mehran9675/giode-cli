package config

import (
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
