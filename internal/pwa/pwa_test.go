package pwa

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMake(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html><head><title>X</title></head><body></body></html>"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.wasm"), []byte("wasm"), 0o644)
	icon := filepath.Join(t.TempDir(), "icon.png")
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for i := range img.Pix {
		img.Pix[i] = 0x80
	}
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	f, _ := os.Create(icon)
	png.Encode(f, img)
	f.Close()
	if err := Make(App{Name: "Notes", ID: "com.example.notes", Icon: icon, ThemeColor: "#112233"}, dir); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	b, _ := os.ReadFile(filepath.Join(dir, "manifest.webmanifest"))
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["name"] != "Notes" || m["display"] != "standalone" || m["background_color"] != "#112233" || len(m["icons"].([]any)) != 3 {
		t.Errorf("manifest %v", m)
	}
	idx, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	for _, w := range []string{`<link rel="manifest" href="manifest.webmanifest">`, `serviceWorker.register("sw.js")`, `content="#112233"`, "apple-touch-icon.png"} {
		if !strings.Contains(string(idx), w) {
			t.Errorf("index lacks %s:\n%s", w, idx)
		}
	}
	sw, _ := os.ReadFile(filepath.Join(dir, "sw.js"))
	if !strings.Contains(string(sw), `"main.wasm"`) || !strings.Contains(string(sw), `"icon-512.png"`) {
		t.Errorf("service worker doesn't cache the files:\n%s", sw)
	}
	// Running again doesn't add the tags twice.
	if err := Make(App{Name: "Notes", ID: "com.example.notes"}, dir); err != nil {
		t.Fatal(err)
	}
	idx2, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	if strings.Count(string(idx2), `rel="manifest"`) != 1 {
		t.Error("manifest linked twice")
	}
	// The maskable icon's padding takes the corner color.
	mf, _ := os.Open(filepath.Join(dir, "icon-maskable-512.png"))
	mimg, _ := png.Decode(mf)
	mf.Close()
	if r, _, _, _ := mimg.At(5, 5).RGBA(); r>>8 != 255 {
		t.Errorf("maskable padding %v, want the corner color", mimg.At(5, 5))
	}
}
