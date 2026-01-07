package ot

import "encoding/binary"

// Hmtx represents the horizontal metrics table.
type Hmtx struct {
	// hMetrics contains advanceWidth and lsb for glyphs 0..numberOfHMetrics-1
	hMetrics []LongHorMetric
	// leftSideBearings for glyphs numberOfHMetrics..numGlyphs-1
	// (these glyphs share the last advanceWidth)
	leftSideBearings []int16
	// lastAdvanceWidth is cached for glyphs >= numberOfHMetrics
	lastAdvanceWidth uint16
}

// LongHorMetric contains the advance width and left side bearing for a glyph.
type LongHorMetric struct {
	AdvanceWidth uint16
	Lsb          int16 // Left side bearing
}

// ParseHmtx parses the hmtx table.
// It requires numberOfHMetrics from hhea and numGlyphs from maxp.
func ParseHmtx(data []byte, numberOfHMetrics, numGlyphs int) (*Hmtx, error) {
	if numberOfHMetrics <= 0 {
		return nil, ErrInvalidTable
	}

	// Calculate expected size
	// numberOfHMetrics * 4 (LongHorMetric) + (numGlyphs - numberOfHMetrics) * 2 (int16)
	expectedSize := numberOfHMetrics*4 + (numGlyphs-numberOfHMetrics)*2
	if len(data) < expectedSize {
		return nil, ErrInvalidTable
	}

	h := &Hmtx{
		hMetrics:         make([]LongHorMetric, numberOfHMetrics),
		leftSideBearings: make([]int16, numGlyphs-numberOfHMetrics),
	}

	// Parse LongHorMetrics
	off := 0
	for i := 0; i < numberOfHMetrics; i++ {
		h.hMetrics[i].AdvanceWidth = binary.BigEndian.Uint16(data[off:])
		h.hMetrics[i].Lsb = int16(binary.BigEndian.Uint16(data[off+2:]))
		off += 4
	}

	// Cache last advance width
	if numberOfHMetrics > 0 {
		h.lastAdvanceWidth = h.hMetrics[numberOfHMetrics-1].AdvanceWidth
	}

	// Parse remaining left side bearings
	for i := 0; i < numGlyphs-numberOfHMetrics; i++ {
		h.leftSideBearings[i] = int16(binary.BigEndian.Uint16(data[off:]))
		off += 2
	}

	return h, nil
}

// GetAdvanceWidth returns the advance width for a glyph.
func (h *Hmtx) GetAdvanceWidth(glyph GlyphID) uint16 {
	if int(glyph) < len(h.hMetrics) {
		return h.hMetrics[glyph].AdvanceWidth
	}
	// Glyphs beyond numberOfHMetrics use the last advance width
	return h.lastAdvanceWidth
}

// GetLsb returns the left side bearing for a glyph.
func (h *Hmtx) GetLsb(glyph GlyphID) int16 {
	if int(glyph) < len(h.hMetrics) {
		return h.hMetrics[glyph].Lsb
	}
	// Check in leftSideBearings array
	idx := int(glyph) - len(h.hMetrics)
	if idx >= 0 && idx < len(h.leftSideBearings) {
		return h.leftSideBearings[idx]
	}
	return 0
}

// GetMetrics returns both advance width and lsb for a glyph.
func (h *Hmtx) GetMetrics(glyph GlyphID) (advanceWidth uint16, lsb int16) {
	return h.GetAdvanceWidth(glyph), h.GetLsb(glyph)
}

// Hhea represents the horizontal header table.
type Hhea struct {
	Version             uint32
	Ascender            int16
	Descender           int16
	LineGap             int16
	AdvanceWidthMax     uint16
	MinLeftSideBearing  int16
	MinRightSideBearing int16
	XMaxExtent          int16
	CaretSlopeRise      int16
	CaretSlopeRun       int16
	CaretOffset         int16
	MetricDataFormat    int16
	NumberOfHMetrics    uint16
}

// ParseHhea parses the hhea (horizontal header) table.
func ParseHhea(data []byte) (*Hhea, error) {
	if len(data) < 36 {
		return nil, ErrInvalidTable
	}

	h := &Hhea{
		Version:             binary.BigEndian.Uint32(data[0:]),
		Ascender:            int16(binary.BigEndian.Uint16(data[4:])),
		Descender:           int16(binary.BigEndian.Uint16(data[6:])),
		LineGap:             int16(binary.BigEndian.Uint16(data[8:])),
		AdvanceWidthMax:     binary.BigEndian.Uint16(data[10:]),
		MinLeftSideBearing:  int16(binary.BigEndian.Uint16(data[12:])),
		MinRightSideBearing: int16(binary.BigEndian.Uint16(data[14:])),
		XMaxExtent:          int16(binary.BigEndian.Uint16(data[16:])),
		CaretSlopeRise:      int16(binary.BigEndian.Uint16(data[18:])),
		CaretSlopeRun:       int16(binary.BigEndian.Uint16(data[20:])),
		CaretOffset:         int16(binary.BigEndian.Uint16(data[22:])),
		// 24-30: reserved (4 int16)
		MetricDataFormat: int16(binary.BigEndian.Uint16(data[32:])),
		NumberOfHMetrics: binary.BigEndian.Uint16(data[34:]),
	}

	return h, nil
}

// ParseHmtxFromFont is a convenience function that parses hmtx from a font,
// automatically reading hhea and maxp for required values.
func ParseHmtxFromFont(font *Font) (*Hmtx, error) {
	// Parse hhea to get numberOfHMetrics
	hheaData, err := font.TableData(TagHhea)
	if err != nil {
		return nil, err
	}
	hhea, err := ParseHhea(hheaData)
	if err != nil {
		return nil, err
	}

	// Get numGlyphs from maxp
	numGlyphs := font.NumGlyphs()
	if numGlyphs == 0 {
		return nil, ErrInvalidTable
	}

	// Parse hmtx
	hmtxData, err := font.TableData(TagHmtx)
	if err != nil {
		return nil, err
	}

	return ParseHmtx(hmtxData, int(hhea.NumberOfHMetrics), numGlyphs)
}
