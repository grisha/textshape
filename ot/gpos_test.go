package ot

import (
	"encoding/binary"
	"testing"
)

// Helper to write signed int16 as big-endian
func putInt16(b []byte, v int16) {
	binary.BigEndian.PutUint16(b, uint16(v))
}

func TestValueFormatLen(t *testing.T) {
	tests := []struct {
		format uint16
		want   int
	}{
		{0, 0},
		{ValueFormatXAdvance, 1},
		{ValueFormatXPlacement | ValueFormatYPlacement, 2},
		{ValueFormatXPlacement | ValueFormatYPlacement | ValueFormatXAdvance | ValueFormatYAdvance, 4},
		{0xFF, 8}, // All flags
	}

	for _, tt := range tests {
		got := valueFormatLen(tt.format)
		if got != tt.want {
			t.Errorf("valueFormatLen(0x%04X) = %d, want %d", tt.format, got, tt.want)
		}
	}
}

func TestParseValueRecord(t *testing.T) {
	// Build data: xPlacement=10, yPlacement=20, xAdvance=-30, yAdvance=0
	data := make([]byte, 8)
	binary.BigEndian.PutUint16(data[0:], 10)
	binary.BigEndian.PutUint16(data[2:], 20)
	putInt16(data[4:], -30)
	binary.BigEndian.PutUint16(data[6:], 0)

	format := uint16(ValueFormatXPlacement | ValueFormatYPlacement | ValueFormatXAdvance | ValueFormatYAdvance)
	vr, size := parseValueRecord(data, 0, format)

	if size != 8 {
		t.Errorf("size = %d, want 8", size)
	}
	if vr.XPlacement != 10 {
		t.Errorf("XPlacement = %d, want 10", vr.XPlacement)
	}
	if vr.YPlacement != 20 {
		t.Errorf("YPlacement = %d, want 20", vr.YPlacement)
	}
	if vr.XAdvance != -30 {
		t.Errorf("XAdvance = %d, want -30", vr.XAdvance)
	}
	if vr.YAdvance != 0 {
		t.Errorf("YAdvance = %d, want 0", vr.YAdvance)
	}
}

func TestParseValueRecordPartial(t *testing.T) {
	// Only xAdvance
	data := make([]byte, 2)
	putInt16(data[0:], -50)

	format := uint16(ValueFormatXAdvance)
	vr, size := parseValueRecord(data, 0, format)

	if size != 2 {
		t.Errorf("size = %d, want 2", size)
	}
	if vr.XPlacement != 0 {
		t.Errorf("XPlacement = %d, want 0", vr.XPlacement)
	}
	if vr.XAdvance != -50 {
		t.Errorf("XAdvance = %d, want -50", vr.XAdvance)
	}
}

// Build a SinglePos Format 1 subtable
func buildSinglePosFormat1(coverageGlyphs []GlyphID, valueFormat uint16, vr ValueRecord) []byte {
	coverage := buildCoverageFormat1(coverageGlyphs)

	// Calculate header size: format(2) + coverageOffset(2) + valueFormat(2) + valueRecord
	vrSize := valueFormatSize(valueFormat)
	headerSize := 6 + vrSize

	data := make([]byte, headerSize+len(coverage))
	binary.BigEndian.PutUint16(data[0:], 1)                  // format
	binary.BigEndian.PutUint16(data[2:], uint16(headerSize)) // coverage offset
	binary.BigEndian.PutUint16(data[4:], valueFormat)

	// Write value record
	off := 6
	if valueFormat&ValueFormatXPlacement != 0 {
		binary.BigEndian.PutUint16(data[off:], uint16(vr.XPlacement))
		off += 2
	}
	if valueFormat&ValueFormatYPlacement != 0 {
		binary.BigEndian.PutUint16(data[off:], uint16(vr.YPlacement))
		off += 2
	}
	if valueFormat&ValueFormatXAdvance != 0 {
		binary.BigEndian.PutUint16(data[off:], uint16(vr.XAdvance))
		off += 2
	}
	if valueFormat&ValueFormatYAdvance != 0 {
		binary.BigEndian.PutUint16(data[off:], uint16(vr.YAdvance))
		off += 2
	}

	copy(data[headerSize:], coverage)
	return data
}

// Build a SinglePos Format 2 subtable
func buildSinglePosFormat2(coverageGlyphs []GlyphID, valueFormat uint16, vrs []ValueRecord) []byte {
	coverage := buildCoverageFormat1(coverageGlyphs)

	vrSize := valueFormatSize(valueFormat)
	headerSize := 8 + len(vrs)*vrSize

	data := make([]byte, headerSize+len(coverage))
	binary.BigEndian.PutUint16(data[0:], 2)                  // format
	binary.BigEndian.PutUint16(data[2:], uint16(headerSize)) // coverage offset
	binary.BigEndian.PutUint16(data[4:], valueFormat)
	binary.BigEndian.PutUint16(data[6:], uint16(len(vrs)))

	off := 8
	for _, vr := range vrs {
		if valueFormat&ValueFormatXPlacement != 0 {
			binary.BigEndian.PutUint16(data[off:], uint16(vr.XPlacement))
			off += 2
		}
		if valueFormat&ValueFormatYPlacement != 0 {
			binary.BigEndian.PutUint16(data[off:], uint16(vr.YPlacement))
			off += 2
		}
		if valueFormat&ValueFormatXAdvance != 0 {
			binary.BigEndian.PutUint16(data[off:], uint16(vr.XAdvance))
			off += 2
		}
		if valueFormat&ValueFormatYAdvance != 0 {
			binary.BigEndian.PutUint16(data[off:], uint16(vr.YAdvance))
			off += 2
		}
	}

	copy(data[headerSize:], coverage)
	return data
}

func TestSinglePosFormat1(t *testing.T) {
	// All covered glyphs get xAdvance = -50
	coverageGlyphs := []GlyphID{65, 66, 67} // A, B, C
	valueFormat := uint16(ValueFormatXAdvance)
	vr := ValueRecord{XAdvance: -50}

	data := buildSinglePosFormat1(coverageGlyphs, valueFormat, vr)

	sp, err := parseSinglePos(data, 0)
	if err != nil {
		t.Fatalf("parseSinglePos failed: %v", err)
	}

	// Test application
	glyphs := []GlyphID{65, 66, 68} // A, B, D (D not covered)
	positions := make([]GlyphPosition, len(glyphs))

	ctx := &GPOSContext{
		Glyphs:    glyphs,
		Positions: positions,
		Index:     0,
	}

	// Apply to first glyph
	if !sp.Apply(ctx) {
		t.Error("Apply returned false for covered glyph")
	}
	if positions[0].XAdvance != -50 {
		t.Errorf("positions[0].XAdvance = %d, want -50", positions[0].XAdvance)
	}

	// Apply to second glyph (ctx.Index should have advanced)
	if ctx.Index != 1 {
		t.Errorf("Index = %d, want 1", ctx.Index)
	}
	if !sp.Apply(ctx) {
		t.Error("Apply returned false for covered glyph")
	}
	if positions[1].XAdvance != -50 {
		t.Errorf("positions[1].XAdvance = %d, want -50", positions[1].XAdvance)
	}

	// Third glyph not covered
	if ctx.Index != 2 {
		t.Errorf("Index = %d, want 2", ctx.Index)
	}
	if sp.Apply(ctx) {
		t.Error("Apply returned true for uncovered glyph")
	}
}

func TestSinglePosFormat2(t *testing.T) {
	// Each covered glyph gets different xAdvance
	coverageGlyphs := []GlyphID{65, 66, 67}
	valueFormat := uint16(ValueFormatXAdvance)
	vrs := []ValueRecord{
		{XAdvance: -10},
		{XAdvance: -20},
		{XAdvance: -30},
	}

	data := buildSinglePosFormat2(coverageGlyphs, valueFormat, vrs)

	sp, err := parseSinglePos(data, 0)
	if err != nil {
		t.Fatalf("parseSinglePos failed: %v", err)
	}

	tests := []struct {
		glyph GlyphID
		want  int16
	}{
		{65, -10},
		{66, -20},
		{67, -30},
	}

	for _, tt := range tests {
		positions := make([]GlyphPosition, 1)
		ctx := &GPOSContext{
			Glyphs:    []GlyphID{tt.glyph},
			Positions: positions,
			Index:     0,
		}

		if !sp.Apply(ctx) {
			t.Errorf("Apply(%d) returned false", tt.glyph)
			continue
		}
		if positions[0].XAdvance != tt.want {
			t.Errorf("Apply(%d): XAdvance = %d, want %d", tt.glyph, positions[0].XAdvance, tt.want)
		}
	}
}

// Build a PairPos Format 1 subtable
func buildPairPosFormat1(firstGlyphs []GlyphID, pairs [][]struct {
	second GlyphID
	kern   int16
}) []byte {
	coverage := buildCoverageFormat1(firstGlyphs)

	// valueFormat: only xAdvance for first glyph
	valueFormat1 := uint16(ValueFormatXAdvance)
	valueFormat2 := uint16(0)

	// Calculate sizes
	pairSetCount := len(pairs)
	headerSize := 10 + pairSetCount*2 // format + covOff + vf1 + vf2 + count + offsets

	// Calculate pair set sizes
	pairSetOffsets := make([]int, pairSetCount)
	currentOff := headerSize
	for i, ps := range pairs {
		pairSetOffsets[i] = currentOff
		currentOff += 2 + len(ps)*4 // count + (secondGlyph + xAdvance) per pair
	}

	totalSize := currentOff + len(coverage)
	data := make([]byte, totalSize)

	// Header
	binary.BigEndian.PutUint16(data[0:], 1)                  // format
	binary.BigEndian.PutUint16(data[2:], uint16(currentOff)) // coverage offset
	binary.BigEndian.PutUint16(data[4:], valueFormat1)
	binary.BigEndian.PutUint16(data[6:], valueFormat2)
	binary.BigEndian.PutUint16(data[8:], uint16(pairSetCount))

	// PairSet offsets
	for i, off := range pairSetOffsets {
		binary.BigEndian.PutUint16(data[10+i*2:], uint16(off))
	}

	// PairSets
	for i, ps := range pairs {
		off := pairSetOffsets[i]
		binary.BigEndian.PutUint16(data[off:], uint16(len(ps)))
		off += 2
		for _, p := range ps {
			binary.BigEndian.PutUint16(data[off:], uint16(p.second))
			binary.BigEndian.PutUint16(data[off+2:], uint16(p.kern))
			off += 4
		}
	}

	// Coverage
	copy(data[currentOff:], coverage)

	return data
}

func TestPairPosFormat1(t *testing.T) {
	// Build kerning: A+V = -80, A+W = -60, V+A = -70
	firstGlyphs := []GlyphID{65, 86} // A, V
	pairs := [][]struct {
		second GlyphID
		kern   int16
	}{
		{{86, -80}, {87, -60}}, // A+V, A+W
		{{65, -70}},            // V+A
	}

	data := buildPairPosFormat1(firstGlyphs, pairs)

	pp, err := parsePairPos(data, 0)
	if err != nil {
		t.Fatalf("parsePairPos failed: %v", err)
	}

	tests := []struct {
		glyphs []GlyphID
		kern0  int16
		desc   string
	}{
		{[]GlyphID{65, 86}, -80, "A+V"},
		{[]GlyphID{65, 87}, -60, "A+W"},
		{[]GlyphID{86, 65}, -70, "V+A"},
		{[]GlyphID{65, 65}, 0, "A+A (no pair)"},
	}

	for _, tt := range tests {
		positions := make([]GlyphPosition, len(tt.glyphs))
		ctx := &GPOSContext{
			Glyphs:    tt.glyphs,
			Positions: positions,
			Index:     0,
		}

		applied := pp.Apply(ctx)
		if tt.kern0 != 0 {
			if !applied {
				t.Errorf("%s: Apply returned false", tt.desc)
				continue
			}
			if positions[0].XAdvance != tt.kern0 {
				t.Errorf("%s: XAdvance = %d, want %d", tt.desc, positions[0].XAdvance, tt.kern0)
			}
		} else {
			if applied && positions[0].XAdvance != 0 {
				t.Errorf("%s: unexpected kerning %d", tt.desc, positions[0].XAdvance)
			}
		}
	}
}

// Build a ClassDef Format 1 table
func buildClassDefFormat1(startGlyph GlyphID, classes []uint16) []byte {
	data := make([]byte, 6+len(classes)*2)
	binary.BigEndian.PutUint16(data[0:], 1) // format
	binary.BigEndian.PutUint16(data[2:], uint16(startGlyph))
	binary.BigEndian.PutUint16(data[4:], uint16(len(classes)))
	for i, c := range classes {
		binary.BigEndian.PutUint16(data[6+i*2:], c)
	}
	return data
}

