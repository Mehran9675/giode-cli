// Package prompt asks questions on a terminal: a checkbox list moved
// through with the arrow keys, and text answers with a default taken by
// pressing Enter. Without a terminal (input piped in) it falls back to
// reading lines.
package prompt

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// ErrCanceled is returned when the user presses Ctrl-C or input ends.
var ErrCanceled = errors.New("canceled")

// Terminal asks questions on in and out.
type Terminal struct {
	in  *bufio.Reader
	out io.Writer
	// Raw switches the terminal to raw mode for key-by-key input and
	// returns a function restoring it; nil means line input only.
	Raw func() (restore func(), err error)
	// Color turns on ANSI styling.
	Color bool
}

// New returns a Terminal on in and out.
func New(in io.Reader, out io.Writer) *Terminal {
	return &Terminal{in: bufio.NewReader(in), out: out}
}

func (t *Terminal) style(code, s string) string {
	if !t.Color {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

// Bold, Dim, Accent and Green style text when Color is on.
func (t *Terminal) Bold(s string) string   { return t.style("1", s) }
func (t *Terminal) Dim(s string) string    { return t.style("2", s) }
func (t *Terminal) Accent(s string) string { return t.style("36", s) }
func (t *Terminal) Green(s string) string  { return t.style("32", s) }

// Printf writes to the terminal.
func (t *Terminal) Printf(format string, a ...any) { fmt.Fprintf(t.out, format, a...) }

// Item is a choice in a checkbox list.
type Item struct {
	Label string
	// Hint is shown dimmed after the label.
	Hint string
	// Checked is the item's starting state.
	Checked bool
}

// Checkboxes asks the user to check items and returns which are
// checked; at least one must be. On a terminal, ↑/↓ (or k/j) move, Space
// checks, a checks all or none, Enter confirms.
func (t *Terminal) Checkboxes(question string, items []Item) ([]bool, error) {
	checked := make([]bool, len(items))
	for i, it := range items {
		checked[i] = it.Checked
	}
	if t.Raw == nil {
		return t.checkboxLines(question, items, checked)
	}
	restore, err := t.Raw()
	if err != nil {
		return t.checkboxLines(question, items, checked)
	}
	defer restore()
	t.out.Write([]byte("\x1b[?25l")) // hide the cursor
	defer t.out.Write([]byte("\x1b[?25h"))

	cur, msg := 0, ""
	lines := 0
	draw := func() {
		var b strings.Builder
		if lines > 0 {
			fmt.Fprintf(&b, "\x1b[%dA", lines)
		}
		b.WriteString("\r\x1b[J")
		fmt.Fprintf(&b, "%s %s\r\n", t.Accent("?"), t.Bold(question))
		b.WriteString(t.Dim("  ↑/↓ move · space check · a all · enter continue") + "\r\n")
		for i, it := range items {
			box := "[ ]"
			if checked[i] {
				box = t.Green("[x]")
			}
			pointer := "  "
			label := it.Label
			if i == cur {
				pointer = t.Accent("❯ ")
				label = t.Bold(label)
			}
			fmt.Fprintf(&b, "%s%s %s", pointer, box, label)
			if it.Hint != "" {
				b.WriteString("  " + t.Dim(it.Hint))
			}
			b.WriteString("\r\n")
		}
		if msg != "" {
			b.WriteString(t.style("33", "  "+msg) + "\r\n")
		}
		lines = len(items) + 2
		if msg != "" {
			lines++
		}
		io.WriteString(t.out, b.String())
	}
	draw()
	for {
		k, err := t.key()
		if err != nil {
			return nil, err
		}
		msg = ""
		switch k {
		case keyUp:
			cur = (cur + len(items) - 1) % len(items)
		case keyDown:
			cur = (cur + 1) % len(items)
		case keySpace:
			checked[cur] = !checked[cur]
		case keyAll:
			all := true
			for _, c := range checked {
				all = all && c
			}
			for i := range checked {
				checked[i] = !all
			}
		case keyEnter:
			if count(checked) == 0 {
				msg = "Check at least one with Space."
				break
			}
			// Leave a one-line summary in place of the list.
			fmt.Fprintf(t.out, "\x1b[%dA\r\x1b[J%s %s %s\r\n", lines, t.Green("✔"), t.Bold(question), t.Accent(summary(items, checked)))
			return checked, nil
		case keyCancel:
			return nil, ErrCanceled
		}
		draw()
	}
}

func count(b []bool) int {
	n := 0
	for _, v := range b {
		if v {
			n++
		}
	}
	return n
}

func summary(items []Item, checked []bool) string {
	var s []string
	for i, it := range items {
		if checked[i] {
			s = append(s, it.Label)
		}
	}
	return strings.Join(s, ", ")
}

type key int

const (
	keyOther key = iota
	keyUp
	keyDown
	keySpace
	keyAll
	keyEnter
	keyCancel
)

// key reads one key press in raw mode.
func (t *Terminal) key() (key, error) {
	b, err := t.in.ReadByte()
	if err != nil {
		return keyCancel, ErrCanceled
	}
	switch b {
	case 3, 4: // Ctrl-C, Ctrl-D
		return keyCancel, nil
	case '\r', '\n':
		return keyEnter, nil
	case ' ', 'x':
		return keySpace, nil
	case 'a':
		return keyAll, nil
	case 'k', 'w':
		return keyUp, nil
	case 'j', 's':
		return keyDown, nil
	case 0x1b:
		if n, _ := t.in.ReadByte(); n != '[' && n != 'O' {
			return keyOther, nil
		}
		switch c, _ := t.in.ReadByte(); c {
		case 'A':
			return keyUp, nil
		case 'B':
			return keyDown, nil
		}
	}
	return keyOther, nil
}

// checkboxLines asks with a numbered list, answered with numbers.
func (t *Terminal) checkboxLines(question string, items []Item, checked []bool) ([]bool, error) {
	for {
		t.Printf("%s %s\n", t.Accent("?"), t.Bold(question))
		for i, it := range items {
			box := "[ ]"
			if checked[i] {
				box = "[x]"
			}
			t.Printf("  %d. %s %s", i+1, box, it.Label)
			if it.Hint != "" {
				t.Printf("  %s", t.Dim(it.Hint))
			}
			t.Printf("\n")
		}
		t.Printf("  Numbers to check, separated by commas (Enter keeps the checked ones): ")
		line, err := t.line()
		if err != nil {
			return nil, err
		}
		if line != "" {
			next := make([]bool, len(items))
			ok := true
			for _, f := range strings.FieldsFunc(line, func(r rune) bool { return r == ',' || r == ' ' }) {
				n, err := strconv.Atoi(f)
				if err != nil || n < 1 || n > len(items) {
					t.Printf("  %q isn't one of the numbers.\n", f)
					ok = false
					break
				}
				next[n-1] = true
			}
			if !ok {
				continue
			}
			checked = next
		}
		if count(checked) == 0 {
			t.Printf("  Check at least one.\n")
			continue
		}
		return checked, nil
	}
}

func (t *Terminal) line() (string, error) {
	s, err := t.in.ReadString('\n')
	if err != nil && (err != io.EOF || s == "") {
		return "", ErrCanceled
	}
	return strings.TrimSpace(s), nil
}

// Input asks a question whose answer defaults to def when the user just
// presses Enter. validate, when set, rejects answers with a message and
// asks again.
func (t *Terminal) Input(question, def string, validate func(string) error) (string, error) {
	for {
		t.Printf("%s %s", t.Accent("?"), t.Bold(question))
		if def != "" {
			t.Printf(" %s", t.Dim("("+def+")"))
		}
		t.Printf(" ")
		ans, err := t.line()
		if err != nil {
			return "", err
		}
		if ans == "" {
			ans = def
		}
		if validate != nil {
			if err := validate(ans); err != nil {
				t.Printf("  %s\n", t.style("33", err.Error()))
				continue
			}
		}
		return ans, nil
	}
}

// Confirm asks a yes/no question, defaulting to def.
func (t *Terminal) Confirm(question string, def bool) (bool, error) {
	d := "Y/n"
	if !def {
		d = "y/N"
	}
	for {
		t.Printf("%s %s %s ", t.Accent("?"), t.Bold(question), t.Dim("("+d+")"))
		ans, err := t.line()
		if err != nil {
			return false, err
		}
		switch strings.ToLower(ans) {
		case "":
			return def, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
	}
}
