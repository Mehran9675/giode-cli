package gogio

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func square(size int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for i := range img.Pix {
		img.Pix[i] = []uint8{c.R, c.G, c.B, c.A}[i%4]
	}
	return img
}

func writePNG(t *testing.T, path string, img image.Image) string {
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAdaptiveIconFromAppIcon(t *testing.T) {
	res := t.TempDir()
	navy := color.NRGBA{0x0f, 0x17, 0x2a, 0xff}
	xml, err := writeAdaptiveIcon(res, square(512, navy), AdaptiveIcon{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(xml, `@color/ic_launcher_background`) || !strings.Contains(xml, `@mipmap/ic_launcher_foreground`) || strings.Contains(xml, "monochrome") {
		t.Errorf("xml:\n%s", xml)
	}
	vals, _ := os.ReadFile(filepath.Join(res, "values", "ic_launcher_background.xml"))
	if !strings.Contains(string(vals), "#FF0F172A") {
		t.Errorf("background color from the icon's edge:\n%s", vals)
	}
	// The foreground fits the 66dp safe zone of the 108dp canvas.
	f, err := os.Open(filepath.Join(res, "mipmap-xxxhdpi", "ic_launcher_foreground.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 432 {
		t.Fatalf("xxxhdpi layer %v, want 432 px", img.Bounds())
	}
	alpha := func(x, y int) uint8 { return color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA).A }
	size, zone := 432.0, safeZone
	inner := int(size*zone + 0.5)
	off := (432 - inner) / 2
	if alpha(off-3, 216) != 0 || alpha(216, off-3) != 0 || alpha(5, 5) != 0 {
		t.Error("the foreground reaches outside the safe zone")
	}
	if alpha(off+3, 216) != 0xff || alpha(216, 216) != 0xff {
		t.Error("the foreground doesn't fill the safe zone")
	}
}

func TestAdaptiveIconLayers(t *testing.T) {
	dir, res := t.TempDir(), t.TempDir()
	fg := writePNG(t, filepath.Join(dir, "fg.png"), square(108, color.NRGBA{255, 255, 255, 255}))
	bg := writePNG(t, filepath.Join(dir, "bg.png"), square(108, color.NRGBA{0, 0, 255, 255}))
	mono := writePNG(t, filepath.Join(dir, "mono.png"), square(108, color.NRGBA{0, 0, 0, 255}))
	xml, err := writeAdaptiveIcon(res, square(64, color.NRGBA{255, 0, 0, 255}), AdaptiveIcon{Foreground: fg, Background: bg, Monochrome: mono})
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{`<background android:drawable="@mipmap/ic_launcher_background" />`, `<foreground android:drawable="@mipmap/ic_launcher_foreground" />`, `<monochrome android:drawable="@mipmap/ic_launcher_monochrome" />`} {
		if !strings.Contains(xml, w) {
			t.Errorf("xml lacks %s:\n%s", w, xml)
		}
	}
	for _, d := range adaptiveSizes {
		for _, name := range []string{"ic_launcher_foreground.png", "ic_launcher_background.png", "ic_launcher_monochrome.png"} {
			if _, err := os.Stat(filepath.Join(res, d.dir, name)); err != nil {
				t.Errorf("missing %s/%s", d.dir, name)
			}
		}
	}
	// An explicit foreground fills the canvas as drawn.
	f, _ := os.Open(filepath.Join(res, "mipmap-mdpi", "ic_launcher_foreground.png"))
	img, _ := png.Decode(f)
	f.Close()
	if a := color.NRGBAModel.Convert(img.At(1, 1)).(color.NRGBA).A; a != 0xff {
		t.Errorf("explicit foreground was shrunk: corner alpha %d", a)
	}

	// A color background.
	res2 := t.TempDir()
	xml, err = writeAdaptiveIcon(res2, square(64, color.NRGBA{255, 0, 0, 255}), AdaptiveIcon{Background: "#1e293b"})
	if err != nil {
		t.Fatal(err)
	}
	vals, _ := os.ReadFile(filepath.Join(res2, "values", "ic_launcher_background.xml"))
	if !strings.Contains(xml, "@color/ic_launcher_background") || !strings.Contains(string(vals), "#FF1E293B") {
		t.Errorf("color background: %s\n%s", xml, vals)
	}
	if _, err := writeAdaptiveIcon(t.TempDir(), square(8, color.NRGBA{A: 255}), AdaptiveIcon{Background: "#12"}); err == nil {
		t.Error("a bad color was accepted")
	}
}

func TestParseHexColor(t *testing.T) {
	for in, want := range map[string]color.NRGBA{
		"#fff": {255, 255, 255, 255}, "#1e293b": {0x1e, 0x29, 0x3b, 255}, "#1e293b80": {0x1e, 0x29, 0x3b, 0x80},
	} {
		if got, err := parseHexColor(in); err != nil || got != want {
			t.Errorf("%s: %v %v", in, got, err)
		}
	}
}

func TestEdgeColorTransparentIsWhite(t *testing.T) {
	if c := edgeColor(square(16, color.NRGBA{})); c != (color.NRGBA{255, 255, 255, 255}) {
		t.Errorf("transparent icon edge gave %v", c)
	}
}