// Build a ClassDef Format 2 table
func buildClassDefFormat2(ranges []struct {
	start, end GlyphID
	class      uint16
}) []byte {
	data := make([]byte, 4+len(ranges)*6)
	binary.BigEndian.PutUint16(data[0:], 2) // format
	binary.BigEndian.PutUint16(data[2:], uint16(len(ranges)))
	for i, r := range ranges {
		off := 4 + i*6
		binary.BigEndian.PutUint16(data[off:], uint16(r.start))
		binary.BigEndian.PutUint16(data[off+2:], uint16(r.end))
		binary.BigEndian.PutUint16(data[off+4:], r.class)
	}
	return data
}

func TestClassDefFormat1(t *testing.T) {
	// Glyphs 65-69 (A-E) have classes 1, 2, 3, 2, 1
	classes := []uint16{1, 2, 3, 2, 1}
	data := buildClassDefFormat1(65, classes)

	cd, err := ParseClassDef(data, 0)
	if err != nil {
		t.Fatalf("ParseClassDef failed: %v", err)
	}

	tests := []struct {
		glyph GlyphID
		want  int
	}{
		{65, 1}, // A
		{66, 2}, // B
		{67, 3}, // C
		{68, 2}, // D
		{69, 1}, // E
		{64, 0}, // Before range -> class 0
		{70, 0}, // After range -> class 0
	}

	for _, tt := range tests {
		got := cd.GetClass(tt.glyph)
		if got != tt.want {
			t.Errorf("GetClass(%d) = %d, want %d", tt.glyph, got, tt.want)
		}
	}
}

func TestClassDefFormat2(t *testing.T) {
	// A-C = class 1, D-F = class 2, X-Z = class 3
	ranges := []struct {
		start, end GlyphID
		class      uint16
	}{
		{65, 67, 1}, // A-C
		{68, 70, 2}, // D-F
		{88, 90, 3}, // X-Z
	}
	data := buildClassDefFormat2(ranges)

	cd, err := ParseClassDef(data, 0)
	if err != nil {
		t.Fatalf("ParseClassDef failed: %v", err)
	}

	tests := []struct {
		glyph GlyphID
		want  int
	}{
		{65, 1}, // A
		{67, 1}, // C
		{68, 2}, // D
		{70, 2}, // F
		{88, 3}, // X
		{90, 3}, // Z
		{71, 0}, // G -> class 0
		{87, 0}, // W -> class 0
	}

	for _, tt := range tests {
		got := cd.GetClass(tt.glyph)
		if got != tt.want {
			t.Errorf("GetClass(%d) = %d, want %d", tt.glyph, got, tt.want)
		}
	}
}

// Build a GPOS table for testing
func buildGPOSTable(lookups [][]byte) []byte {
	headerSize := 10
	scriptListSize := 2
	featureListSize := 2

	lookupListHeaderSize := 2 + len(lookups)*2
	lookupListSize := lookupListHeaderSize
	for _, l := range lookups {
		lookupListSize += len(l)
	}

	totalSize := headerSize + scriptListSize + featureListSize + lookupListSize
	data := make([]byte, totalSize)

	// Header
	binary.BigEndian.PutUint16(data[0:], 1) // version major
	binary.BigEndian.PutUint16(data[2:], 0) // version minor
	binary.BigEndian.PutUint16(data[4:], uint16(headerSize))
	binary.BigEndian.PutUint16(data[6:], uint16(headerSize+scriptListSize))
	binary.BigEndian.PutUint16(data[8:], uint16(headerSize+scriptListSize+featureListSize))

	// Empty ScriptList
	binary.BigEndian.PutUint16(data[headerSize:], 0)

	// Empty FeatureList
	binary.BigEndian.PutUint16(data[headerSize+scriptListSize:], 0)

	// LookupList
	lookupListOff := headerSize + scriptListSize + featureListSize
	binary.BigEndian.PutUint16(data[lookupListOff:], uint16(len(lookups)))

	offset := lookupListHeaderSize
	for i, l := range lookups {
		binary.BigEndian.PutUint16(data[lookupListOff+2+i*2:], uint16(offset))
		copy(data[lookupListOff+offset:], l)
		offset += len(l)
	}

	return data
}

// Build a GPOS lookup wrapper
func buildGPOSLookup(lookupType uint16, subtables [][]byte) []byte {
	headerSize := 6 + len(subtables)*2
	totalSize := headerSize
	for _, st := range subtables {
		totalSize += len(st)
	}

	data := make([]byte, totalSize)
	binary.BigEndian.PutUint16(data[0:], lookupType)
	binary.BigEndian.PutUint16(data[2:], 0) // flag
	binary.BigEndian.PutUint16(data[4:], uint16(len(subtables)))

	offset := headerSize
	for i, st := range subtables {
		binary.BigEndian.PutUint16(data[6+i*2:], uint16(offset))
		copy(data[offset:], st)
		offset += len(st)
	}

	return data
}

func TestParseGPOS(t *testing.T) {
	// Build a GPOS with one SinglePos lookup
	subtable := buildSinglePosFormat1([]GlyphID{65, 66}, ValueFormatXAdvance, ValueRecord{XAdvance: -50})
	lookup := buildGPOSLookup(GPOSTypeSingle, [][]byte{subtable})
	gposData := buildGPOSTable([][]byte{lookup})

	gpos, err := ParseGPOS(gposData)
	if err != nil {
		t.Fatalf("ParseGPOS failed: %v", err)
	}

	if gpos.NumLookups() != 1 {
		t.Errorf("NumLookups = %d, want 1", gpos.NumLookups())
	}

	// Apply lookup
	glyphs := []GlyphID{65, 66, 67}
	positions := make([]GlyphPosition, len(glyphs))
	gpos.ApplyLookup(0, glyphs, positions)

	// 65 and 66 should have -50 xAdvance, 67 should be 0
	if positions[0].XAdvance != -50 {
		t.Errorf("positions[0].XAdvance = %d, want -50", positions[0].XAdvance)
	}
	if positions[1].XAdvance != -50 {
		t.Errorf("positions[1].XAdvance = %d, want -50", positions[1].XAdvance)
	}
	if positions[2].XAdvance != 0 {
		t.Errorf("positions[2].XAdvance = %d, want 0", positions[2].XAdvance)
	}
}

func TestGPOSApplyKerning(t *testing.T) {
	// Build kerning: A+V = -80
	firstGlyphs := []GlyphID{65}
	pairs := [][]struct {
		second GlyphID
		kern   int16
	}{
		{{86, -80}}, // A+V
	}
	subtable := buildPairPosFormat1(firstGlyphs, pairs)
	lookup := buildGPOSLookup(GPOSTypePair, [][]byte{subtable})
	gposData := buildGPOSTable([][]byte{lookup})

	gpos, err := ParseGPOS(gposData)
	if err != nil {
		t.Fatalf("ParseGPOS failed: %v", err)
	}

	glyphs := []GlyphID{65, 86, 65, 65} // A V A A
	positions := gpos.ApplyKerning(glyphs)

	// First A should have kerning -80 (before V)
	if positions[0].XAdvance != -80 {
		t.Errorf("positions[0].XAdvance = %d, want -80", positions[0].XAdvance)
	}
	// V should have no adjustment
	if positions[1].XAdvance != 0 {
		t.Errorf("positions[1].XAdvance = %d, want 0", positions[1].XAdvance)
	}
	// Second A has no V following, so no kerning
	if positions[2].XAdvance != 0 {
		t.Errorf("positions[2].XAdvance = %d, want 0", positions[2].XAdvance)
	}
}

func TestGPOSContext(t *testing.T) {
	ctx := &GPOSContext{
		Glyphs:    []GlyphID{1, 2, 3},
		Positions: make([]GlyphPosition, 3),
		Index:     0,
	}

	// Test AdjustPosition
	vr := &ValueRecord{XPlacement: 10, YPlacement: 20, XAdvance: -30, YAdvance: 5}
	ctx.AdjustPosition(1, vr)

	if ctx.Positions[1].XPlacement != 10 {
		t.Errorf("XPlacement = %d, want 10", ctx.Positions[1].XPlacement)
	}
	if ctx.Positions[1].YPlacement != 20 {
		t.Errorf("YPlacement = %d, want 20", ctx.Positions[1].YPlacement)
	}
	if ctx.Positions[1].XAdvance != -30 {
		t.Errorf("XAdvance = %d, want -30", ctx.Positions[1].XAdvance)
	}
	if ctx.Positions[1].YAdvance != 5 {
		t.Errorf("YAdvance = %d, want 5", ctx.Positions[1].YAdvance)
	}

	// Adjustments accumulate
	ctx.AdjustPosition(1, vr)
	if ctx.Positions[1].XAdvance != -60 {
		t.Errorf("XAdvance after second adjust = %d, want -60", ctx.Positions[1].XAdvance)
	}
}

// --- MarkBasePos Tests ---

// Helper to build an Anchor table (Format 1)
func buildAnchor(x, y int16) []byte {
	data := make([]byte, 6)
	binary.BigEndian.PutUint16(data[0:], 1) // format
	binary.BigEndian.PutUint16(data[2:], uint16(x))
	binary.BigEndian.PutUint16(data[4:], uint16(y))
	return data
}

// Helper to build a MarkArray
func buildMarkArray(records []struct {
	class  uint16
	anchor []byte
}) []byte {
	// MarkArray: count(2) + records(4 each) + anchor tables
	headerSize := 2 + len(records)*4
	totalSize := headerSize
	for _, r := range records {
		totalSize += len(r.anchor)
	}

	data := make([]byte, totalSize)
	binary.BigEndian.PutUint16(data[0:], uint16(len(records)))

	anchorOff := headerSize
	for i, r := range records {
		recOff := 2 + i*4
		binary.BigEndian.PutUint16(data[recOff:], r.class)
		binary.BigEndian.PutUint16(data[recOff+2:], uint16(anchorOff))
		copy(data[anchorOff:], r.anchor)
		anchorOff += len(r.anchor)
	}

	return data
}

// Helper to build a BaseArray (AnchorMatrix)
func buildBaseArray(rows int, classCount int, anchors [][]*struct{ x, y int16 }) []byte {
	// BaseArray: rows(2) + offsets(2 each) + anchor tables
	totalAnchors := rows * classCount
	headerSize := 2 + totalAnchors*2

	// Count non-nil anchors for size calculation
	anchorSize := 0
	for _, row := range anchors {
		for _, a := range row {
			if a != nil {
				anchorSize += 6
			}
		}
	}

	data := make([]byte, headerSize+anchorSize)
	binary.BigEndian.PutUint16(data[0:], uint16(rows))

	anchorOff := headerSize
	for row := 0; row < rows; row++ {
		for col := 0; col < classCount; col++ {
			idx := row*classCount + col
			offPos := 2 + idx*2

			if row < len(anchors) && col < len(anchors[row]) && anchors[row][col] != nil {
				binary.BigEndian.PutUint16(data[offPos:], uint16(anchorOff))
				// Write anchor
				binary.BigEndian.PutUint16(data[anchorOff:], 1) // format
				binary.BigEndian.PutUint16(data[anchorOff+2:], uint16(anchors[row][col].x))
				binary.BigEndian.PutUint16(data[anchorOff+4:], uint16(anchors[row][col].y))
				anchorOff += 6
			} else {
				binary.BigEndian.PutUint16(data[offPos:], 0) // NULL
			}
		}
	}

	return data
}

// Helper to build a MarkBasePos subtable
func buildMarkBasePos(
	markGlyphs []GlyphID,
	baseGlyphs []GlyphID,
	classCount int,
	markRecords []struct {
		class uint16
		x, y  int16
	},
	baseAnchors [][]*struct{ x, y int16 },
) []byte {
	markCoverage := buildCoverageFormat1(markGlyphs)
	baseCoverage := buildCoverageFormat1(baseGlyphs)

	// Build mark array
	markRecs := make([]struct {
		class  uint16
		anchor []byte
	}, len(markRecords))
	for i, r := range markRecords {
		markRecs[i] = struct {
			class  uint16
			anchor []byte
		}{
			class:  r.class,
			anchor: buildAnchor(r.x, r.y),
		}
	}
	markArray := buildMarkArray(markRecs)

	// Build base array
	baseArray := buildBaseArray(len(baseGlyphs), classCount, baseAnchors)

	// Calculate offsets
	// Header: format(2) + markCoverageOff(2) + baseCoverageOff(2) + classCount(2) + markArrayOff(2) + baseArrayOff(2) = 12
	headerSize := 12
	markCovOff := headerSize
	baseCovOff := markCovOff + len(markCoverage)
	markArrayOff := baseCovOff + len(baseCoverage)
	baseArrayOff := markArrayOff + len(markArray)

	totalSize := baseArrayOff + len(baseArray)
	data := make([]byte, totalSize)

	binary.BigEndian.PutUint16(data[0:], 1) // format
	binary.BigEndian.PutUint16(data[2:], uint16(markCovOff))
	binary.BigEndian.PutUint16(data[4:], uint16(baseCovOff))
	binary.BigEndian.PutUint16(data[6:], uint16(classCount))
	binary.BigEndian.PutUint16(data[8:], uint16(markArrayOff))
	binary.BigEndian.PutUint16(data[10:], uint16(baseArrayOff))

	copy(data[markCovOff:], markCoverage)
	copy(data[baseCovOff:], baseCoverage)
	copy(data[markArrayOff:], markArray)
	copy(data[baseArrayOff:], baseArray)

	return data
}

