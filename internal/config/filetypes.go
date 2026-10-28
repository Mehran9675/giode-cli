package config

import (
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"
)

var extRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_+-]*$`)

func (c *Config) validateFileTypes() []error {
	var errs []error
	for i, t := range c.FileTypes {
		if t.Name == "" {
			errs = append(errs, fmt.Errorf("file_types[%d] needs a name", i))
		}
		if len(t.Extensions) == 0 && len(t.MIMETypes) == 0 {
			errs = append(errs, fmt.Errorf("file_types %q needs extensions or mime_types", t.Name))
		}
		for _, e := range t.Extensions {
			if !extRE.MatchString(e) {
				errs = append(errs, fmt.Errorf("file_types %q: extension %q (give it without the dot)", t.Name, e))
			}
		}
		if t.Role != "" && t.Role != "viewer" && t.Role != "editor" {
			errs = append(errs, fmt.Errorf("file_types %q: role is viewer or editor", t.Name))
		}
	}
	return errs
}

// uti is a type's macOS type identifier, declared by the app when the
// type doesn't name one.
func (c *Config) uti(t FileType) (id string, declare bool) {
	if t.UTI != "" {
		return t.UTI, false
	}
	key := strings.ToLower(t.Name)
	if len(t.Extensions) > 0 {
		key = strings.ToLower(t.Extensions[0])
	}
	key = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		return '-'
	}, key)
	return c.App.ID + "." + key, true
}

// conformsTo is the UTI a declared type conforms to, by its MIME type.
func conformsTo(t FileType) string {
	for _, m := range t.MIMETypes {
		switch {
		case strings.HasPrefix(m, "video/"):
			return "public.movie"
		case strings.HasPrefix(m, "audio/"):
			return "public.audio"
		case strings.HasPrefix(m, "image/"):
			return "public.image"
		case strings.HasPrefix(m, "text/"):
			return "public.text"
		}
	}
	return "public.data"
}

// macFileTypes returns the Info.plist entries for file_types:
// CFBundleDocumentTypes and, for types giode declares,
// UTImportedTypeDeclarations.
func (c *Config) macFileTypes() map[string]any {
	if len(c.FileTypes) == 0 {
		return nil
	}
	var docs, imported []any
	for _, t := range c.FileTypes {
		id, declare := c.uti(t)
		role := "Viewer"
		if t.Role == "editor" {
			role = "Editor"
		}
		doc := map[string]any{
			"CFBundleTypeName":   t.Name,
			"CFBundleTypeRole":   role,
			"LSHandlerRank":      "Alternate",
			"LSItemContentTypes": []any{id},
		}
		if len(t.Extensions) > 0 {
			doc["CFBundleTypeExtensions"] = toAny(t.Extensions)
		}
		docs = append(docs, doc)
		if declare {
			tags := map[string]any{}
			if len(t.Extensions) > 0 {
				tags["public.filename-extension"] = toAny(t.Extensions)
			}
			if len(t.MIMETypes) > 0 {
				tags["public.mime-type"] = toAny(t.MIMETypes)
			}
			imported = append(imported, map[string]any{
				"UTTypeIdentifier":       id,
				"UTTypeDescription":      t.Name,
				"UTTypeConformsTo":       []any{conformsTo(t)},
				"UTTypeTagSpecification": tags,
			})
		}
	}
	out := map[string]any{"CFBundleDocumentTypes": docs}
	if len(imported) > 0 {
		out["UTImportedTypeDeclarations"] = imported
	}
	return out
}

func toAny(s []string) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

// LinuxMIMETypes returns the MIME types the app opens, for the .desktop
// file's MimeType, and a shared-mime-info package declaring the ones
// only the app knows (types given by extension alone), or "".
func (c *Config) LinuxMIMETypes(command string) (types []string, mimeXML string) {
	var decl strings.Builder
	for _, t := range c.FileTypes {
		mimes := t.MIMETypes
		if len(mimes) == 0 {
			m := "application/x-" + command + "-" + strings.ToLower(t.Extensions[0])
			mimes = []string{m}
			fmt.Fprintf(&decl, "  <mime-type type=\"%s\">\n    <comment>%s</comment>\n", m, html.EscapeString(t.Name))
			for _, e := range t.Extensions {
				fmt.Fprintf(&decl, "    <glob pattern=\"*.%s\"/>\n", e)
			}
			decl.WriteString("  </mime-type>\n")
		}
		types = append(types, mimes...)
	}
	if decl.Len() > 0 {
		mimeXML = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<mime-info xmlns=\"http://www.freedesktop.org/standards/shared-mime-info\">\n" + decl.String() + "</mime-info>\n"
	}
	return types, mimeXML
}

// WindowsFileType is a file type registered for "Open with" on Windows.
type WindowsFileType struct {
	// ProgID is the app's class for the type ("com.example.player.mkv").
	ProgID, Name string
	// Extensions with the dot (".mkv").
	Extensions []string
	Icon       string
}

// WindowsFileTypes returns file_types for Windows' registry and MSIX.
func (c *Config) WindowsFileTypes() []WindowsFileType {
	var out []WindowsFileType
	for _, t := range c.FileTypes {
		if len(t.Extensions) == 0 {
			continue
		}
		w := WindowsFileType{ProgID: c.App.ID + "." + strings.ToLower(t.Extensions[0]), Name: t.Name, Icon: c.Path(t.Icon)}
		for _, e := range t.Extensions {
			w.Extensions = append(w.Extensions, "."+strings.ToLower(e))
		}
		sort.Strings(w.Extensions)
		out = append(out, w)
	}
	return out
}
