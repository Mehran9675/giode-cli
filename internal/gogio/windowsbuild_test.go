// SPDX-License-Identifier: Unlicense OR MIT
//
// Derived from gioui.org/cmd/gogio (v0.10.3); see LICENSE in this directory.

package gogio

import "testing"

func TestWindowsFixedVersion(t *testing.T) {
	tests := []struct {
		version string
		want    [2]uint32
	}{
		{"0.0.0.0", [2]uint32{0x00000000, 0x00000000}},
		{"1.0.0.1", [2]uint32{0x00010000, 0x00000001}},
		{"1.2.3.4", [2]uint32{0x00010002, 0x00030004}},
		{"0.18.0.0", [2]uint32{0x00000012, 0x00000000}},
		// Each component occupies 16 bits, so 65535 is the largest value
		// a Windows version resource can represent. Android version codes
		// routinely exceed that; see TestVersion.
		{"65535.65535.65535.65535", [2]uint32{0xFFFFFFFF, 0xFFFFFFFF}},
	}

	for i, test := range tests {
		ver, err := parseSemver(test.version)
		if err != nil {
			t.Errorf("(%d): %q failed to parse: %v", i, test.version, err)
			continue
		}
		if got := windowsFixedVersion(ver); got != test.want {
			t.Errorf("(%d): %q packed to {%#08x, %#08x}, want {%#08x, %#08x}",
				i, test.version, got[0], got[1], test.want[0], test.want[1])
		}
	}
}

func TestCheckWindowsVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		version string
		valid   bool
	}{
		{"1.2.3.4", true},
		{"65535.65535.65535.65535", true},
		{"65536.0.0.0", false},
		{"0.65536.0.0", false},
		{"0.0.65536.0", false},
		// A version code Android accepts, but Windows cannot represent.
		{"0.0.1.20241024", false},
	}

	for i, test := range tests {
		ver, err := parseSemver(test.version)
		if err != nil {
			t.Errorf("(%d): %q failed to parse: %v", i, test.version, err)
			continue
		}
		switch err := checkWindowsVersion(ver); {
		case err != nil && test.valid:
			t.Errorf("(%d): %q was rejected: %v", i, test.version, err)
		case err == nil && !test.valid:
			t.Errorf("(%d): %q was unexpectedly accepted", i, test.version)
		}
	}
}