func TestMarkBasePosBasic(t *testing.T) {
	// Test: Position an accent mark (glyph 200) on a base letter (glyph 65='a')
	// Base anchor at (300, 500), mark anchor at (100, 0)
	// Expected offset: (300-100, 500-0) = (200, 500)

	markBasePos := buildMarkBasePos(
		[]GlyphID{200}, // mark glyphs
		[]GlyphID{65},  // base glyphs
		1,              // 1 class
		[]struct {
			class uint16
			x, y  int16
		}{{0, 100, 0}}, // mark: class 0, anchor at (100, 0)
		[][]*struct{ x, y int16 }{{{300, 500}}}, // base: anchor at (300, 500) for class 0
	)

	subtable, err := parseMarkBasePos(markBasePos, 0)
	if err != nil {
		t.Fatalf("parseMarkBasePos failed: %v", err)
	}

	ctx := &GPOSContext{
		Glyphs:    []GlyphID{65, 200}, // base 'a', then mark
		Positions: make([]GlyphPosition, 2),
		Index:     1, // Start at the mark
	}

	if !subtable.Apply(ctx) {
		t.Fatal("Apply returned false, expected true")
	}

	// Check positioning
	if ctx.Positions[1].XOffset != 200 {
		t.Errorf("XOffset = %d, want 200", ctx.Positions[1].XOffset)
	}
	if ctx.Positions[1].YOffset != 500 {
		t.Errorf("YOffset = %d, want 500", ctx.Positions[1].YOffset)
	}
	if ctx.Positions[1].AttachType != AttachTypeMark {
		t.Errorf("AttachType = %d, want %d", ctx.Positions[1].AttachType, AttachTypeMark)
	}
	if ctx.Positions[1].AttachChain != -1 {
		t.Errorf("AttachChain = %d, want -1", ctx.Positions[1].AttachChain)
	}
}

func TestMarkBasePosMultipleClasses(t *testing.T) {
	// Test with multiple mark classes
	// Class 0: above marks (accent), Class 1: below marks (cedilla)
	// Base 'a' (65) has above anchor at (300, 600) and below anchor at (300, -100)
	// Mark 200 (acute) is class 0, anchor at (50, 0)
	// Mark 201 (cedilla) is class 1, anchor at (50, 50)

	markBasePos := buildMarkBasePos(
		[]GlyphID{200, 201}, // mark glyphs
		[]GlyphID{65},       // base glyphs
		2,                   // 2 classes
		[]struct {
			class uint16
			x, y  int16
		}{
			{0, 50, 0},  // mark 200: class 0, anchor at (50, 0)
			{1, 50, 50}, // mark 201: class 1, anchor at (50, 50)
		},
		[][]*struct{ x, y int16 }{
			{{300, 600}, {300, -100}}, // base: class 0 at (300,600), class 1 at (300,-100)
		},
	)

	subtable, err := parseMarkBasePos(markBasePos, 0)
	if err != nil {
		t.Fatalf("parseMarkBasePos failed: %v", err)
	}

	// Test above mark (class 0)
	ctx := &GPOSContext{
		Glyphs:    []GlyphID{65, 200},
		Positions: make([]GlyphPosition, 2),
		Index:     1,
	}

	if !subtable.Apply(ctx) {
		t.Fatal("Apply returned false for above mark")
	}

	// Expected: (300-50, 600-0) = (250, 600)
	if ctx.Positions[1].XOffset != 250 {
		t.Errorf("Above mark XOffset = %d, want 250", ctx.Positions[1].XOffset)
	}
	if ctx.Positions[1].YOffset != 600 {
		t.Errorf("Above mark YOffset = %d, want 600", ctx.Positions[1].YOffset)
	}

	// Test below mark (class 1)
	ctx = &GPOSContext{
		Glyphs:    []GlyphID{65, 201},
		Positions: make([]GlyphPosition, 2),
		Index:     1,
	}

	if !subtable.Apply(ctx) {
		t.Fatal("Apply returned false for below mark")
	}

	// Expected: (300-50, -100-50) = (250, -150)
	if ctx.Positions[1].XOffset != 250 {
		t.Errorf("Below mark XOffset = %d, want 250", ctx.Positions[1].XOffset)
	}
	if ctx.Positions[1].YOffset != -150 {
		t.Errorf("Below mark YOffset = %d, want -150", ctx.Positions[1].YOffset)
	}
}

func TestMarkBasePosNoMatch(t *testing.T) {
	markBasePos := buildMarkBasePos(
		[]GlyphID{200},
		[]GlyphID{65},
		1,
		[]struct {
			class uint16
			x, y  int16
		}{{0, 100, 0}},
		[][]*struct{ x, y int16 }{{{300, 500}}},
	)

	subtable, err := parseMarkBasePos(markBasePos, 0)
	if err != nil {
		t.Fatalf("parseMarkBasePos failed: %v", err)
	}

	// Test: mark without preceding base
	ctx := &GPOSContext{
		Glyphs:    []GlyphID{200},
		Positions: make([]GlyphPosition, 1),
		Index:     0,
	}

	if subtable.Apply(ctx) {
		t.Error("Apply returned true for mark without base, expected false")
	}

	// Test: non-mark glyph
	ctx = &GPOSContext{
		Glyphs:    []GlyphID{65, 66},
		Positions: make([]GlyphPosition, 2),
		Index:     1,
	}

	if subtable.Apply(ctx) {
		t.Error("Apply returned true for non-mark glyph, expected false")
	}
}

// --- MarkLigPos Tests ---

// Helper to build a LigatureAttach table
func buildLigatureAttach(componentCount int, classCount int, anchors [][]*struct{ x, y int16 }) []byte {
	totalAnchors := componentCount * classCount
	headerSize := 2 + totalAnchors*2

	// Count non-nil anchors for size calculation
	anchorSize := 0
	for _, comp := range anchors {
		for _, a := range comp {
			if a != nil {
				anchorSize += 6
			}
		}
	}

	data := make([]byte, headerSize+anchorSize)
	binary.BigEndian.PutUint16(data[0:], uint16(componentCount))

	anchorOff := headerSize
	for comp := 0; comp < componentCount; comp++ {
		for class := 0; class < classCount; class++ {
			idx := comp*classCount + class
			offPos := 2 + idx*2

			if comp < len(anchors) && class < len(anchors[comp]) && anchors[comp][class] != nil {
				binary.BigEndian.PutUint16(data[offPos:], uint16(anchorOff))
				binary.BigEndian.PutUint16(data[anchorOff:], 1) // format
				binary.BigEndian.PutUint16(data[anchorOff+2:], uint16(anchors[comp][class].x))
				binary.BigEndian.PutUint16(data[anchorOff+4:], uint16(anchors[comp][class].y))
				anchorOff += 6
			} else {
				binary.BigEndian.PutUint16(data[offPos:], 0) // NULL
			}
		}
	}

	return data
}

// Helper to build a LigatureArray
func buildLigatureArray(classCount int, ligAttachments [][][]*struct{ x, y int16 }) []byte {
	// Build all ligature attach tables first
	attachTables := make([][]byte, len(ligAttachments))
	for i, anchors := range ligAttachments {
		componentCount := len(anchors)
		attachTables[i] = buildLigatureAttach(componentCount, classCount, anchors)
	}

	// LigatureArray: count(2) + offsets(2 each) + attach tables
	headerSize := 2 + len(ligAttachments)*2
	totalSize := headerSize
	for _, t := range attachTables {
		totalSize += len(t)
	}

	data := make([]byte, totalSize)
	binary.BigEndian.PutUint16(data[0:], uint16(len(ligAttachments)))

	attachOff := headerSize
	for i, t := range attachTables {
		binary.BigEndian.PutUint16(data[2+i*2:], uint16(attachOff))
		copy(data[attachOff:], t)
		attachOff += len(t)
	}

	return data
}

// Helper to build a MarkLigPos subtable
func buildMarkLigPos(
	markGlyphs []GlyphID,
	ligGlyphs []GlyphID,
	classCount int,
	markRecords []struct {
		class uint16
		x, y  int16
	},
	ligAttachments [][][]*struct{ x, y int16 }, // [ligature][component][class]
) []byte {
	markCoverage := buildCoverageFormat1(markGlyphs)
	ligCoverage := buildCoverageFormat1(ligGlyphs)

	// Build mark array
	markRecs := make([]struct {
		class  uint16
		anchor []byte
	}, len(markRecords))
	for i, r := range markRecords {
		markRecs[i] = struct {
			class  uint16
			anchor []byte
		}{
			class:  r.class,
			anchor: buildAnchor(r.x, r.y),
		}
	}
	markArray := buildMarkArray(markRecs)

	// Build ligature array
	ligArray := buildLigatureArray(classCount, ligAttachments)

	// Calculate offsets
	headerSize := 12
	markCovOff := headerSize
	ligCovOff := markCovOff + len(markCoverage)
	markArrayOff := ligCovOff + len(ligCoverage)
	ligArrayOff := markArrayOff + len(markArray)

	totalSize := ligArrayOff + len(ligArray)
	data := make([]byte, totalSize)

	binary.BigEndian.PutUint16(data[0:], 1) // format
	binary.BigEndian.PutUint16(data[2:], uint16(markCovOff))
	binary.BigEndian.PutUint16(data[4:], uint16(ligCovOff))
	binary.BigEndian.PutUint16(data[6:], uint16(classCount))
	binary.BigEndian.PutUint16(data[8:], uint16(markArrayOff))
	binary.BigEndian.PutUint16(data[10:], uint16(ligArrayOff))

	copy(data[markCovOff:], markCoverage)
	copy(data[ligCovOff:], ligCoverage)
	copy(data[markArrayOff:], markArray)
	copy(data[ligArrayOff:], ligArray)

	return data
}

func TestMarkLigPosBasic(t *testing.T) {
	// Test: Position an accent mark (200) on a ligature "fi" (glyph 500)
	// The ligature has 2 components: 'f' and 'i'
	// Component 0 (f): anchor at (100, 600)
	// Component 1 (i): anchor at (300, 600)
	// Mark anchor at (50, 0)
	// By default, attaches to last component (1)
	// Expected offset: (300-50, 600-0) = (250, 600)

	markLigPos := buildMarkLigPos(
		[]GlyphID{200}, // mark glyphs
		[]GlyphID{500}, // ligature glyphs
		1,              // 1 class
		[]struct {
			class uint16
			x, y  int16
		}{{0, 50, 0}}, // mark: class 0, anchor at (50, 0)
		[][][]*struct{ x, y int16 }{
			// Ligature 500 with 2 components
			{
				{{100, 600}}, // component 0: class 0 anchor at (100, 600)
				{{300, 600}}, // component 1: class 0 anchor at (300, 600)
			},
		},
	)

	subtable, err := parseMarkLigPos(markLigPos, 0)
	if err != nil {
		t.Fatalf("parseMarkLigPos failed: %v", err)
	}

	ctx := &GPOSContext{
		Glyphs:    []GlyphID{500, 200}, // ligature "fi", then mark
		Positions: make([]GlyphPosition, 2),
		Index:     1, // Start at the mark
	}

	if !subtable.Apply(ctx) {
		t.Fatal("Apply returned false, expected true")
	}

	// Default attaches to last component (component 1)
	// Expected: (300-50, 600-0) = (250, 600)
	if ctx.Positions[1].XOffset != 250 {
		t.Errorf("XOffset = %d, want 250", ctx.Positions[1].XOffset)
	}
	if ctx.Positions[1].YOffset != 600 {
		t.Errorf("YOffset = %d, want 600", ctx.Positions[1].YOffset)
	}
}

