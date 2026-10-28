// Package maven resolves Android library dependencies from Maven
// repositories - Google's, Maven Central - and downloads them: the
// artifacts Gradle would put in an app, without Gradle.
//
// Resolution follows each POM's compile and runtime dependencies,
// inheriting parent POMs' properties and managed versions; when two
// paths ask for different versions of a library the highest wins, as
// in Gradle.
package maven

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Coord is a group:artifact:version coordinate.
type Coord struct {
	Group, Artifact, Version string
}

func (c Coord) String() string { return c.Group + ":" + c.Artifact + ":" + c.Version }

func (c Coord) key() string { return c.Group + ":" + c.Artifact }

// Parse parses group:artifact:version.
func Parse(s string) (Coord, error) {
	p := strings.Split(strings.TrimSpace(s), ":")
	if len(p) != 3 || p[0] == "" || p[1] == "" || p[2] == "" {
		return Coord{}, fmt.Errorf("maven: %q isn't group:artifact:version", s)
	}
	return Coord{p[0], p[1], p[2]}, nil
}

// Artifact is a resolved, downloaded library.
type Artifact struct {
	Coord
	// Path is the downloaded .aar or .jar.
	Path string
}

// Resolver resolves and downloads dependencies.
type Resolver struct {
	// Repos are repository base URLs, tried in order.
	Repos []string
	// Cache is where downloads are kept.
	Cache string
	// Client fetches files; http.DefaultClient when nil.
	Client *http.Client
	// Log, when set, receives a line per download.
	Log func(string)

	poms map[string]*pom
}

// DefaultRepos are Google's Maven repository and Maven Central.
var DefaultRepos = []string{"https://dl.google.com/dl/android/maven2", "https://repo1.maven.org/maven2"}

type pom struct {
	XMLName    xml.Name `xml:"project"`
	GroupID    string   `xml:"groupId"`
	ArtifactID string   `xml:"artifactId"`
	Version    string   `xml:"version"`
	Packaging  string   `xml:"packaging"`
	Parent     struct {
		GroupID    string `xml:"groupId"`
		ArtifactID string `xml:"artifactId"`
		Version    string `xml:"version"`
	} `xml:"parent"`
	Properties struct {
		Entries []struct {
			XMLName xml.Name
			Value   string `xml:",chardata"`
		} `xml:",any"`
	} `xml:"properties"`
	Managed      []dependency `xml:"dependencyManagement>dependencies>dependency"`
	Dependencies []dependency `xml:"dependencies>dependency"`

	props   map[string]string
	managed map[string]dependency
}

type dependency struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
	Scope      string `xml:"scope"`
	Type       string `xml:"type"`
	Optional   string `xml:"optional"`
}

// Resolve resolves roots and their dependencies, downloads them, and
// returns them sorted by coordinate.
func (r *Resolver) Resolve(roots []Coord) ([]Artifact, error) {
	if r.poms == nil {
		r.poms = map[string]*pom{}
	}
	chosen := map[string]Coord{}
	packaging := map[string]string{}
	// Breadth first, re-walking a library when a higher version wins.
	queue := append([]Coord{}, roots...)
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if cur, ok := chosen[c.key()]; ok && compareVersions(cur.Version, c.Version) >= 0 {
			continue
		}
		chosen[c.key()] = c
		p, err := r.pom(c)
		if err != nil {
			return nil, err
		}
		packaging[c.key()] = p.Packaging
		for _, d := range p.Dependencies {
			if d.Scope == "test" || d.Scope == "provided" || d.Scope == "system" || d.Optional == "true" {
				continue
			}
			g, a, v := p.expand(d.GroupID), p.expand(d.ArtifactID), p.expand(d.Version)
			if v == "" {
				if m, ok := p.managed[g+":"+a]; ok {
					v = p.expand(m.Version)
				}
			}
			v = cleanVersion(v)
			if v == "" {
				return nil, fmt.Errorf("maven: %s needs %s:%s without a version", c, g, a)
			}
			queue = append(queue, Coord{g, a, v})
		}
	}
	var out []Artifact
	for k, c := range chosen {
		if packaging[k] == "pom" {
			continue
		}
		ext := "jar"
		if packaging[k] == "aar" {
			ext = "aar"
		}
		path, err := r.fetch(c, ext)
		if err != nil && ext == "jar" {
			// Some POMs omit <packaging>aar</packaging>.
			path, err = r.fetch(c, "aar")
		}
		if err != nil {
			return nil, err
		}
		out = append(out, Artifact{Coord: c, Path: path})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out, nil
}

