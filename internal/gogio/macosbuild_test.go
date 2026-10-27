// SPDX-License-Identifier: Unlicense OR MIT
//
// Derived from gioui.org/cmd/gogio (v0.10.3); see LICENSE in this directory.

package gogio

import (
	"strings"
	"testing"
)

func TestMacInfoPlistVersion(t *testing.T) {
	t.Parallel()

	ver, err := parseSemver("1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	var b macBuilder
	b.setInfo(&buildInfo{appID: "com.example.app", version: ver}, "app")

	for _, want := range []string{
		"<key>CFBundleShortVersionString</key>\n\t<string>1.2.3</string>",
		"<key>CFBundleVersion</key>\n\t<string>4</string>",
	} {
		if !strings.Contains(string(b.Manifest), want) {
			t.Errorf("Info.plist is missing %q:\n%s", want, b.Manifest)
		}
	}
}