func TestMarkLigPosWithComponent(t *testing.T) {
	// Test: Attach to specific component
	markLigPos := buildMarkLigPos(
		[]GlyphID{200},
		[]GlyphID{500},
		1,
		[]struct {
			class uint16
			x, y  int16
		}{{0, 50, 0}},
		[][][]*struct{ x, y int16 }{
			{
				{{100, 600}}, // component 0
				{{300, 600}}, // component 1
			},
		},
	)

	subtable, err := parseMarkLigPos(markLigPos, 0)
	if err != nil {
		t.Fatalf("parseMarkLigPos failed: %v", err)
	}

	// Test attaching to component 0
	ctx := &GPOSContext{
		Glyphs:    []GlyphID{500, 200},
		Positions: make([]GlyphPosition, 2),
		Index:     1,
	}

	if !subtable.ApplyWithComponent(ctx, 0) {
		t.Fatal("ApplyWithComponent returned false")
	}

	// Expected: (100-50, 600-0) = (50, 600)
	if ctx.Positions[1].XOffset != 50 {
		t.Errorf("Component 0 XOffset = %d, want 50", ctx.Positions[1].XOffset)
	}
	if ctx.Positions[1].YOffset != 600 {
		t.Errorf("Component 0 YOffset = %d, want 600", ctx.Positions[1].YOffset)
	}
}

func TestMarkLigPosMultipleClasses(t *testing.T) {
	// Test with multiple mark classes on a ligature
	// Class 0: above marks, Class 1: below marks
	// Ligature "fi" component 1 (i):
	//   - Class 0 (above): anchor at (300, 700)
	//   - Class 1 (below): anchor at (300, -50)

	markLigPos := buildMarkLigPos(
		[]GlyphID{200, 201}, // marks: 200 (above), 201 (below)
		[]GlyphID{500},      // ligature
		2,                   // 2 classes
		[]struct {
			class uint16
			x, y  int16
		}{
			{0, 50, 0},  // mark 200: class 0, anchor at (50, 0)
			{1, 50, 30}, // mark 201: class 1, anchor at (50, 30)
		},
		[][][]*struct{ x, y int16 }{
			{
				// Component 0: only class 0
				{{200, 600}, nil},
				// Component 1: both classes
				{{300, 700}, {300, -50}},
			},
		},
	)

	subtable, err := parseMarkLigPos(markLigPos, 0)
	if err != nil {
		t.Fatalf("parseMarkLigPos failed: %v", err)
	}

	// Test above mark (class 0) on component 1
	ctx := &GPOSContext{
		Glyphs:    []GlyphID{500, 200},
		Positions: make([]GlyphPosition, 2),
		Index:     1,
	}

	if !subtable.Apply(ctx) {
		t.Fatal("Apply returned false for above mark")
	}

	// Expected: (300-50, 700-0) = (250, 700)
	if ctx.Positions[1].XOffset != 250 {
		t.Errorf("Above mark XOffset = %d, want 250", ctx.Positions[1].XOffset)
	}
	if ctx.Positions[1].YOffset != 700 {
		t.Errorf("Above mark YOffset = %d, want 700", ctx.Positions[1].YOffset)
	}

	// Test below mark (class 1)
	ctx = &GPOSContext{
		Glyphs:    []GlyphID{500, 201},
		Positions: make([]GlyphPosition, 2),
		Index:     1,
	}

	if !subtable.Apply(ctx) {
		t.Fatal("Apply returned false for below mark")
	}

	// Expected: (300-50, -50-30) = (250, -80)
	if ctx.Positions[1].XOffset != 250 {
		t.Errorf("Below mark XOffset = %d, want 250", ctx.Positions[1].XOffset)
	}
	if ctx.Positions[1].YOffset != -80 {
		t.Errorf("Below mark YOffset = %d, want -80", ctx.Positions[1].YOffset)
	}
}

func TestMarkLigPosNoMatch(t *testing.T) {
	markLigPos := buildMarkLigPos(
		[]GlyphID{200},
		[]GlyphID{500},
		1,
		[]struct {
			class uint16
			x, y  int16
		}{{0, 50, 0}},
		[][][]*struct{ x, y int16 }{
			{{{100, 600}}},
		},
	)

	subtable, err := parseMarkLigPos(markLigPos, 0)
	if err != nil {
		t.Fatalf("parseMarkLigPos failed: %v", err)
	}

	// Test: mark without preceding ligature
	ctx := &GPOSContext{
		Glyphs:    []GlyphID{200},
		Positions: make([]GlyphPosition, 1),
		Index:     0,
	}

	if subtable.Apply(ctx) {
		t.Error("Apply returned true for mark without ligature, expected false")
	}

	// Test: non-mark glyph
	ctx = &GPOSContext{
		Glyphs:    []GlyphID{500, 66},
		Positions: make([]GlyphPosition, 2),
		Index:     1,
	}

	if subtable.Apply(ctx) {
		t.Error("Apply returned true for non-mark glyph, expected false")
	}
}

// --- MarkMarkPos Tests ---

// Helper to build a MarkMarkPos subtable (reuses buildMarkBasePos helper structure)
func buildMarkMarkPos(
	mark1Glyphs []GlyphID,
	mark2Glyphs []GlyphID,
	classCount int,
	mark1Records []struct {
		class uint16
		x, y  int16
	},
	mark2Anchors [][]*struct{ x, y int16 },
) []byte {
	mark1Coverage := buildCoverageFormat1(mark1Glyphs)
	mark2Coverage := buildCoverageFormat1(mark2Glyphs)

	// Build mark1 array
	mark1Recs := make([]struct {
		class  uint16
		anchor []byte
	}, len(mark1Records))
	for i, r := range mark1Records {
		mark1Recs[i] = struct {
			class  uint16
			anchor []byte
		}{
			class:  r.class,
			anchor: buildAnchor(r.x, r.y),
		}
	}
	mark1Array := buildMarkArray(mark1Recs)

	// Build mark2 array (same structure as base array)
	mark2Array := buildBaseArray(len(mark2Glyphs), classCount, mark2Anchors)

	// Calculate offsets
	// Header: format(2) + mark1CoverageOff(2) + mark2CoverageOff(2) + classCount(2) + mark1ArrayOff(2) + mark2ArrayOff(2) = 12
	headerSize := 12
	mark1CovOff := headerSize
	mark2CovOff := mark1CovOff + len(mark1Coverage)
	mark1ArrayOff := mark2CovOff + len(mark2Coverage)
	mark2ArrayOff := mark1ArrayOff + len(mark1Array)

	totalSize := mark2ArrayOff + len(mark2Array)
	data := make([]byte, totalSize)

	binary.BigEndian.PutUint16(data[0:], 1) // format
	binary.BigEndian.PutUint16(data[2:], uint16(mark1CovOff))
	binary.BigEndian.PutUint16(data[4:], uint16(mark2CovOff))
	binary.BigEndian.PutUint16(data[6:], uint16(classCount))
	binary.BigEndian.PutUint16(data[8:], uint16(mark1ArrayOff))
	binary.BigEndian.PutUint16(data[10:], uint16(mark2ArrayOff))

	copy(data[mark1CovOff:], mark1Coverage)
	copy(data[mark2CovOff:], mark2Coverage)
	copy(data[mark1ArrayOff:], mark1Array)
	copy(data[mark2ArrayOff:], mark2Array)

	return data
}

func TestMarkMarkPosBasic(t *testing.T) {
	// Test: Position a second accent (mark1, glyph 201) on top of a first accent (mark2, glyph 200)
	// Mark2 (first accent) has anchor at (50, 100) for class 0
	// Mark1 (second accent) has anchor at (50, 0), class 0
	// Expected offset: (50-50, 100-0) = (0, 100)

	markMarkPos := buildMarkMarkPos(
		[]GlyphID{201}, // mark1 glyphs (the attaching mark)
		[]GlyphID{200}, // mark2 glyphs (the base mark)
		1,              // 1 class
		[]struct {
			class uint16
			x, y  int16
		}{{0, 50, 0}}, // mark1: class 0, anchor at (50, 0)
		[][]*struct{ x, y int16 }{{{50, 100}}}, // mark2: anchor at (50, 100) for class 0
	)

	subtable, err := parseMarkMarkPos(markMarkPos, 0)
	if err != nil {
		t.Fatalf("parseMarkMarkPos failed: %v", err)
	}

	ctx := &GPOSContext{
		Glyphs:    []GlyphID{65, 200, 201}, // base 'a', first accent, second accent
		Positions: make([]GlyphPosition, 3),
		Index:     2, // Start at mark1 (second accent)
	}

	if !subtable.Apply(ctx) {
		t.Fatal("Apply returned false, expected true")
	}

	// Check positioning
	if ctx.Positions[2].XOffset != 0 {
		t.Errorf("XOffset = %d, want 0", ctx.Positions[2].XOffset)
	}
	if ctx.Positions[2].YOffset != 100 {
		t.Errorf("YOffset = %d, want 100", ctx.Positions[2].YOffset)
	}
	if ctx.Positions[2].AttachType != AttachTypeMark {
		t.Errorf("AttachType = %d, want %d", ctx.Positions[2].AttachType, AttachTypeMark)
	}
	if ctx.Positions[2].AttachChain != -1 {
		t.Errorf("AttachChain = %d, want -1", ctx.Positions[2].AttachChain)
	}
}

func TestMarkMarkPosStackedDiacritics(t *testing.T) {
	// Test stacking multiple diacritics
	// Mark2 (first accent, glyph 200) can have marks attached above
	// Mark1 (second accent, glyph 201) attaches on top
	// Mark2 anchor for class 0: (100, 200) - where to place mark1
	// Mark1 anchor: (100, 0) - the attachment point of mark1

	markMarkPos := buildMarkMarkPos(
		[]GlyphID{201},
		[]GlyphID{200},
		1,
		[]struct {
			class uint16
			x, y  int16
		}{{0, 100, 0}},
		[][]*struct{ x, y int16 }{{{100, 200}}},
	)

	subtable, err := parseMarkMarkPos(markMarkPos, 0)
	if err != nil {
		t.Fatalf("parseMarkMarkPos failed: %v", err)
	}

	ctx := &GPOSContext{
		Glyphs:    []GlyphID{200, 201}, // first accent, second accent (no base needed for test)
		Positions: make([]GlyphPosition, 2),
		Index:     1,
	}

	if !subtable.Apply(ctx) {
		t.Fatal("Apply returned false, expected true")
	}

	// Expected: (100-100, 200-0) = (0, 200)
	if ctx.Positions[1].XOffset != 0 {
		t.Errorf("XOffset = %d, want 0", ctx.Positions[1].XOffset)
	}
	if ctx.Positions[1].YOffset != 200 {
		t.Errorf("YOffset = %d, want 200", ctx.Positions[1].YOffset)
	}
}

func TestMarkMarkPosNoMatch(t *testing.T) {
	markMarkPos := buildMarkMarkPos(
		[]GlyphID{201},
		[]GlyphID{200},
		1,
		[]struct {
			class uint16
			x, y  int16
		}{{0, 50, 0}},
		[][]*struct{ x, y int16 }{{{50, 100}}},
	)

	subtable, err := parseMarkMarkPos(markMarkPos, 0)
	if err != nil {
		t.Fatalf("parseMarkMarkPos failed: %v", err)
	}

	// Test: mark1 without preceding mark2
	ctx := &GPOSContext{
		Glyphs:    []GlyphID{65, 201}, // base, then mark1 (no mark2 in between)
		Positions: make([]GlyphPosition, 2),
		Index:     1,
	}

	if subtable.Apply(ctx) {
		t.Error("Apply returned true for mark1 without mark2, expected false")
	}

	// Test: non-mark1 glyph
	ctx = &GPOSContext{
		Glyphs:    []GlyphID{200, 202}, // mark2, then unknown glyph
		Positions: make([]GlyphPosition, 2),
		Index:     1,
	}

	if subtable.Apply(ctx) {
		t.Error("Apply returned true for non-mark1 glyph, expected false")
	}
}

func TestMarkMarkPosMultipleClasses(t *testing.T) {
	// Test with multiple mark classes
	// Class 0: top marks, Class 1: bottom marks
	// Mark2 (200) has top anchor at (50, 150) and bottom anchor at (50, -50)
	// Mark1 (201) is class 0 (top), anchor at (50, 0)
	// Mark1 (202) is class 1 (bottom), anchor at (50, 30)

	markMarkPos := buildMarkMarkPos(
		[]GlyphID{201, 202}, // mark1 glyphs
		[]GlyphID{200},      // mark2 glyphs
		2,                   // 2 classes
		[]struct {
			class uint16
			x, y  int16
		}{
			{0, 50, 0},  // mark1 201: class 0, anchor at (50, 0)
			{1, 50, 30}, // mark1 202: class 1, anchor at (50, 30)
		},
		[][]*struct{ x, y int16 }{
			{{50, 150}, {50, -50}}, // mark2: class 0 at (50,150), class 1 at (50,-50)
		},
	)

	subtable, err := parseMarkMarkPos(markMarkPos, 0)
	if err != nil {
		t.Fatalf("parseMarkMarkPos failed: %v", err)
	}

	// Test top mark (class 0)
	ctx := &GPOSContext{
		Glyphs:    []GlyphID{200, 201},
		Positions: make([]GlyphPosition, 2),
		Index:     1,
	}

	if !subtable.Apply(ctx) {
		t.Fatal("Apply returned false for top mark")
	}

	// Expected: (50-50, 150-0) = (0, 150)
	if ctx.Positions[1].XOffset != 0 {
		t.Errorf("Top mark XOffset = %d, want 0", ctx.Positions[1].XOffset)
	}
	if ctx.Positions[1].YOffset != 150 {
		t.Errorf("Top mark YOffset = %d, want 150", ctx.Positions[1].YOffset)
	}

	// Test bottom mark (class 1)
	ctx = &GPOSContext{
		Glyphs:    []GlyphID{200, 202},
		Positions: make([]GlyphPosition, 2),
		Index:     1,
	}

	if !subtable.Apply(ctx) {
		t.Fatal("Apply returned false for bottom mark")
	}

	// Expected: (50-50, -50-30) = (0, -80)
	if ctx.Positions[1].XOffset != 0 {
		t.Errorf("Bottom mark XOffset = %d, want 0", ctx.Positions[1].XOffset)
	}
	if ctx.Positions[1].YOffset != -80 {
		t.Errorf("Bottom mark YOffset = %d, want -80", ctx.Positions[1].YOffset)
	}
}

