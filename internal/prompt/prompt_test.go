package prompt

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

var items = []Item{{Label: "Windows"}, {Label: "macOS", Checked: true}, {Label: "Linux"}, {Label: "Android", Hint: "needs the Android SDK"}}

func raw(in string) (*Terminal, *bytes.Buffer) {
	var out bytes.Buffer
	t := New(strings.NewReader(in), &out)
	t.Raw = func() (func(), error) { return func() {}, nil }
	return t, &out
}

func TestCheckboxesKeys(t *testing.T) {
	// Down to Linux (arrow keys), check it, k back up to macOS and
	// uncheck it, j j j to Android (wrapping), check, Enter.
	term, out := raw("\x1b[B\x1b[B \x1b[A \x1b[B\x1b[B \r")
	got, err := term.Checkboxes("Platforms?", items)
	if err != nil {
		t.Fatal(err)
	}
	if want := []bool{false, false, true, true}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if !strings.Contains(out.String(), "needs the Android SDK") || !strings.Contains(out.String(), "Platforms? Linux, Android") {
		t.Errorf("output:\n%q", out.String())
	}
}

func TestCheckboxesNeedsOne(t *testing.T) {
	// Uncheck macOS, Enter refuses, a checks all, Enter.
	term, out := raw("j \ra\r")
	got, err := term.Checkboxes("Platforms?", items)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []bool{true, true, true, true}) {
		t.Errorf("got %v", got)
	}
	if !strings.Contains(out.String(), "at least one") {
		t.Error("no message for an empty choice")
	}
	if _, err := New(strings.NewReader("\x03"), out).withRaw().Checkboxes("q", items); err != ErrCanceled {
		t.Errorf("Ctrl-C: %v", err)
	}
}

func (t *Terminal) withRaw() *Terminal {
	t.Raw = func() (func(), error) { return func() {}, nil }
	return t
}

func TestCheckboxesLines(t *testing.T) {
	var out bytes.Buffer
	term := New(strings.NewReader("9\n1, 4\n"), &out)
	got, err := term.Checkboxes("Platforms?", items)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []bool{true, false, false, true}) {
		t.Errorf("got %v", got)
	}
	term = New(strings.NewReader("\n"), &out)
	if got, _ := term.Checkboxes("Platforms?", items); !reflect.DeepEqual(got, []bool{false, true, false, false}) {
		t.Errorf("Enter keeps defaults: %v", got)
	}
}

func TestInput(t *testing.T) {
	var out bytes.Buffer
	term := New(strings.NewReader("\nbad id\ncom.acme.notes\n"), &out)
	if got, _ := term.Input("Name", "Notes", nil); got != "Notes" {
		t.Errorf("default: %q", got)
	}
	got, err := term.Input("App ID", "com.example.notes", func(s string) error {
		if strings.Contains(s, " ") {
			return errString("no spaces")
		}
		return nil
	})
	if err != nil || got != "com.acme.notes" || !strings.Contains(out.String(), "no spaces") {
		t.Errorf("%q %v\n%s", got, err, out.String())
	}
	if _, err := term.Input("More", "", nil); err != ErrCanceled {
		t.Errorf("EOF: %v", err)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
