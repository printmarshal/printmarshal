// Package csvsafe keeps values out of a spreadsheet's formula bar. A cell that
// starts with =, +, - or @ runs as a formula when a CSV is opened, and several
// values the console exports, such as a printer's name, come from the network.
// Cell puts an apostrophe in front of such a value, and Undo takes it off again
// so the console's own importers read back what was exported.
package csvsafe

import (
	"encoding/csv"
	"io"
	"strconv"
	"strings"
)

// triggers are the characters spreadsheets read as the start of a formula,
// with the fullwidth forms some of them also accept.
const triggers = "=+-@＝＋－＠"

func needsProtection(s string) bool {
	if strings.HasPrefix(s, "'") {
		return needsProtection(s[1:]) // so Undo gives back exactly what came in
	}
	trimmed := strings.TrimLeft(s, " \t\r\n")
	if trimmed == "" || !strings.ContainsAny(trimmed[:firstRuneLen(trimmed)], triggers) {
		return false
	}
	if len(trimmed) == firstRuneLen(trimmed) {
		return false // a lone "-" is a placeholder, not a formula
	}
	_, err := strconv.ParseFloat(trimmed, 64)
	return err != nil // -5 and +3.25 are numbers and stay numbers
}

func firstRuneLen(s string) int {
	for i := range s {
		if i > 0 {
			return i
		}
	}
	return len(s)
}

// Cell returns s, safe to write as a CSV field.
func Cell(s string) string {
	if needsProtection(s) {
		return "'" + s
	}
	return s
}

// Undo reverses Cell. An apostrophe that protects nothing is the value's own.
func Undo(s string) string {
	if strings.HasPrefix(s, "'") && needsProtection(s[1:]) {
		return s[1:]
	}
	return s
}

// Writer is a csv.Writer that applies Cell to every field, so a new column
// cannot be forgotten.
type Writer struct{ *csv.Writer }

// NewWriter returns a Writer on w.
func NewWriter(w io.Writer) *Writer { return &Writer{csv.NewWriter(w)} }

// Write writes one record with every field made safe.
func (w *Writer) Write(record []string) error {
	safe := make([]string, len(record))
	for i, v := range record {
		safe[i] = Cell(v)
	}
	return w.Writer.Write(safe)
}