// --- CursivePos Tests ---

// Helper to build a CursivePos subtable
func buildCursivePos(
	coverageGlyphs []GlyphID,
	entryExits []struct {
		entryX, entryY *int16 // nil means no entry anchor
		exitX, exitY   *int16 // nil means no exit anchor
	},
) []byte {
	coverage := buildCoverageFormat1(coverageGlyphs)

	// Count anchors
	anchorCount := 0
	for _, ee := range entryExits {
		if ee.entryX != nil {
			anchorCount++
		}
		if ee.exitX != nil {
			anchorCount++
		}
	}

	// EntryExitRecord: entryAnchorOffset(2) + exitAnchorOffset(2) = 4 bytes each
	headerSize := 6 + len(entryExits)*4
	anchorSize := anchorCount * 6

	data := make([]byte, headerSize+anchorSize+len(coverage))
	binary.BigEndian.PutUint16(data[0:], 1)                             // format
	binary.BigEndian.PutUint16(data[2:], uint16(headerSize+anchorSize)) // coverage offset
	binary.BigEndian.PutUint16(data[4:], uint16(len(entryExits)))

	anchorOff := headerSize
	for i, ee := range entryExits {
		recOff := 6 + i*4

		// Entry anchor
		if ee.entryX != nil {
			binary.BigEndian.PutUint16(data[recOff:], uint16(anchorOff))
			binary.BigEndian.PutUint16(data[anchorOff:], 1) // format
			binary.BigEndian.PutUint16(data[anchorOff+2:], uint16(*ee.entryX))
			binary.BigEndian.PutUint16(data[anchorOff+4:], uint16(*ee.entryY))
			anchorOff += 6
		} else {
			binary.BigEndian.PutUint16(data[recOff:], 0) // NULL
		}

		// Exit anchor
		if ee.exitX != nil {
			binary.BigEndian.PutUint16(data[recOff+2:], uint16(anchorOff))
			binary.BigEndian.PutUint16(data[anchorOff:], 1) // format
			binary.BigEndian.PutUint16(data[anchorOff+2:], uint16(*ee.exitX))
			binary.BigEndian.PutUint16(data[anchorOff+4:], uint16(*ee.exitY))
			anchorOff += 6
		} else {
			binary.BigEndian.PutUint16(data[recOff+2:], 0) // NULL
		}
	}

	copy(data[headerSize+anchorSize:], coverage)
	return data
}

// Helper to create int16 pointers
func int16Ptr(v int16) *int16 {
	return &v
}

func TestCursivePosBasicRTL(t *testing.T) {
	// Test cursive attachment for Arabic-style RTL text
	// Glyph 100 (beh): exit at (0, 500)
	// Glyph 101 (noon): entry at (600, 500), exit at (0, 450)
	// Glyph 102 (final form): entry at (550, 450)
	//
	// In RTL, we read right-to-left. The previous glyph's exit connects to current's entry.
	// With RightToLeft flag, the previous glyph (child) adjusts to the current glyph (parent).

	cursivePos := buildCursivePos(
		[]GlyphID{100, 101, 102},
		[]struct {
			entryX, entryY *int16
			exitX, exitY   *int16
		}{
			{nil, nil, int16Ptr(0), int16Ptr(500)},                     // glyph 100: exit only
			{int16Ptr(600), int16Ptr(500), int16Ptr(0), int16Ptr(450)}, // glyph 101: entry and exit
			{int16Ptr(550), int16Ptr(450), nil, nil},                   // glyph 102: entry only
		},
	)

	subtable, err := parseCursivePos(cursivePos, 0)
	if err != nil {
		t.Fatalf("parseCursivePos failed: %v", err)
	}

	// Test: Connect glyph 100 (exit) to glyph 101 (entry) in RTL
	glyphs := []GlyphID{100, 101}
	positions := make([]GlyphPosition, 2)

	ctx := &GPOSContext{
		Glyphs:     glyphs,
		Positions:  positions,
		Index:      1, // Processing glyph 101
		Direction:  DirectionRTL,
		LookupFlag: LookupFlagRightToLeft,
	}

	if !subtable.Apply(ctx) {
		t.Fatal("Apply returned false, expected true")
	}

	// In RTL with RightToLeft flag:
	// - child = i (previous, glyph 100)
	// - parent = j (current, glyph 101)
	// - yOffset = entryY - exitY = 500 - 500 = 0
	// The child's YOffset should be set (horizontal direction)
	if positions[0].AttachType != AttachTypeCursive {
		t.Errorf("positions[0].AttachType = %d, want %d", positions[0].AttachType, AttachTypeCursive)
	}
	if positions[0].AttachChain != 1 {
		t.Errorf("positions[0].AttachChain = %d, want 1", positions[0].AttachChain)
	}
	if positions[0].YOffset != 0 {
		t.Errorf("positions[0].YOffset = %d, want 0", positions[0].YOffset)
	}
}

func TestCursivePosBasicLTR(t *testing.T) {
	// Test cursive attachment for LTR text
	// In LTR, the previous glyph's exit connects to current's entry
	// Without RightToLeft flag, the current glyph (child) adjusts to previous (parent)

	cursivePos := buildCursivePos(
		[]GlyphID{100, 101},
		[]struct {
			entryX, entryY *int16
			exitX, exitY   *int16
		}{
			{nil, nil, int16Ptr(500), int16Ptr(200)}, // glyph 100: exit only
			{int16Ptr(50), int16Ptr(250), nil, nil},  // glyph 101: entry only
		},
	)

	subtable, err := parseCursivePos(cursivePos, 0)
	if err != nil {
		t.Fatalf("parseCursivePos failed: %v", err)
	}

	glyphs := []GlyphID{100, 101}
	positions := make([]GlyphPosition, 2)

	ctx := &GPOSContext{
		Glyphs:     glyphs,
		Positions:  positions,
		Index:      1,
		Direction:  DirectionLTR,
		LookupFlag: 0, // No RightToLeft flag
	}

	if !subtable.Apply(ctx) {
		t.Fatal("Apply returned false, expected true")
	}

	// In LTR without RightToLeft flag:
	// - child = j (current, glyph 101)
	// - parent = i (previous, glyph 100)
	// - yOffset = -(entryY - exitY) = -(250 - 200) = -50
	if positions[1].AttachType != AttachTypeCursive {
		t.Errorf("positions[1].AttachType = %d, want %d", positions[1].AttachType, AttachTypeCursive)
	}
	if positions[1].AttachChain != -1 {
		t.Errorf("positions[1].AttachChain = %d, want -1", positions[1].AttachChain)
	}
	if positions[1].YOffset != -50 {
		t.Errorf("positions[1].YOffset = %d, want -50", positions[1].YOffset)
	}

	// Check advance adjustments for LTR
	// Previous glyph's xAdvance should be set to exit X
	if positions[0].XAdvance != 500 {
		t.Errorf("positions[0].XAdvance = %d, want 500", positions[0].XAdvance)
	}
	// Current glyph's xAdvance and xOffset should be adjusted by entry X
	if positions[1].XAdvance != -50 {
		t.Errorf("positions[1].XAdvance = %d, want -50", positions[1].XAdvance)
	}
	if positions[1].XOffset != -50 {
		t.Errorf("positions[1].XOffset = %d, want -50", positions[1].XOffset)
	}
}

func TestCursivePosNoMatch(t *testing.T) {
	cursivePos := buildCursivePos(
		[]GlyphID{100, 101},
		[]struct {
			entryX, entryY *int16
			exitX, exitY   *int16
		}{
			{nil, nil, int16Ptr(500), int16Ptr(200)},
			{int16Ptr(50), int16Ptr(250), nil, nil},
		},
	)

	subtable, err := parseCursivePos(cursivePos, 0)
	if err != nil {
		t.Fatalf("parseCursivePos failed: %v", err)
	}

	// Test: Current glyph not in coverage
	ctx := &GPOSContext{
		Glyphs:    []GlyphID{100, 200}, // 200 not in coverage
		Positions: make([]GlyphPosition, 2),
		Index:     1,
		Direction: DirectionLTR,
	}

	if subtable.Apply(ctx) {
		t.Error("Apply returned true for uncovered glyph, expected false")
	}

	// Test: Current glyph has no entry anchor
	ctx = &GPOSContext{
		Glyphs:    []GlyphID{101, 100}, // glyph 100 has no entry anchor
		Positions: make([]GlyphPosition, 2),
		Index:     1,
		Direction: DirectionLTR,
	}

	if subtable.Apply(ctx) {
		t.Error("Apply returned true for glyph without entry anchor, expected false")
	}

	// Test: Previous glyph has no exit anchor
	ctx = &GPOSContext{
		Glyphs:    []GlyphID{101, 101}, // first 101 has entry but no exit at first position (actually it does have exit)
		Positions: make([]GlyphPosition, 2),
		Index:     1,
		Direction: DirectionLTR,
	}
	// Actually 101 has both entry and exit, so this will match. Let me fix the test.

	// Proper test: neither prev nor current can match
	ctx = &GPOSContext{
		Glyphs:    []GlyphID{200, 101}, // 200 not in coverage
		Positions: make([]GlyphPosition, 2),
		Index:     1,
		Direction: DirectionLTR,
	}

	if subtable.Apply(ctx) {
		t.Error("Apply returned true when previous glyph not in coverage, expected false")
	}
}

func TestCursivePosChain(t *testing.T) {
	// Test multiple glyphs connected in a cursive chain
	cursivePos := buildCursivePos(
		[]GlyphID{100, 101, 102},
		[]struct {
			entryX, entryY *int16
			exitX, exitY   *int16
		}{
			{nil, nil, int16Ptr(500), int16Ptr(100)},                    // 100: exit only (first letter)
			{int16Ptr(50), int16Ptr(100), int16Ptr(450), int16Ptr(150)}, // 101: entry and exit (middle)
			{int16Ptr(50), int16Ptr(150), nil, nil},                     // 102: entry only (final)
		},
	)

	subtable, err := parseCursivePos(cursivePos, 0)
	if err != nil {
		t.Fatalf("parseCursivePos failed: %v", err)
	}

	glyphs := []GlyphID{100, 101, 102}
	positions := make([]GlyphPosition, 3)

	// Process 100 -> 101 connection
	ctx := &GPOSContext{
		Glyphs:     glyphs,
		Positions:  positions,
		Index:      1,
		Direction:  DirectionLTR,
		LookupFlag: 0,
	}

	if !subtable.Apply(ctx) {
		t.Fatal("Apply returned false for first connection")
	}

	// Reset and process 101 -> 102 connection
	ctx.Index = 2
	if !subtable.Apply(ctx) {
		t.Fatal("Apply returned false for second connection")
	}

	// Verify chain structure
	// 101 should be attached to 100
	if positions[1].AttachChain != -1 {
		t.Errorf("positions[1].AttachChain = %d, want -1", positions[1].AttachChain)
	}
	// 102 should be attached to 101
	if positions[2].AttachChain != -1 {
		t.Errorf("positions[2].AttachChain = %d, want -1", positions[2].AttachChain)
	}
}

// --- ContextPos Tests ---

// contextPosRule represents a rule for building test context pos subtables
type contextPosRule struct {
	input   []GlyphID
	lookups []GPOSLookupRecord
}

