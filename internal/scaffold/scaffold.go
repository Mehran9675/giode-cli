// Package scaffold creates new giode projects from templates.
package scaffold

import (
	"embed"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"math"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"unicode"
)

//go:embed templates
var templates embed.FS

// Templates lists the project templates.
var Templates = map[string]string{
	"app":     "a notes app: theme switch, composer, scrolling list with swipe to delete",
	"minimal": "a window with a counter button",
}

// Project describes a project to create.
type Project struct {
	// Dir is the directory to create; it must not exist or be empty.
	Dir string
	// Name is the app's display name.
	Name string
	// Module is the Go module path.
	Module string
	// AppID is the reverse-DNS app ID.
	AppID string
	// Template is a key of Templates.
	Template string
}

// data is what the templates see.
type data struct {
	Name, Module, AppID, Binary string
}

// Create writes the project's files: main.go, giode.toml, an icon,
// .gitignore and README.md. It doesn't create go.mod; the caller runs
// go mod init and fetches giode.
func Create(p Project) error {
	if _, ok := Templates[p.Template]; !ok {
		return fmt.Errorf("unknown template %q", p.Template)
	}
	if entries, err := os.ReadDir(p.Dir); err == nil && len(entries) > 0 {
		return fmt.Errorf("%s exists and isn't empty", p.Dir)
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	created := false
	if _, err := os.Stat(p.Dir); errors.Is(err, fs.ErrNotExist) {
		created = true
	}
	if err := create(p); err != nil {
		if created {
			os.RemoveAll(p.Dir)
		}
		return err
	}
	return nil
}

func create(p Project) error {
	d := data{Name: p.Name, Module: p.Module, AppID: p.AppID, Binary: path.Base(p.Module)}
	files := map[string]string{
		"main.go":    "templates/" + p.Template + "/main.go.tmpl",
		"giode.toml": "templates/common/giode.toml.tmpl",
		".gitignore": "templates/common/gitignore.tmpl",
		"README.md":  "templates/common/README.md.tmpl",
	}
	for dst, src := range files {
		// [[ ]] delimiters, since Go code is full of {{ }}.
		t, err := template.New(path.Base(src)).Delims("[[", "]]").ParseFS(templates, src)
		if err != nil {
			return err
		}
		out := filepath.Join(p.Dir, dst)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		f, err := os.Create(out)
		if err != nil {
			return err
		}
		err = t.Execute(f, d)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return fmt.Errorf("writing %s: %w", dst, err)
		}
	}
	return writeIcon(filepath.Join(p.Dir, "assets", "icon.png"))
}

// writeIcon writes a placeholder 1024x1024 app icon: a rounded square
// with a ring, to replace with the real one.
func writeIcon(path string) error {
	const size = 1024
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	top, bottom := color.NRGBA{59, 130, 246, 255}, color.NRGBA{30, 64, 175, 255}
	for y := 0; y < size; y++ {
		t := float64(y) / size
		bg := color.NRGBA{
			R: uint8(float64(top.R)*(1-t) + float64(bottom.R)*t),
			G: uint8(float64(top.G)*(1-t) + float64(bottom.G)*t),
			B: uint8(float64(top.B)*(1-t) + float64(bottom.B)*t),
			A: 255,
		}
		for x := 0; x < size; x++ {
			// Distance from the center of a ring of radius 260 and
			// width 90.
			dx, dy := float64(x)-size/2, float64(y)-size/2
			d := math.Abs(math.Hypot(dx, dy) - 260)
			c := bg
			if a := clamp01(45 - d + 0.5); a > 0 {
				c = blend(bg, color.NRGBA{255, 255, 255, 255}, a)
			}
			img.SetNRGBA(x, y, c)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	err = png.Encode(f, img)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func blend(a, b color.NRGBA, t float64) color.NRGBA {
	mix := func(x, y uint8) uint8 { return uint8(float64(x)*(1-t) + float64(y)*t + 0.5) }
	return color.NRGBA{mix(a.R, b.R), mix(a.G, b.G), mix(a.B, b.B), 255}
}

var nonID = regexp.MustCompile(`[^a-z0-9]+`)

// Defaults derives the display name, module path and app ID from a
// project directory name ("my-notes" gives "My Notes", "my-notes" and
// "com.example.mynotes").
func Defaults(dir string) (name, module, appID string) {
	base := filepath.Base(dir)
	words := strings.FieldsFunc(base, func(r rune) bool { return r == '-' || r == '_' || r == ' ' || r == '.' })
	for i, w := range words {
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	name = strings.Join(words, " ")
	if name == "" {
		name = "App"
	}
	module = strings.ToLower(nonID.ReplaceAllString(base, "-"))
	module = strings.Trim(module, "-")
	if module == "" {
		module = "app"
	}
	appID = "com.example." + AppIDPart(base)
	return name, module, appID
}

// AppIDPart turns s into a valid last part of an app ID.
func AppIDPart(s string) string {
	p := nonID.ReplaceAllString(strings.ToLower(s), "")
	if p == "" || unicode.IsDigit(rune(p[0])) {
		p = "app" + p
	}
	return p
}
