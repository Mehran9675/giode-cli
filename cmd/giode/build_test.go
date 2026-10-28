package main

import "testing"

func TestAddTag(t *testing.T) {
	for _, c := range []struct{ tags, tag, want string }{
		{"", "giode_gio", "giode_gio"},
		{"foo bar", "giode_gio", "foo,bar,giode_gio"},
		{"foo,giode_gio", "giode_gio", "foo,giode_gio"},
	} {
		if got := addTag(c.tags, c.tag); got != c.want {
			t.Errorf("addTag(%q, %q) = %q", c.tags, c.tag, got)
		}
	}
}