// Helper to build a ContextPosFormat1 subtable
func buildContextPosFormat1(
	coverageGlyphs []GlyphID,
	rules [][]contextPosRule, // [ruleSetIndex][ruleIndex] -> rule data
) []byte {
	coverage := buildCoverageFormat1(coverageGlyphs)

	// Build rule sets
	ruleSets := make([][]byte, len(rules))
	for i, ruleSet := range rules {
		if len(ruleSet) == 0 {
			ruleSets[i] = nil
			continue
		}

		// Build rules for this set
		ruleBytes := make([][]byte, len(ruleSet))
		for j, rule := range ruleSet {
			// Rule: glyphCount(2) + lookupCount(2) + input glyphs + lookup records
			glyphCount := len(rule.input) + 1
			lookupCount := len(rule.lookups)
			ruleSize := 4 + len(rule.input)*2 + lookupCount*4

			ruleData := make([]byte, ruleSize)
			binary.BigEndian.PutUint16(ruleData[0:], uint16(glyphCount))
			binary.BigEndian.PutUint16(ruleData[2:], uint16(lookupCount))

			for k, g := range rule.input {
				binary.BigEndian.PutUint16(ruleData[4+k*2:], uint16(g))
			}

			lookupOff := 4 + len(rule.input)*2
			for k, lr := range rule.lookups {
				binary.BigEndian.PutUint16(ruleData[lookupOff+k*4:], lr.SequenceIndex)
				binary.BigEndian.PutUint16(ruleData[lookupOff+k*4+2:], lr.LookupIndex)
			}

			ruleBytes[j] = ruleData
		}

		// RuleSet: ruleCount(2) + ruleOffsets + rules
		ruleSetHeaderSize := 2 + len(ruleBytes)*2
		totalRuleSize := 0
		for _, rb := range ruleBytes {
			totalRuleSize += len(rb)
		}

		ruleSetData := make([]byte, ruleSetHeaderSize+totalRuleSize)
		binary.BigEndian.PutUint16(ruleSetData[0:], uint16(len(ruleBytes)))

		ruleOff := ruleSetHeaderSize
		for j, rb := range ruleBytes {
			binary.BigEndian.PutUint16(ruleSetData[2+j*2:], uint16(ruleOff))
			copy(ruleSetData[ruleOff:], rb)
			ruleOff += len(rb)
		}

		ruleSets[i] = ruleSetData
	}

	// Calculate total size
	// Header: format(2) + coverageOff(2) + ruleSetCount(2) + ruleSetOffsets
	headerSize := 6 + len(ruleSets)*2
	totalRuleSetSize := 0
	for _, rs := range ruleSets {
		totalRuleSetSize += len(rs)
	}

	totalSize := headerSize + totalRuleSetSize + len(coverage)
	data := make([]byte, totalSize)

	binary.BigEndian.PutUint16(data[0:], 1)                                   // format
	binary.BigEndian.PutUint16(data[2:], uint16(headerSize+totalRuleSetSize)) // coverage offset
	binary.BigEndian.PutUint16(data[4:], uint16(len(ruleSets)))

	ruleSetOff := headerSize
	for i, rs := range ruleSets {
		if rs == nil {
			binary.BigEndian.PutUint16(data[6+i*2:], 0) // NULL
		} else {
			binary.BigEndian.PutUint16(data[6+i*2:], uint16(ruleSetOff))
			copy(data[ruleSetOff:], rs)
			ruleSetOff += len(rs)
		}
	}

	copy(data[headerSize+totalRuleSetSize:], coverage)
	return data
}

// Helper to build a ContextPosFormat3 subtable
func buildContextPosFormat3(
	inputGlyphs [][]GlyphID, // each element is coverage glyphs for that position
	lookups []GPOSLookupRecord,
) []byte {
	// Build coverages
	coverages := make([][]byte, len(inputGlyphs))
	for i, glyphs := range inputGlyphs {
		coverages[i] = buildCoverageFormat1(glyphs)
	}

	// Header: format(2) + glyphCount(2) + lookupCount(2) + coverageOffsets + lookupRecords + coverages
	headerSize := 6 + len(coverages)*2 + len(lookups)*4
	totalCovSize := 0
	for _, c := range coverages {
		totalCovSize += len(c)
	}

	totalSize := headerSize + totalCovSize
	data := make([]byte, totalSize)

	binary.BigEndian.PutUint16(data[0:], 3) // format
	binary.BigEndian.PutUint16(data[2:], uint16(len(coverages)))
	binary.BigEndian.PutUint16(data[4:], uint16(len(lookups)))

	covOff := headerSize
	for i, c := range coverages {
		binary.BigEndian.PutUint16(data[6+i*2:], uint16(covOff))
		copy(data[covOff:], c)
		covOff += len(c)
	}

	lookupOff := 6 + len(coverages)*2
	for i, lr := range lookups {
		binary.BigEndian.PutUint16(data[lookupOff+i*4:], lr.SequenceIndex)
		binary.BigEndian.PutUint16(data[lookupOff+i*4+2:], lr.LookupIndex)
	}

	return data
}

func TestContextPosFormat1Basic(t *testing.T) {
	// Test Format 1: Simple glyph context
	// When glyph 65 is followed by glyph 66, apply lookup 0 to position 0
	contextPos := buildContextPosFormat1(
		[]GlyphID{65}, // Coverage: glyph 65
		[][]contextPosRule{
			{ // RuleSet 0 (for coverage index 0 = glyph 65)
				{ // Rule 0
					input:   []GlyphID{66}, // followed by 66
					lookups: []GPOSLookupRecord{{SequenceIndex: 0, LookupIndex: 0}},
				},
			},
		},
	)

	subtable, err := parseContextPosFormat1(contextPos, 0, nil)
	if err != nil {
		t.Fatalf("parseContextPosFormat1 failed: %v", err)
	}

	// Test matching sequence
	ctx := &GPOSContext{
		Glyphs:    []GlyphID{65, 66, 67},
		Positions: make([]GlyphPosition, 3),
		Index:     0,
	}

	// Without GPOS reference, we can only test matching (not nested lookup application)
	result := subtable.Apply(ctx)
	if !result {
		t.Error("Apply returned false for matching sequence, expected true")
	}
	if ctx.Index != 2 { // Should advance by input length (2)
		t.Errorf("Index = %d, want 2", ctx.Index)
	}
}

func TestContextPosFormat1NoMatch(t *testing.T) {
	contextPos := buildContextPosFormat1(
		[]GlyphID{65},
		[][]contextPosRule{
			{
				{
					input:   []GlyphID{66},
					lookups: []GPOSLookupRecord{{SequenceIndex: 0, LookupIndex: 0}},
				},
			},
		},
	)

	subtable, err := parseContextPosFormat1(contextPos, 0, nil)
	if err != nil {
		t.Fatalf("parseContextPosFormat1 failed: %v", err)
	}

	// Test non-matching sequence (65 followed by 67, not 66)
	ctx := &GPOSContext{
		Glyphs:    []GlyphID{65, 67, 68},
		Positions: make([]GlyphPosition, 3),
		Index:     0,
	}

	if subtable.Apply(ctx) {
		t.Error("Apply returned true for non-matching sequence, expected false")
	}

	// Test glyph not in coverage
	ctx = &GPOSContext{
		Glyphs:    []GlyphID{64, 66, 67},
		Positions: make([]GlyphPosition, 3),
		Index:     0,
	}

	if subtable.Apply(ctx) {
		t.Error("Apply returned true for glyph not in coverage, expected false")
	}
}

func TestContextPosFormat3Basic(t *testing.T) {
	// Test Format 3: Coverage-based context
	// Match: glyph in {65,66} followed by glyph in {67,68}
	contextPos := buildContextPosFormat3(
		[][]GlyphID{
			{65, 66}, // Position 0: match 65 or 66
			{67, 68}, // Position 1: match 67 or 68
		},
		[]GPOSLookupRecord{
			{SequenceIndex: 0, LookupIndex: 0},
			{SequenceIndex: 1, LookupIndex: 1},
		},
	)

	subtable, err := parseContextPosFormat3(contextPos, 0, nil)
	if err != nil {
		t.Fatalf("parseContextPosFormat3 failed: %v", err)
	}

	// Test matching: 65, 67
	ctx := &GPOSContext{
		Glyphs:    []GlyphID{65, 67, 100},
		Positions: make([]GlyphPosition, 3),
		Index:     0,
	}

	if !subtable.Apply(ctx) {
		t.Error("Apply returned false for matching sequence (65, 67)")
	}
	if ctx.Index != 2 {
		t.Errorf("Index = %d, want 2", ctx.Index)
	}

	// Test matching: 66, 68
	ctx = &GPOSContext{
		Glyphs:    []GlyphID{66, 68, 100},
		Positions: make([]GlyphPosition, 3),
		Index:     0,
	}

	if !subtable.Apply(ctx) {
		t.Error("Apply returned false for matching sequence (66, 68)")
	}
}

func TestContextPosFormat3NoMatch(t *testing.T) {
	contextPos := buildContextPosFormat3(
		[][]GlyphID{
			{65, 66},
			{67, 68},
		},
		[]GPOSLookupRecord{{SequenceIndex: 0, LookupIndex: 0}},
	)

	subtable, err := parseContextPosFormat3(contextPos, 0, nil)
	if err != nil {
		t.Fatalf("parseContextPosFormat3 failed: %v", err)
	}

	// Test non-matching: first glyph not in coverage
	ctx := &GPOSContext{
		Glyphs:    []GlyphID{64, 67},
		Positions: make([]GlyphPosition, 2),
		Index:     0,
	}

	if subtable.Apply(ctx) {
		t.Error("Apply returned true when first glyph not in coverage")
	}

	// Test non-matching: second glyph not in coverage
	ctx = &GPOSContext{
		Glyphs:    []GlyphID{65, 69},
		Positions: make([]GlyphPosition, 2),
		Index:     0,
	}

	if subtable.Apply(ctx) {
		t.Error("Apply returned true when second glyph not in coverage")
	}
}

func TestContextPosMultipleRules(t *testing.T) {
	// Test with multiple rules in a ruleset
	contextPos := buildContextPosFormat1(
		[]GlyphID{65},
		[][]contextPosRule{
			{ // RuleSet for glyph 65
				{ // Rule 0: 65 followed by 66, 67
					input:   []GlyphID{66, 67},
					lookups: []GPOSLookupRecord{{SequenceIndex: 0, LookupIndex: 0}},
				},
				{ // Rule 1: 65 followed by 68
					input:   []GlyphID{68},
					lookups: []GPOSLookupRecord{{SequenceIndex: 0, LookupIndex: 1}},
				},
			},
		},
	)

	subtable, err := parseContextPosFormat1(contextPos, 0, nil)
	if err != nil {
		t.Fatalf("parseContextPosFormat1 failed: %v", err)
	}

	// Test first rule matches
	ctx := &GPOSContext{
		Glyphs:    []GlyphID{65, 66, 67, 100},
		Positions: make([]GlyphPosition, 4),
		Index:     0,
	}

	if !subtable.Apply(ctx) {
		t.Error("Apply returned false for first rule match")
	}
	if ctx.Index != 3 { // 65, 66, 67 = 3 glyphs
		t.Errorf("Index = %d, want 3", ctx.Index)
	}

	// Test second rule matches (first rule doesn't match)
	ctx = &GPOSContext{
		Glyphs:    []GlyphID{65, 68, 100},
		Positions: make([]GlyphPosition, 3),
		Index:     0,
	}

	if !subtable.Apply(ctx) {
		t.Error("Apply returned false for second rule match")
	}
	if ctx.Index != 2 { // 65, 68 = 2 glyphs
		t.Errorf("Index = %d, want 2", ctx.Index)
	}
}

// --- ChainContextPos Tests ---

// chainContextPosRule represents a rule for building test chain context pos subtables
type chainContextPosRule struct {
	backtrack []GlyphID
	input     []GlyphID
	lookahead []GlyphID
	lookups   []GPOSLookupRecord
}

