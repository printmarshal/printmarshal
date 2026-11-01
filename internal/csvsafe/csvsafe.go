// Package csvsafe is defined by the tests until the fix lands.
package csvsafe

import (
	"encoding/csv"
	"io"
)

// Cell is not implemented yet.
func Cell(s string) string { return s }

// Undo is not implemented yet.
func Undo(s string) string { return s }

// Writer is a csv.Writer.
type Writer struct{ *csv.Writer }

// NewWriter returns a Writer on w.
func NewWriter(w io.Writer) *Writer { return &Writer{csv.NewWriter(w)} }
