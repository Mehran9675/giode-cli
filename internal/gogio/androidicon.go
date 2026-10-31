package gogio

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	xdraw "golang.org/x/image/draw"
)

// AdaptiveIcon is an Android adaptive icon's layers, each a 108dp
// square of which launchers show the middle 72dp, cut to their shape.
type AdaptiveIcon struct {
	// Foreground is a PNG drawn over the background, designed for the
	// whole 108dp canvas with the artwork inside the middle 66dp. Empty
	// uses the app icon, shrunk into that safe zone.
	Foreground string
	// Background is a PNG for the whole canvas or a color ("#1e293b",
	// "#rgb", "#rrggbbaa"). Empty uses the color around the app icon's
	// edge, or white.
	Background string
	// Monochrome is a PNG whose shape (its alpha) Android 13+ tints for
	// themed icons. Empty leaves themed icons off.
	Monochrome string
}

// safeZone is the fraction of the 108dp canvas that stays visible under
// every launcher mask: a 66dp circle.
const safeZone = 66.0 / 108

// adaptiveSizes are the 108dp layer's sizes per density.
var adaptiveSizes = []struct {
	dir  string
	size int
}{
	{"mipmap-mdpi", 108}, {"mipmap-hdpi", 162}, {"mipmap-xhdpi", 216}, {"mipmap-xxhdpi", 324}, {"mipmap-xxxhdpi", 432},
}

// writeAdaptiveIcon writes the adaptive icon's layers into resDir and
// returns the mipmap-anydpi-v26/ic_launcher.xml that combines them.
func writeAdaptiveIcon(resDir string, appIcon image.Image, ic AdaptiveIcon) (string, error) {
	fg := appIcon
	pad := safeZone
	if ic.Foreground != "" {
		img, err := decodePNG(ic.Foreground)
		if err != nil {
			return "", fmt.Errorf("android.icon_foreground: %w", err)
		}
		fg, pad = img, 1
	}
	if err := writeLayer(resDir, "ic_launcher_foreground.png", fg, pad); err != nil {
		return "", err
	}

	background := `@color/ic_launcher_background`
	switch bg := ic.Background; {
	case bg == "":
		if err := writeColor(resDir, edgeColor(appIcon)); err != nil {
			return "", err
		}
	case strings.HasPrefix(bg, "#"):
		c, err := parseHexColor(bg)
		if err != nil {
			return "", fmt.Errorf("android.icon_background: %w", err)
		}
		if err := writeColor(resDir, c); err != nil {
			return "", err
		}
	default:
		img, err := decodePNG(bg)
		if err != nil {
			return "", fmt.Errorf("android.icon_background: %w", err)
		}
		if err := writeLayer(resDir, "ic_launcher_background.png", img, 1); err != nil {
			return "", err
		}
		background = `@mipmap/ic_launcher_background`
	}

	mono := ""
	if ic.Monochrome != "" {
		img, err := decodePNG(ic.Monochrome)
		if err != nil {
			return "", fmt.Errorf("android.icon_monochrome: %w", err)
		}
		if err := writeLayer(resDir, "ic_launcher_monochrome.png", img, 1); err != nil {
			return "", err
		}
		mono = "\n    <monochrome android:drawable=\"@mipmap/ic_launcher_monochrome\" />"
	}
	return `<?xml version="1.0" encoding="utf-8"?>
<adaptive-icon xmlns:android="http://schemas.android.com/apk/res/android">
    <background android:drawable="` + background + `" />
    <foreground android:drawable="@mipmap/ic_launcher_foreground" />` + mono + `
</adaptive-icon>`, nil
}

// writeLayer writes img at every density as name, scaled to fraction of
// the canvas and centered on transparency.
func writeLayer(resDir, name string, img image.Image, fraction float64) error {
	for _, d := range adaptiveSizes {
		canvas := image.NewNRGBA(image.Rect(0, 0, d.size, d.size))
		inner := int(float64(d.size)*fraction + 0.5)
		off := (d.size - inner) / 2
		xdraw.CatmullRom.Scale(canvas, image.Rect(off, off, off+inner, off+inner), img, img.Bounds(), draw.Over, nil)
		dir := filepath.Join(resDir, d.dir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		err = png.Encode(f, canvas)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func writeColor(resDir string, c color.NRGBA) error {
	dir := filepath.Join(resDir, "values")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	xml := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<resources>
    <color name="ic_launcher_background">#%02X%02X%02X%02X</color>
</resources>
`, c.A, c.R, c.G, c.B)
	return os.WriteFile(filepath.Join(dir, "ic_launcher_background.xml"), []byte(xml), 0o644)
}

// edgeColor is the average opaque color along img's border: the
// backdrop a square icon was drawn on. A transparent border gives white.
func edgeColor(img image.Image) color.NRGBA {
	b := img.Bounds()
	var r, g, bl, n uint64
	add := func(x, y int) {
		c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
		if c.A < 0xf0 {
			return
		}
		r, g, bl, n = r+uint64(c.R), g+uint64(c.G), bl+uint64(c.B), n+1
	}
	for x := b.Min.X; x < b.Max.X; x++ {
		add(x, b.Min.Y)
		add(x, b.Max.Y-1)
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		add(b.Min.X, y)
		add(b.Max.X-1, y)
	}
	perimeter := uint64(2 * (b.Dx() + b.Dy()))
	if n == 0 || n < perimeter/2 {
		return color.NRGBA{0xff, 0xff, 0xff, 0xff}
	}
	return color.NRGBA{uint8(r / n), uint8(g / n), uint8(bl / n), 0xff}
}

// parseHexColor reads #rgb, #rrggbb or #rrggbbaa (alpha last, as in
// CSS and giode's styles).
func parseHexColor(s string) (color.NRGBA, error) {
	h := strings.TrimPrefix(s, "#")
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) == 6 {
		h += "ff"
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if len(h) != 8 || err != nil {
		return color.NRGBA{}, fmt.Errorf("%q isn't a color: use #rgb, #rrggbb or #rrggbbaa", s)
	}
	return color.NRGBA{uint8(v >> 24), uint8(v >> 16), uint8(v >> 8), uint8(v)}, nil
}

func decodePNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}
