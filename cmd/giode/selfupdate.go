package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"time"
)

// releaseRepo is where releases are published.
const releaseRepo = "Mehran9675/giode-cli"

const selfUpdateUsage = `
Usage: giode self-update [version]

Replaces this giode with the latest release (or the given one, like
v0.2.0) for this computer, downloaded from GitHub. Installed with go
install instead? Run go install github.com/mehran9675/giode-cli/cmd/giode@latest.
`

func cmdSelfUpdate(args []string) error {
	fs := flags("self-update", selfUpdateUsage)
	force := fs.Bool("force", false, "reinstall even when already up to date")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	tag := ""
	if len(pos) > 0 {
		tag = pos[0]
	} else {
		tag, err = latestRelease(client)
		if err != nil {
			return err
		}
	}
	if tag == currentVersion() && !*force {
		fmt.Printf("giode %s is the latest.\n", tag)
		return nil
	}
	name := fmt.Sprintf("giode_%s_%s", runtime.GOOS, runtime.GOARCH)
	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	url := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s%s", releaseRepo, tag, name, ext)
	fmt.Println("Downloading", url)
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", url, resp.Status)
	}
	archive, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	bin, err := extractBinary(archive, ext == ".zip")
	if err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if self, err = filepath.EvalSymlinks(self); err != nil {
		return err
	}
	if err := replaceExecutable(self, bin); err != nil {
		return fmt.Errorf("replacing %s: %w", self, err)
	}
	fmt.Printf("Updated giode %s → %s\n", currentVersion(), tag)
	return nil
}

func latestRelease(client *http.Client) (string, error) {
	resp, err := client.Get("https://api.github.com/repos/" + releaseRepo + "/releases/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", errors.New("no giode release is published yet; install with go install github.com/mehran9675/giode-cli/cmd/giode@latest")
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("checking the latest release: %s", resp.Status)
	}
	var r struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", err
	}
	return r.TagName, nil
}

// extractBinary returns the giode executable from a release archive.
func extractBinary(archive []byte, isZip bool) ([]byte, error) {
	want := "giode"
	if isZip {
		want = "giode.exe"
		zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			if path.Base(f.Name) == want {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return io.ReadAll(rc)
			}
		}
		return nil, errors.New("the archive has no giode.exe")
	}
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil, errors.New("the archive has no giode executable")
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag == tar.TypeReg && path.Base(h.Name) == want {
			return io.ReadAll(tr)
		}
	}
}

// replaceExecutable swaps the running executable for bin. Windows can't
// overwrite a running program but can rename it, so the old one moves
// aside first.
func replaceExecutable(self string, bin []byte) error {
	tmp := self + ".new"
	if err := os.WriteFile(tmp, bin, 0o755); err != nil {
		return err
	}
	old := self + ".old"
	os.Remove(old)
	if err := os.Rename(self, old); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, self); err != nil {
		os.Rename(old, self)
		return err
	}
	if runtime.GOOS != "windows" {
		os.Remove(old)
	}
	return nil
}
