package ot

import (
	"encoding/binary"
)

// Glyf represents the parsed glyf table (glyph data).
type Glyf struct {
	data []byte
	loca *Loca
}

// Loca represents the parsed loca table (index to location).
type Loca struct {
	offsets   []uint32 // Glyph offsets into glyf table
	numGlyphs int
	isShort   bool // true for short format (16-bit offsets)
}

// GlyphData represents the raw data for a single glyph.
type GlyphData struct {
	Data             []byte
	NumberOfContours int16 // -1 for composite, >= 0 for simple
}

// ParseLoca parses the loca table.
// indexToLocFormat: 0 = short (16-bit), 1 = long (32-bit)
func ParseLoca(data []byte, numGlyphs int, indexToLocFormat int16) (*Loca, error) {
	l := &Loca{
		numGlyphs: numGlyphs,
		isShort:   indexToLocFormat == 0,
	}

	// loca has numGlyphs+1 entries
	numEntries := numGlyphs + 1

	if l.isShort {
		// Short format: 16-bit offsets (actual offset = value * 2)
		if len(data) < numEntries*2 {
			return nil, ErrInvalidOffset
		}
		l.offsets = make([]uint32, numEntries)
		for i := 0; i < numEntries; i++ {
			l.offsets[i] = uint32(binary.BigEndian.Uint16(data[i*2:])) * 2
		}
	} else {
		// Long format: 32-bit offsets
		if len(data) < numEntries*4 {
			return nil, ErrInvalidOffset
		}
		l.offsets = make([]uint32, numEntries)
		for i := 0; i < numEntries; i++ {
			l.offsets[i] = binary.BigEndian.Uint32(data[i*4:])
		}
	}

	return l, nil
}

// GetOffset returns the offset and length for a glyph.
// Returns (offset, length, ok)
func (l *Loca) GetOffset(gid GlyphID) (uint32, uint32, bool) {
	idx := int(gid)
	if idx < 0 || idx >= l.numGlyphs {
		return 0, 0, false
	}
	start := l.offsets[idx]
	end := l.offsets[idx+1]
	return start, end - start, true
}

// NumGlyphs returns the number of glyphs.
func (l *Loca) NumGlyphs() int {
	return l.numGlyphs
}

// IsShort returns true if using short (16-bit) format.
func (l *Loca) IsShort() bool {
	return l.isShort
}

// ParseGlyf parses the glyf table using a loca table.
func ParseGlyf(data []byte, loca *Loca) (*Glyf, error) {
	return &Glyf{
		data: data,
		loca: loca,
	}, nil
}

// GetGlyph returns the glyph data for a glyph ID.
func (g *Glyf) GetGlyph(gid GlyphID) *GlyphData {
	offset, length, ok := g.loca.GetOffset(gid)
	if !ok {
		return nil
	}

	// Empty glyph (like space)
	if length == 0 {
		return &GlyphData{
			Data:             nil,
			NumberOfContours: 0,
		}
	}

	if int(offset)+int(length) > len(g.data) {
		return nil
	}

	data := g.data[offset : offset+length]
	if len(data) < 2 {
		return nil
	}

	numberOfContours := int16(binary.BigEndian.Uint16(data))

	return &GlyphData{
		Data:             data,
		NumberOfContours: numberOfContours,
	}
}

// GetGlyphBytes returns the raw bytes for a glyph.
func (g *Glyf) GetGlyphBytes(gid GlyphID) []byte {
	offset, length, ok := g.loca.GetOffset(gid)
	if !ok || length == 0 {
		return nil
	}
	if int(offset)+int(length) > len(g.data) {
		return nil
	}
	return g.data[offset : offset+length]
}

// IsComposite returns true if the glyph is a composite glyph.
func (gd *GlyphData) IsComposite() bool {
	return gd.NumberOfContours < 0
}

// Composite glyph flags
const (
	argAreWords     uint16 = 0x0001 // Args are words (otherwise bytes)
	argsAreXYValues uint16 = 0x0002 // Args are xy values (otherwise points)
	roundXYToGrid   uint16 = 0x0004
	weHaveAScale    uint16 = 0x0008 // Scale value present
	moreComponents  uint16 = 0x0020 // More components follow
	weHaveXYScale   uint16 = 0x0040 // Separate X and Y scale
	weHave2x2       uint16 = 0x0080 // 2x2 transform matrix
	weHaveInstr     uint16 = 0x0100 // Instructions follow
	useMyMetrics    uint16 = 0x0200
	overlapCompound uint16 = 0x0400
)

// CompositeComponent represents a component in a composite glyph.
type CompositeComponent struct {
	GlyphID GlyphID
	Flags   uint16
	Arg1    int16
	Arg2    int16
	// Transform matrix components (optional)
	Scale   float32
	ScaleX  float32
	ScaleY  float32
	Scale01 float32
	Scale10 float32
}

