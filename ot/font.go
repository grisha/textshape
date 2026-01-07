package ot

import "encoding/binary"

// Font represents an OpenType font.
type Font struct {
	data   []byte
	tables map[Tag]tableRecord
}

type tableRecord struct {
	offset uint32
	length uint32
}

// ParseFont parses an OpenType font from data.
// For TrueType Collections (.ttc), use index to select a font.
func ParseFont(data []byte, index int) (*Font, error) {
	if len(data) < 12 {
		return nil, ErrInvalidFont
	}

	p := NewParser(data)

	// Check for TTC
	magic, _ := p.U32()
	if magic == 0x74746366 { // 'ttcf'
		return parseTTC(data, index)
	}

	// Single font
	if index != 0 {
		return nil, ErrInvalidFont
	}

	return parseOffsetTable(data, 0)
}

func parseTTC(data []byte, index int) (*Font, error) {
	p := NewParser(data)
	p.Skip(4) // 'ttcf'

	_, err := p.U32() // version
	if err != nil {
		return nil, ErrInvalidFont
	}

	numFonts, err := p.U32()
	if err != nil {
		return nil, ErrInvalidFont
	}

	if index < 0 || index >= int(numFonts) {
		return nil, ErrInvalidFont
	}

	// Read offset for requested font
	p.Skip(index * 4)
	offset, err := p.U32()
	if err != nil {
		return nil, ErrInvalidFont
	}

	return parseOffsetTable(data, int(offset))
}

func parseOffsetTable(data []byte, offset int) (*Font, error) {
	if offset+12 > len(data) {
		return nil, ErrInvalidFont
	}

	p := NewParser(data)
	p.SetOffset(offset)

	sfntVersion, _ := p.U32()
	// Valid: 0x00010000 (TrueType), 'OTTO' (CFF), 'true', 'typ1'
	if sfntVersion != 0x00010000 &&
		sfntVersion != 0x4F54544F && // OTTO
		sfntVersion != 0x74727565 && // true
		sfntVersion != 0x74797031 { // typ1
		return nil, ErrInvalidFont
	}

	numTables, _ := p.U16()
	p.Skip(6) // searchRange, entrySelector, rangeShift

	font := &Font{
		data:   data,
		tables: make(map[Tag]tableRecord, numTables),
	}

	for i := 0; i < int(numTables); i++ {
		tag, _ := p.Tag()
		p.Skip(4) // checksum
		tableOffset, _ := p.U32()
		tableLength, _ := p.U32()

		font.tables[tag] = tableRecord{
			offset: tableOffset,
			length: tableLength,
		}
	}

	return font, nil
}

// HasTable returns true if the font has the given table.
func (f *Font) HasTable(tag Tag) bool {
	_, ok := f.tables[tag]
	return ok
}

// TableData returns the raw data for a table.
func (f *Font) TableData(tag Tag) ([]byte, error) {
	rec, ok := f.tables[tag]
	if !ok {
		return nil, ErrTableNotFound
	}

	end := rec.offset + rec.length
	if end > uint32(len(f.data)) {
		return nil, ErrInvalidTable
	}

	return f.data[rec.offset:end], nil
}

// TableParser returns a parser for the given table.
func (f *Font) TableParser(tag Tag) (*Parser, error) {
	data, err := f.TableData(tag)
	if err != nil {
		return nil, err
	}
	return NewParser(data), nil
}

// NumGlyphs returns the number of glyphs in the font.
// Returns 0 if maxp table is missing or invalid.
func (f *Font) NumGlyphs() int {
	data, err := f.TableData(TagMaxp)
	if err != nil || len(data) < 6 {
		return 0
	}
	return int(binary.BigEndian.Uint16(data[4:]))
}

// GlyphID represents a glyph index.
type GlyphID = uint16

// Codepoint represents a Unicode codepoint.
type Codepoint = uint32