// Helper to build a ChainContextPosFormat1 subtable
func buildChainContextPosFormat1(
	coverageGlyphs []GlyphID,
	rules [][]chainContextPosRule, // [ruleSetIndex][ruleIndex] -> rule data
) []byte {
	coverage := buildCoverageFormat1(coverageGlyphs)

	// Build rule sets
	ruleSets := make([][]byte, len(rules))
	for i, ruleSet := range rules {
		if len(ruleSet) == 0 {
			ruleSets[i] = nil
			continue
		}

		// Build rules for this set
		ruleBytes := make([][]byte, len(ruleSet))
		for j, rule := range ruleSet {
			// ChainRule: backtrackCount(2) + backtrackGlyphs + inputCount(2) + inputGlyphs +
			//            lookaheadCount(2) + lookaheadGlyphs + lookupCount(2) + lookupRecords
			glyphCount := len(rule.input) + 1
			ruleSize := 2 + len(rule.backtrack)*2 + 2 + len(rule.input)*2 + 2 + len(rule.lookahead)*2 + 2 + len(rule.lookups)*4

			ruleData := make([]byte, ruleSize)
			off := 0

			// Backtrack
			binary.BigEndian.PutUint16(ruleData[off:], uint16(len(rule.backtrack)))
			off += 2
			for k, g := range rule.backtrack {
				binary.BigEndian.PutUint16(ruleData[off+k*2:], uint16(g))
			}
			off += len(rule.backtrack) * 2

			// Input
			binary.BigEndian.PutUint16(ruleData[off:], uint16(glyphCount))
			off += 2
			for k, g := range rule.input {
				binary.BigEndian.PutUint16(ruleData[off+k*2:], uint16(g))
			}
			off += len(rule.input) * 2

			// Lookahead
			binary.BigEndian.PutUint16(ruleData[off:], uint16(len(rule.lookahead)))
			off += 2
			for k, g := range rule.lookahead {
				binary.BigEndian.PutUint16(ruleData[off+k*2:], uint16(g))
			}
			off += len(rule.lookahead) * 2

			// Lookups
			binary.BigEndian.PutUint16(ruleData[off:], uint16(len(rule.lookups)))
			off += 2
			for k, lr := range rule.lookups {
				binary.BigEndian.PutUint16(ruleData[off+k*4:], lr.SequenceIndex)
				binary.BigEndian.PutUint16(ruleData[off+k*4+2:], lr.LookupIndex)
			}

			ruleBytes[j] = ruleData
		}

		// RuleSet: ruleCount(2) + ruleOffsets + rules
		ruleSetHeaderSize := 2 + len(ruleBytes)*2
		totalRuleSize := 0
		for _, rb := range ruleBytes {
			totalRuleSize += len(rb)
		}

		ruleSetData := make([]byte, ruleSetHeaderSize+totalRuleSize)
		binary.BigEndian.PutUint16(ruleSetData[0:], uint16(len(ruleBytes)))

		ruleOff := ruleSetHeaderSize
		for j, rb := range ruleBytes {
			binary.BigEndian.PutUint16(ruleSetData[2+j*2:], uint16(ruleOff))
			copy(ruleSetData[ruleOff:], rb)
			ruleOff += len(rb)
		}

		ruleSets[i] = ruleSetData
	}

	// Calculate total size
	// Header: format(2) + coverageOff(2) + ruleSetCount(2) + ruleSetOffsets
	headerSize := 6 + len(ruleSets)*2
	totalRuleSetSize := 0
	for _, rs := range ruleSets {
		totalRuleSetSize += len(rs)
	}

	totalSize := headerSize + totalRuleSetSize + len(coverage)
	data := make([]byte, totalSize)

	binary.BigEndian.PutUint16(data[0:], 1)                                   // format
	binary.BigEndian.PutUint16(data[2:], uint16(headerSize+totalRuleSetSize)) // coverage offset
	binary.BigEndian.PutUint16(data[4:], uint16(len(ruleSets)))

	ruleSetOff := headerSize
	for i, rs := range ruleSets {
		if rs == nil {
			binary.BigEndian.PutUint16(data[6+i*2:], 0) // NULL
		} else {
			binary.BigEndian.PutUint16(data[6+i*2:], uint16(ruleSetOff))
			copy(data[ruleSetOff:], rs)
			ruleSetOff += len(rs)
		}
	}

	copy(data[headerSize+totalRuleSetSize:], coverage)
	return data
}

// Helper to build a ChainContextPosFormat3 subtable
func buildChainContextPosFormat3(
	backtrackGlyphs [][]GlyphID, // each element is coverage glyphs for that backtrack position
	inputGlyphs [][]GlyphID, // each element is coverage glyphs for that input position
	lookaheadGlyphs [][]GlyphID, // each element is coverage glyphs for that lookahead position
	lookups []GPOSLookupRecord,
) []byte {
	// Build coverages
	backtrackCoverages := make([][]byte, len(backtrackGlyphs))
	for i, glyphs := range backtrackGlyphs {
		backtrackCoverages[i] = buildCoverageFormat1(glyphs)
	}

	inputCoverages := make([][]byte, len(inputGlyphs))
	for i, glyphs := range inputGlyphs {
		inputCoverages[i] = buildCoverageFormat1(glyphs)
	}

	lookaheadCoverages := make([][]byte, len(lookaheadGlyphs))
	for i, glyphs := range lookaheadGlyphs {
		lookaheadCoverages[i] = buildCoverageFormat1(glyphs)
	}

	// Calculate header size
	// format(2) + backtrackCount(2) + backtrackOffsets + inputCount(2) + inputOffsets +
	// lookaheadCount(2) + lookaheadOffsets + lookupCount(2) + lookupRecords
	headerSize := 2 + 2 + len(backtrackCoverages)*2 + 2 + len(inputCoverages)*2 + 2 + len(lookaheadCoverages)*2 + 2 + len(lookups)*4

	totalCovSize := 0
	for _, c := range backtrackCoverages {
		totalCovSize += len(c)
	}
	for _, c := range inputCoverages {
		totalCovSize += len(c)
	}
	for _, c := range lookaheadCoverages {
		totalCovSize += len(c)
	}

	totalSize := headerSize + totalCovSize
	data := make([]byte, totalSize)

	off := 0
	binary.BigEndian.PutUint16(data[off:], 3) // format
	off += 2

	// Backtrack coverages
	binary.BigEndian.PutUint16(data[off:], uint16(len(backtrackCoverages)))
	off += 2

	covOff := headerSize
	for i, c := range backtrackCoverages {
		binary.BigEndian.PutUint16(data[off+i*2:], uint16(covOff))
		copy(data[covOff:], c)
		covOff += len(c)
	}
	off += len(backtrackCoverages) * 2

	// Input coverages
	binary.BigEndian.PutUint16(data[off:], uint16(len(inputCoverages)))
	off += 2
	for i, c := range inputCoverages {
		binary.BigEndian.PutUint16(data[off+i*2:], uint16(covOff))
		copy(data[covOff:], c)
		covOff += len(c)
	}
	off += len(inputCoverages) * 2

	// Lookahead coverages
	binary.BigEndian.PutUint16(data[off:], uint16(len(lookaheadCoverages)))
	off += 2
	for i, c := range lookaheadCoverages {
		binary.BigEndian.PutUint16(data[off+i*2:], uint16(covOff))
		copy(data[covOff:], c)
		covOff += len(c)
	}
	off += len(lookaheadCoverages) * 2

	// Lookup records
	binary.BigEndian.PutUint16(data[off:], uint16(len(lookups)))
	off += 2
	for i, lr := range lookups {
		binary.BigEndian.PutUint16(data[off+i*4:], lr.SequenceIndex)
		binary.BigEndian.PutUint16(data[off+i*4+2:], lr.LookupIndex)
	}

	return data
}

func TestChainContextPosFormat1Basic(t *testing.T) {
	// Test Format 1: Simple glyph chain context
	// Backtrack: glyph 64
	// Input: glyph 65 followed by 66
	// Lookahead: glyph 67
	// When this pattern matches, apply lookup 0 to position 0
	chainContextPos := buildChainContextPosFormat1(
		[]GlyphID{65}, // Coverage: glyph 65
		[][]chainContextPosRule{
			{ // RuleSet 0 (for coverage index 0 = glyph 65)
				{ // Rule 0
					backtrack: []GlyphID{64},
					input:     []GlyphID{66}, // followed by 66
					lookahead: []GlyphID{67},
					lookups:   []GPOSLookupRecord{{SequenceIndex: 0, LookupIndex: 0}},
				},
			},
		},
	)

	subtable, err := parseChainContextPosFormat1(chainContextPos, 0, nil)
	if err != nil {
		t.Fatalf("parseChainContextPosFormat1 failed: %v", err)
	}

	// Test matching sequence: 64, 65, 66, 67
	ctx := &GPOSContext{
		Glyphs:    []GlyphID{64, 65, 66, 67},
		Positions: make([]GlyphPosition, 4),
		Index:     1, // Start at glyph 65
	}

	// Without GPOS reference, we can only test matching (not nested lookup application)
	result := subtable.Apply(ctx)
	if !result {
		t.Error("Apply returned false for matching sequence, expected true")
	}
	if ctx.Index != 3 { // Should advance by input length (2: glyph 65 and 66)
		t.Errorf("Index = %d, want 3", ctx.Index)
	}
}

func TestChainContextPosFormat1NoBacktrackMatch(t *testing.T) {
	chainContextPos := buildChainContextPosFormat1(
		[]GlyphID{65},
		[][]chainContextPosRule{
			{
				{
					backtrack: []GlyphID{64},
					input:     []GlyphID{66},
					lookahead: []GlyphID{67},
					lookups:   []GPOSLookupRecord{{SequenceIndex: 0, LookupIndex: 0}},
				},
			},
		},
	)

	subtable, err := parseChainContextPosFormat1(chainContextPos, 0, nil)
	if err != nil {
		t.Fatalf("parseChainContextPosFormat1 failed: %v", err)
	}

	// Test non-matching: wrong backtrack glyph (63 instead of 64)
	ctx := &GPOSContext{
		Glyphs:    []GlyphID{63, 65, 66, 67},
		Positions: make([]GlyphPosition, 4),
		Index:     1,
	}

	if subtable.Apply(ctx) {
		t.Error("Apply returned true when backtrack doesn't match, expected false")
	}
}

func TestChainContextPosFormat1NoLookaheadMatch(t *testing.T) {
	chainContextPos := buildChainContextPosFormat1(
		[]GlyphID{65},
		[][]chainContextPosRule{
			{
				{
					backtrack: []GlyphID{64},
					input:     []GlyphID{66},
					lookahead: []GlyphID{67},
					lookups:   []GPOSLookupRecord{{SequenceIndex: 0, LookupIndex: 0}},
				},
			},
		},
	)

	subtable, err := parseChainContextPosFormat1(chainContextPos, 0, nil)
	if err != nil {
		t.Fatalf("parseChainContextPosFormat1 failed: %v", err)
	}

	// Test non-matching: wrong lookahead glyph (68 instead of 67)
	ctx := &GPOSContext{
		Glyphs:    []GlyphID{64, 65, 66, 68},
		Positions: make([]GlyphPosition, 4),
		Index:     1,
	}

	if subtable.Apply(ctx) {
		t.Error("Apply returned true when lookahead doesn't match, expected false")
	}
}

func TestChainContextPosFormat1InsufficientBacktrack(t *testing.T) {
	chainContextPos := buildChainContextPosFormat1(
		[]GlyphID{65},
		[][]chainContextPosRule{
			{
				{
					backtrack: []GlyphID{64, 63}, // Requires 2 backtrack glyphs
					input:     []GlyphID{66},
					lookahead: []GlyphID{},
					lookups:   []GPOSLookupRecord{{SequenceIndex: 0, LookupIndex: 0}},
				},
			},
		},
	)

	subtable, err := parseChainContextPosFormat1(chainContextPos, 0, nil)
	if err != nil {
		t.Fatalf("parseChainContextPosFormat1 failed: %v", err)
	}

	// Test: not enough glyphs before current position for backtrack
	ctx := &GPOSContext{
		Glyphs:    []GlyphID{64, 65, 66}, // Only 1 glyph before 65, but need 2
		Positions: make([]GlyphPosition, 3),
		Index:     1,
	}

	if subtable.Apply(ctx) {
		t.Error("Apply returned true with insufficient backtrack, expected false")
	}
}

func TestChainContextPosFormat3Basic(t *testing.T) {
	// Test Format 3: Coverage-based chain context
	// Backtrack: glyph in {63, 64}
	// Input: glyph in {65, 66} followed by glyph in {67, 68}
	// Lookahead: glyph in {69, 70}
	chainContextPos := buildChainContextPosFormat3(
		[][]GlyphID{{63, 64}},           // Backtrack: one position, match 63 or 64
		[][]GlyphID{{65, 66}, {67, 68}}, // Input: two positions
		[][]GlyphID{{69, 70}},           // Lookahead: one position, match 69 or 70
		[]GPOSLookupRecord{
			{SequenceIndex: 0, LookupIndex: 0},
			{SequenceIndex: 1, LookupIndex: 1},
		},
	)

	subtable, err := parseChainContextPosFormat3(chainContextPos, 0, nil)
	if err != nil {
		t.Fatalf("parseChainContextPosFormat3 failed: %v", err)
	}

	// Test matching: 64, 65, 67, 69
	ctx := &GPOSContext{
		Glyphs:    []GlyphID{64, 65, 67, 69},
		Positions: make([]GlyphPosition, 4),
		Index:     1, // Start at glyph 65
	}

	if !subtable.Apply(ctx) {
		t.Error("Apply returned false for matching sequence (64, 65, 67, 69)")
	}
	if ctx.Index != 3 { // Should advance by input length (2)
		t.Errorf("Index = %d, want 3", ctx.Index)
	}

	// Test matching with different values: 63, 66, 68, 70
	ctx = &GPOSContext{
		Glyphs:    []GlyphID{63, 66, 68, 70},
		Positions: make([]GlyphPosition, 4),
		Index:     1,
	}

	if !subtable.Apply(ctx) {
		t.Error("Apply returned false for matching sequence (63, 66, 68, 70)")
	}
}

