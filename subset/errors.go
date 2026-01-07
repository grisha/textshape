package subset

import "errors"

var (
	// ErrNoTables is returned when building a font with no tables.
	ErrNoTables = errors.New("subset: no tables to build")

	// ErrMissingTable is returned when a required table is missing.
	ErrMissingTable = errors.New("subset: required table missing")

	// ErrInvalidGlyph is returned for invalid glyph references.
	ErrInvalidGlyph = errors.New("subset: invalid glyph reference")
)