// pom loads, and caches, c's POM with its parents' properties and
// managed versions.
func (r *Resolver) pom(c Coord) (*pom, error) {
	if p, ok := r.poms[c.String()]; ok {
		return p, nil
	}
	path, err := r.fetch(c, "pom")
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	p := &pom{}
	if err := xml.Unmarshal(b, p); err != nil {
		return nil, fmt.Errorf("maven: %s's POM: %w", c, err)
	}
	p.props = map[string]string{}
	p.managed = map[string]dependency{}
	if p.Parent.ArtifactID != "" {
		parent, err := r.pom(Coord{p.Parent.GroupID, p.Parent.ArtifactID, p.Parent.Version})
		if err != nil {
			return nil, err
		}
		for k, v := range parent.props {
			p.props[k] = v
		}
		for k, v := range parent.managed {
			p.managed[k] = v
		}
		if p.GroupID == "" {
			p.GroupID = parent.GroupID
		}
		if p.Version == "" {
			p.Version = parent.Version
		}
	}
	for _, e := range p.Properties.Entries {
		p.props[e.XMLName.Local] = strings.TrimSpace(e.Value)
	}
	p.props["project.version"], p.props["version"] = p.Version, p.Version
	p.props["project.groupId"], p.props["groupId"] = p.GroupID, p.GroupID
	for _, m := range p.Managed {
		p.managed[p.expand(m.GroupID)+":"+p.expand(m.ArtifactID)] = m
	}
	r.poms[c.String()] = p
	return p, nil
}

var propRE = regexp.MustCompile(`\$\{([^}]+)\}`)

func (p *pom) expand(s string) string {
	for i := 0; i < 5 && strings.Contains(s, "${"); i++ {
		s = propRE.ReplaceAllStringFunc(s, func(m string) string {
			if v, ok := p.props[m[2:len(m)-1]]; ok {
				return v
			}
			return m
		})
	}
	return strings.TrimSpace(s)
}

// cleanVersion takes the version a range like [1.2.3] pins, or the
// lower bound of [1.2,2.0).
func cleanVersion(v string) string {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "[") || strings.HasPrefix(v, "(") {
		v = strings.Trim(v, "[]()")
		if i := strings.IndexByte(v, ','); i >= 0 {
			v = v[:i]
		}
	}
	return strings.TrimSpace(v)
}

// fetch downloads c's file with extension ext into the cache, once.
func (r *Resolver) fetch(c Coord, ext string) (string, error) {
	rel := strings.ReplaceAll(c.Group, ".", "/") + "/" + c.Artifact + "/" + c.Version + "/" + c.Artifact + "-" + c.Version + "." + ext
	local := filepath.Join(r.Cache, filepath.FromSlash(rel))
	if _, err := os.Stat(local); err == nil {
		return local, nil
	}
	client := r.Client
	if client == nil {
		client = http.DefaultClient
	}
	var lastErr error
	for _, repo := range r.Repos {
		url := strings.TrimRight(repo, "/") + "/" + rel
		resp, err := client.Get(url)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("%s: %s", url, resp.Status)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
			resp.Body.Close()
			return "", err
		}
		tmp := local + ".part"
		f, err := os.Create(tmp)
		if err != nil {
			resp.Body.Close()
			return "", err
		}
		_, err = io.Copy(f, resp.Body)
		resp.Body.Close()
		f.Close()
		if err != nil {
			return "", err
		}
		if r.Log != nil && ext != "pom" {
			r.Log(c.String() + " (" + ext + ")")
		}
		return local, os.Rename(tmp, local)
	}
	return "", fmt.Errorf("maven: can't download %s %s: %v", c, ext, lastErr)
}

// compareVersions compares dotted versions numerically, then
// qualifiers (a release beats its -alpha, -beta, -rc).
func compareVersions(a, b string) int {
	split := func(v string) ([]string, string) {
		q := ""
		if i := strings.IndexByte(v, '-'); i >= 0 {
			v, q = v[:i], v[i+1:]
		}
		return strings.Split(v, "."), q
	}
	pa, qa := split(a)
	pb, qb := split(b)
	for i := 0; i < max(len(pa), len(pb)); i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	switch {
	case qa == qb:
		return 0
	case qa == "":
		return 1
	case qb == "":
		return -1
	}
	return strings.Compare(qa, qb)
}