func TestChainContextPosFormat3NoMatch(t *testing.T) {
	chainContextPos := buildChainContextPosFormat3(
		[][]GlyphID{{63, 64}},
		[][]GlyphID{{65, 66}, {67, 68}},
		[][]GlyphID{{69, 70}},
		[]GPOSLookupRecord{{SequenceIndex: 0, LookupIndex: 0}},
	)

	subtable, err := parseChainContextPosFormat3(chainContextPos, 0, nil)
	if err != nil {
		t.Fatalf("parseChainContextPosFormat3 failed: %v", err)
	}

	// Test non-matching: backtrack not in coverage (62 instead of 63/64)
	ctx := &GPOSContext{
		Glyphs:    []GlyphID{62, 65, 67, 69},
		Positions: make([]GlyphPosition, 4),
		Index:     1,
	}

	if subtable.Apply(ctx) {
		t.Error("Apply returned true when backtrack not in coverage")
	}

	// Test non-matching: lookahead not in coverage (71 instead of 69/70)
	ctx = &GPOSContext{
		Glyphs:    []GlyphID{64, 65, 67, 71},
		Positions: make([]GlyphPosition, 4),
		Index:     1,
	}

	if subtable.Apply(ctx) {
		t.Error("Apply returned true when lookahead not in coverage")
	}

	// Test non-matching: input not in coverage
	ctx = &GPOSContext{
		Glyphs:    []GlyphID{64, 65, 69, 69}, // 69 at position 2 should be 67 or 68
		Positions: make([]GlyphPosition, 4),
		Index:     1,
	}

	if subtable.Apply(ctx) {
		t.Error("Apply returned true when input not in coverage")
	}
}

func TestChainContextPosFormat3NoBacktrack(t *testing.T) {
	// Test with empty backtrack (should match from the start)
	chainContextPos := buildChainContextPosFormat3(
		[][]GlyphID{}, // No backtrack
		[][]GlyphID{{65, 66}, {67, 68}},
		[][]GlyphID{{69, 70}},
		[]GPOSLookupRecord{{SequenceIndex: 0, LookupIndex: 0}},
	)

	subtable, err := parseChainContextPosFormat3(chainContextPos, 0, nil)
	if err != nil {
		t.Fatalf("parseChainContextPosFormat3 failed: %v", err)
	}

	// Test matching at start (no backtrack needed)
	ctx := &GPOSContext{
		Glyphs:    []GlyphID{65, 67, 69},
		Positions: make([]GlyphPosition, 3),
		Index:     0,
	}

	if !subtable.Apply(ctx) {
		t.Error("Apply returned false for matching sequence with no backtrack")
	}
	if ctx.Index != 2 {
		t.Errorf("Index = %d, want 2", ctx.Index)
	}
}

func TestChainContextPosFormat3NoLookahead(t *testing.T) {
	// Test with empty lookahead (should match at the end)
	chainContextPos := buildChainContextPosFormat3(
		[][]GlyphID{{63, 64}},
		[][]GlyphID{{65, 66}, {67, 68}},
		[][]GlyphID{}, // No lookahead
		[]GPOSLookupRecord{{SequenceIndex: 0, LookupIndex: 0}},
	)

	subtable, err := parseChainContextPosFormat3(chainContextPos, 0, nil)
	if err != nil {
		t.Fatalf("parseChainContextPosFormat3 failed: %v", err)
	}

	// Test matching at end (no lookahead needed)
	ctx := &GPOSContext{
		Glyphs:    []GlyphID{64, 65, 67},
		Positions: make([]GlyphPosition, 3),
		Index:     1,
	}

	if !subtable.Apply(ctx) {
		t.Error("Apply returned false for matching sequence with no lookahead")
	}
	if ctx.Index != 3 {
		t.Errorf("Index = %d, want 3", ctx.Index)
	}
}

func TestChainContextPosMultipleBacktrackAndLookahead(t *testing.T) {
	// Test with multiple backtrack and lookahead positions
	chainContextPos := buildChainContextPosFormat3(
		[][]GlyphID{{61}, {62}}, // Two backtrack positions (order: [0] = closest, [1] = further back)
		[][]GlyphID{{65}},       // One input position
		[][]GlyphID{{68}, {69}}, // Two lookahead positions
		[]GPOSLookupRecord{{SequenceIndex: 0, LookupIndex: 0}},
	)

	subtable, err := parseChainContextPosFormat3(chainContextPos, 0, nil)
	if err != nil {
		t.Fatalf("parseChainContextPosFormat3 failed: %v", err)
	}

	// Sequence: 62, 61, 65, 68, 69
	// Backtrack[0]=61 matches glyph at index 1 (right before 65)
	// Backtrack[1]=62 matches glyph at index 0 (further back)
	// Input: 65
	// Lookahead: 68, 69
	ctx := &GPOSContext{
		Glyphs:    []GlyphID{62, 61, 65, 68, 69},
		Positions: make([]GlyphPosition, 5),
		Index:     2, // Start at glyph 65
	}

	if !subtable.Apply(ctx) {
		t.Error("Apply returned false for sequence with multiple backtrack and lookahead")
	}
	if ctx.Index != 3 { // Should advance by input length (1)
		t.Errorf("Index = %d, want 3", ctx.Index)
	}

	// Test failure: wrong backtrack order (61, 62 instead of 62, 61)
	ctx = &GPOSContext{
		Glyphs:    []GlyphID{61, 62, 65, 68, 69},
		Positions: make([]GlyphPosition, 5),
		Index:     2,
	}

	if subtable.Apply(ctx) {
		t.Error("Apply returned true when backtrack order is wrong")
	}
}

// --- Extension Positioning Tests ---

// buildExtensionPosSubtable builds an Extension positioning subtable that wraps another subtable
// Extension format: format(2) + extensionLookupType(2) + extensionOffset(4) + actual subtable
func buildExtensionPosSubtable(extensionLookupType uint16, subtable []byte) []byte {
	// Extension header is 8 bytes, subtable follows immediately after
	data := make([]byte, 8+len(subtable))
	binary.BigEndian.PutUint16(data[0:], 1)                   // format = 1
	binary.BigEndian.PutUint16(data[2:], extensionLookupType) // actual lookup type
	binary.BigEndian.PutUint32(data[4:], 8)                   // offset to subtable (right after header)
	copy(data[8:], subtable)
	return data
}

func TestExtensionPosWithSinglePos(t *testing.T) {
	// Test Extension lookup wrapping a SinglePos
	// SinglePos: adjust xAdvance by -50 for glyphs 65, 66, 67
	singlePos := buildSinglePosFormat1([]GlyphID{65, 66, 67}, ValueFormatXAdvance, ValueRecord{XAdvance: -50})

	// Wrap in Extension
	extensionSubtable := buildExtensionPosSubtable(GPOSTypeSingle, singlePos)

	// Build lookup with Extension type
	lookup := buildGPOSLookup(GPOSTypeExtension, [][]byte{extensionSubtable})
	gposData := buildGPOSTable([][]byte{lookup})

	gpos, err := ParseGPOS(gposData)
	if err != nil {
		t.Fatalf("ParseGPOS failed: %v", err)
	}

	if gpos.NumLookups() != 1 {
		t.Fatalf("NumLookups = %d, want 1", gpos.NumLookups())
	}

	// Apply lookup
	glyphs := []GlyphID{65, 66, 67, 68}
	positions := make([]GlyphPosition, len(glyphs))
	gpos.ApplyLookup(0, glyphs, positions)

	// 65, 66, 67 should have xAdvance -50
	// 68 is not in coverage, should remain 0
	for i := 0; i < 3; i++ {
		if positions[i].XAdvance != -50 {
			t.Errorf("positions[%d].XAdvance = %d, want -50", i, positions[i].XAdvance)
		}
	}
	if positions[3].XAdvance != 0 {
		t.Errorf("positions[3].XAdvance = %d, want 0", positions[3].XAdvance)
	}
}

func TestExtensionPosWithPairPos(t *testing.T) {
	// Test Extension lookup wrapping a PairPos (kerning)
	// PairPos: A+V = -80 kerning
	firstGlyphs := []GlyphID{65} // A
	pairs := [][]struct {
		second GlyphID
		kern   int16
	}{
		{{86, -80}}, // A+V
	}
	pairPos := buildPairPosFormat1(firstGlyphs, pairs)

	// Wrap in Extension
	extensionSubtable := buildExtensionPosSubtable(GPOSTypePair, pairPos)

	// Build lookup with Extension type
	lookup := buildGPOSLookup(GPOSTypeExtension, [][]byte{extensionSubtable})
	gposData := buildGPOSTable([][]byte{lookup})

	gpos, err := ParseGPOS(gposData)
	if err != nil {
		t.Fatalf("ParseGPOS failed: %v", err)
	}

	// Apply lookup directly (not ApplyKerning, which filters by Type)
	glyphs := []GlyphID{65, 86, 67} // A, V, C
	positions := make([]GlyphPosition, len(glyphs))
	gpos.ApplyLookup(0, glyphs, positions)

	// A should have kerning -80 (before V)
	if positions[0].XAdvance != -80 {
		t.Errorf("positions[0].XAdvance = %d, want -80", positions[0].XAdvance)
	}
	// V and C should have no kerning
	if positions[1].XAdvance != 0 {
		t.Errorf("positions[1].XAdvance = %d, want 0", positions[1].XAdvance)
	}
	if positions[2].XAdvance != 0 {
		t.Errorf("positions[2].XAdvance = %d, want 0", positions[2].XAdvance)
	}
}

func TestExtensionPosInvalidFormat(t *testing.T) {
	// Test that invalid extension format is handled gracefully
	singlePos := buildSinglePosFormat1([]GlyphID{65}, ValueFormatXAdvance, ValueRecord{XAdvance: -50})

	// Build extension with invalid format (2 instead of 1)
	data := make([]byte, 8+len(singlePos))
	binary.BigEndian.PutUint16(data[0:], 2) // format = 2 (INVALID)
	binary.BigEndian.PutUint16(data[2:], GPOSTypeSingle)
	binary.BigEndian.PutUint32(data[4:], 8)
	copy(data[8:], singlePos)

	lookup := buildGPOSLookup(GPOSTypeExtension, [][]byte{data})
	gposData := buildGPOSTable([][]byte{lookup})

	gpos, err := ParseGPOS(gposData)
	if err != nil {
		t.Fatalf("ParseGPOS failed: %v", err)
	}

	// The lookup should have no subtables (invalid extension was skipped)
	glyphs := []GlyphID{65}
	positions := make([]GlyphPosition, 1)
	gpos.ApplyLookup(0, glyphs, positions)

	// Should be unchanged (no valid subtable to apply)
	if positions[0].XAdvance != 0 {
		t.Errorf("positions[0].XAdvance = %d, want 0 (unchanged)", positions[0].XAdvance)
	}
}

func TestExtensionPosMultipleSubtables(t *testing.T) {
	// Test Extension lookup with multiple subtables
	singlePos1 := buildSinglePosFormat1([]GlyphID{65}, ValueFormatXAdvance, ValueRecord{XAdvance: -30})
	singlePos2 := buildSinglePosFormat1([]GlyphID{66}, ValueFormatXAdvance, ValueRecord{XAdvance: -40})

	// Wrap both in Extension
	ext1 := buildExtensionPosSubtable(GPOSTypeSingle, singlePos1)
	ext2 := buildExtensionPosSubtable(GPOSTypeSingle, singlePos2)

	// Build lookup with Extension type and multiple subtables
	lookup := buildGPOSLookup(GPOSTypeExtension, [][]byte{ext1, ext2})
	gposData := buildGPOSTable([][]byte{lookup})

	gpos, err := ParseGPOS(gposData)
	if err != nil {
		t.Fatalf("ParseGPOS failed: %v", err)
	}

	// Apply lookup
	glyphs := []GlyphID{65, 66, 67}
	positions := make([]GlyphPosition, len(glyphs))
	gpos.ApplyLookup(0, glyphs, positions)

	// 65 should get -30 from first subtable
	if positions[0].XAdvance != -30 {
		t.Errorf("positions[0].XAdvance = %d, want -30", positions[0].XAdvance)
	}
	// 66 should get -40 from second subtable
	if positions[1].XAdvance != -40 {
		t.Errorf("positions[1].XAdvance = %d, want -40", positions[1].XAdvance)
	}
	// 67 not covered
	if positions[2].XAdvance != 0 {
		t.Errorf("positions[2].XAdvance = %d, want 0", positions[2].XAdvance)
	}
}
