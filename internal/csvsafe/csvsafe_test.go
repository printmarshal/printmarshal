package csvsafe

import (
	"strings"
	"testing"
)

func TestCell(t *testing.T) {
	for in, want := range map[string]string{
		"":                     "",
		"Front desk":           "Front desk",
		"a=b":                  "a=b",
		"=HYPERLINK(\"x\")":    "'=HYPERLINK(\"x\")",
		"+cmd":                 "'+cmd",
		"-2+3":                 "'-2+3",
		"@SUM(A1)":             "'@SUM(A1)",
		"\t=1+1":               "'\t=1+1",
		"\r=1+1":               "'\r=1+1",
		"  =1+1":               "'  =1+1",
		"＝1+1":                 "'＝1+1",
		"'=1+1":                "''=1+1",
		"'plain":               "'plain",
		"-5":                   "-5",
		"+3.25":                "+3.25",
		"-0.5":                 "-0.5",
		"2026-11-01T10:00:00Z": "2026-11-01T10:00:00Z",
		"-":                    "-",
	} {
		if got := Cell(in); got != want {
			t.Errorf("Cell(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUndoReversesCell(t *testing.T) {
	for _, in := range []string{"", "plain", "=1+1", "+cmd", "-2+3", "@x", "\t=1", "'=1", "''=1", "'plain", "-5", "'"} {
		if got := Undo(Cell(in)); got != in {
			t.Errorf("Undo(Cell(%q)) = %q", in, got)
		}
	}
	if got := Undo("'plain"); got != "'plain" {
		t.Errorf("an apostrophe that protects nothing is the value's own: %q", got)
	}
}

func TestWriterAppliesCellToEveryField(t *testing.T) {
	var b strings.Builder
	w := NewWriter(&b)
	if err := w.Write([]string{"=a", "b", "-5"}); err != nil {
		t.Fatal(err)
	}
	w.Flush()
	if got := b.String(); got != "'=a,b,-5\n" {
		t.Errorf("wrote %q", got)
	}
}