// GetComponents returns the component glyph IDs for a composite glyph.
// For simple glyphs, returns nil.
func (g *Glyf) GetComponents(gid GlyphID) []GlyphID {
	glyph := g.GetGlyph(gid)
	if glyph == nil || !glyph.IsComposite() {
		return nil
	}

	components := g.parseComposite(glyph.Data)
	result := make([]GlyphID, len(components))
	for i, comp := range components {
		result[i] = comp.GlyphID
	}
	return result
}

// parseComposite parses composite glyph components.
func (g *Glyf) parseComposite(data []byte) []CompositeComponent {
	if len(data) < 10 {
		return nil
	}

	// Skip glyph header (10 bytes: numberOfContours, xMin, yMin, xMax, yMax)
	offset := 10
	var components []CompositeComponent

	for {
		if offset+4 > len(data) {
			break
		}

		flags := binary.BigEndian.Uint16(data[offset:])
		glyphIndex := GlyphID(binary.BigEndian.Uint16(data[offset+2:]))
		offset += 4

		comp := CompositeComponent{
			GlyphID: glyphIndex,
			Flags:   flags,
		}

		// Parse arguments
		if flags&argAreWords != 0 {
			if offset+4 > len(data) {
				break
			}
			comp.Arg1 = int16(binary.BigEndian.Uint16(data[offset:]))
			comp.Arg2 = int16(binary.BigEndian.Uint16(data[offset+2:]))
			offset += 4
		} else {
			if offset+2 > len(data) {
				break
			}
			comp.Arg1 = int16(int8(data[offset]))
			comp.Arg2 = int16(int8(data[offset+1]))
			offset += 2
		}

		// Skip transform components (we just need glyph IDs for closure)
		if flags&weHaveAScale != 0 {
			offset += 2 // F2Dot14
		} else if flags&weHaveXYScale != 0 {
			offset += 4 // 2 x F2Dot14
		} else if flags&weHave2x2 != 0 {
			offset += 8 // 4 x F2Dot14
		}

		components = append(components, comp)

		if flags&moreComponents == 0 {
			break
		}
	}

	return components
}

// RemapComposite creates a new composite glyph with remapped component IDs.
func RemapComposite(data []byte, glyphMap map[GlyphID]GlyphID) []byte {
	if len(data) < 10 {
		return data
	}

	// Check if this is a composite
	numberOfContours := int16(binary.BigEndian.Uint16(data))
	if numberOfContours >= 0 {
		// Simple glyph, no remapping needed
		return data
	}

	// Make a copy to modify
	result := make([]byte, len(data))
	copy(result, data)

	// Parse and remap component glyph IDs
	offset := 10
	for {
		if offset+4 > len(result) {
			break
		}

		flags := binary.BigEndian.Uint16(result[offset:])
		oldGID := GlyphID(binary.BigEndian.Uint16(result[offset+2:]))

		// Remap the glyph ID
		if newGID, ok := glyphMap[oldGID]; ok {
			binary.BigEndian.PutUint16(result[offset+2:], uint16(newGID))
		}

		offset += 4

		// Skip arguments
		if flags&argAreWords != 0 {
			offset += 4
		} else {
			offset += 2
		}

		// Skip transform components
		if flags&weHaveAScale != 0 {
			offset += 2
		} else if flags&weHaveXYScale != 0 {
			offset += 4
		} else if flags&weHave2x2 != 0 {
			offset += 8
		}

		if flags&moreComponents == 0 {
			break
		}
	}

	return result
}

// BuildLoca builds a loca table from glyph offsets.
// If useShort is true, uses 16-bit format (offsets must be even and < 131072).
func BuildLoca(offsets []uint32, useShort bool) []byte {
	if useShort {
		data := make([]byte, len(offsets)*2)
		for i, off := range offsets {
			binary.BigEndian.PutUint16(data[i*2:], uint16(off/2))
		}
		return data
	}

	data := make([]byte, len(offsets)*4)
	for i, off := range offsets {
		binary.BigEndian.PutUint32(data[i*4:], off)
	}
	return data
}

// ParseGlyfFromFont parses both glyf and loca tables from a font.
func ParseGlyfFromFont(font *Font) (*Glyf, error) {
	// Get numGlyphs from maxp
	maxpData, err := font.TableData(TagMaxp)
	if err != nil {
		return nil, err
	}
	if len(maxpData) < 6 {
		return nil, ErrInvalidTable
	}
	numGlyphs := int(binary.BigEndian.Uint16(maxpData[4:]))

	// Get indexToLocFormat from head
	headData, err := font.TableData(TagHead)
	if err != nil {
		return nil, err
	}
	if len(headData) < 54 {
		return nil, ErrInvalidTable
	}
	indexToLocFormat := int16(binary.BigEndian.Uint16(headData[50:]))

	// Parse loca
	locaData, err := font.TableData(TagLoca)
	if err != nil {
		return nil, err
	}
	loca, err := ParseLoca(locaData, numGlyphs, indexToLocFormat)
	if err != nil {
		return nil, err
	}

	// Parse glyf
	glyfData, err := font.TableData(TagGlyf)
	if err != nil {
		return nil, err
	}

	return ParseGlyf(glyfData, loca)
}
