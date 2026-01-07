package ot

import (
	"encoding/binary"
	"testing"
)

// --- GDEF Test Helpers ---

// buildGDEFHeader builds a GDEF table header.
func buildGDEFHeader(versionMajor, versionMinor uint16, glyphClassDefOff, attachListOff, ligCaretListOff, markAttachClassDefOff, markGlyphSetsDefOff uint16) []byte {
	size := 12
	if versionMinor >= 2 {
		size = 14
	}
	data := make([]byte, size)
	binary.BigEndian.PutUint16(data[0:], versionMajor)
	binary.BigEndian.PutUint16(data[2:], versionMinor)
	binary.BigEndian.PutUint16(data[4:], glyphClassDefOff)
	binary.BigEndian.PutUint16(data[6:], attachListOff)
	binary.BigEndian.PutUint16(data[8:], ligCaretListOff)
	binary.BigEndian.PutUint16(data[10:], markAttachClassDefOff)
	if versionMinor >= 2 {
		binary.BigEndian.PutUint16(data[12:], markGlyphSetsDefOff)
	}
	return data
}

// buildAttachList builds an AttachList table.
func buildAttachList(coverageGlyphs []GlyphID, attachPoints [][]uint16) []byte {
	// Calculate size
	glyphCount := len(attachPoints)
	headerSize := 4 + glyphCount*2 // coverage offset + glyph count + offsets

	// Calculate attachment point tables size
	attachTablesSize := 0
	for _, points := range attachPoints {
		attachTablesSize += 2 + len(points)*2 // point count + points
	}

	coverage := buildCoverageFormat1(coverageGlyphs)
	totalSize := headerSize + attachTablesSize + len(coverage)

	data := make([]byte, totalSize)

	// Coverage offset (after all attach point tables)
	covOffset := headerSize + attachTablesSize
	binary.BigEndian.PutUint16(data[0:], uint16(covOffset))
	binary.BigEndian.PutUint16(data[2:], uint16(glyphCount))

	// Attachment point offsets and tables
	offset := headerSize
	for i, points := range attachPoints {
		binary.BigEndian.PutUint16(data[4+i*2:], uint16(offset))

		binary.BigEndian.PutUint16(data[offset:], uint16(len(points)))
		for j, pt := range points {
			binary.BigEndian.PutUint16(data[offset+2+j*2:], pt)
		}
		offset += 2 + len(points)*2
	}

	// Copy coverage
	copy(data[covOffset:], coverage)

	return data
}

// buildLigCaretList builds a LigCaretList table.
func buildLigCaretList(coverageGlyphs []GlyphID, caretValues [][]CaretValue) []byte {
	ligGlyphCount := len(caretValues)
	headerSize := 4 + ligGlyphCount*2 // coverage offset + lig glyph count + offsets

	// Calculate LigGlyph tables size
	var ligGlyphSizes []int
	ligGlyphsSize := 0
	for _, carets := range caretValues {
		size := 2 + len(carets)*2 // caret count + caret offsets
		size += len(carets) * 4   // CaretValue tables (format + value each)
		ligGlyphSizes = append(ligGlyphSizes, size)
		ligGlyphsSize += size
	}

	coverage := buildCoverageFormat1(coverageGlyphs)
	totalSize := headerSize + ligGlyphsSize + len(coverage)

	data := make([]byte, totalSize)

	// Coverage offset
	covOffset := headerSize + ligGlyphsSize
	binary.BigEndian.PutUint16(data[0:], uint16(covOffset))
	binary.BigEndian.PutUint16(data[2:], uint16(ligGlyphCount))

	// LigGlyph offsets and tables
	offset := headerSize
	for i, carets := range caretValues {
		binary.BigEndian.PutUint16(data[4+i*2:], uint16(offset))

		// LigGlyph table
		binary.BigEndian.PutUint16(data[offset:], uint16(len(carets)))

		// CaretValue offsets (relative to LigGlyph)
		caretTableOffset := 2 + len(carets)*2
		for j := range carets {
			binary.BigEndian.PutUint16(data[offset+2+j*2:], uint16(caretTableOffset+j*4))
		}

		// CaretValue tables
		for j := range carets {
			cvOff := offset + caretTableOffset + j*4
			binary.BigEndian.PutUint16(data[cvOff:], carets[j].format)
			switch carets[j].format {
			case 1, 3:
				binary.BigEndian.PutUint16(data[cvOff+2:], uint16(carets[j].coordinate))
			case 2:
				binary.BigEndian.PutUint16(data[cvOff+2:], carets[j].pointIndex)
			}
		}

		offset += ligGlyphSizes[i]
	}

	// Copy coverage
	copy(data[covOffset:], coverage)

	return data
}

// buildMarkGlyphSetsDef builds a MarkGlyphSetsDef table.
func buildMarkGlyphSetsDef(markSets [][]GlyphID) []byte {
	markSetCount := len(markSets)
	headerSize := 4 + markSetCount*4 // format + count + 32-bit offsets

	// Build coverage tables
	var coverages [][]byte
	coveragesSize := 0
	for _, glyphs := range markSets {
		cov := buildCoverageFormat1(glyphs)
		coverages = append(coverages, cov)
		coveragesSize += len(cov)
	}

	totalSize := headerSize + coveragesSize
	data := make([]byte, totalSize)

	binary.BigEndian.PutUint16(data[0:], 1) // format
	binary.BigEndian.PutUint16(data[2:], uint16(markSetCount))

	// Coverage offsets and tables
	offset := headerSize
	for i, cov := range coverages {
		binary.BigEndian.PutUint32(data[4+i*4:], uint32(offset))
		copy(data[offset:], cov)
		offset += len(cov)
	}

	return data
}

// --- GDEF Tests ---

func TestGDEFBasicParsing(t *testing.T) {
	// Build a minimal GDEF v1.0 with just glyph class definitions
	classDefData := buildClassDefFormat1(65, []uint16{
		GlyphClassBase,      // glyph 65 (A) - Base
		GlyphClassBase,      // glyph 66 (B) - Base
		GlyphClassMark,      // glyph 67 (C) - Mark
		GlyphClassLigature,  // glyph 68 (D) - Ligature
		GlyphClassComponent, // glyph 69 (E) - Component
	})

	header := buildGDEFHeader(1, 0, 12, 0, 0, 0, 0)
	data := append(header, classDefData...)

	gdef, err := ParseGDEF(data)
	if err != nil {
		t.Fatalf("ParseGDEF failed: %v", err)
	}

	major, minor := gdef.Version()
	if major != 1 || minor != 0 {
		t.Errorf("Version = (%d, %d), want (1, 0)", major, minor)
	}

	if !gdef.HasGlyphClasses() {
		t.Error("HasGlyphClasses() = false, want true")
	}

	tests := []struct {
		glyph    GlyphID
		expected int
	}{
		{65, GlyphClassBase},
		{66, GlyphClassBase},
		{67, GlyphClassMark},
		{68, GlyphClassLigature},
		{69, GlyphClassComponent},
		{70, GlyphClassUnclassified}, // Not in class def
		{100, GlyphClassUnclassified},
	}

	for _, tt := range tests {
		got := gdef.GetGlyphClass(tt.glyph)
		if got != tt.expected {
			t.Errorf("GetGlyphClass(%d) = %d, want %d", tt.glyph, got, tt.expected)
		}
	}
}

func TestGDEFGlyphClassHelpers(t *testing.T) {
	classDefData := buildClassDefFormat1(65, []uint16{
		GlyphClassBase,
		GlyphClassLigature,
		GlyphClassMark,
		GlyphClassComponent,
	})

	header := buildGDEFHeader(1, 0, 12, 0, 0, 0, 0)
	data := append(header, classDefData...)

	gdef, err := ParseGDEF(data)
	if err != nil {
		t.Fatalf("ParseGDEF failed: %v", err)
	}

	if !gdef.IsBaseGlyph(65) {
		t.Error("IsBaseGlyph(65) = false, want true")
	}
	if !gdef.IsLigatureGlyph(66) {
		t.Error("IsLigatureGlyph(66) = false, want true")
	}
	if !gdef.IsMarkGlyph(67) {
		t.Error("IsMarkGlyph(67) = false, want true")
	}
	if !gdef.IsComponentGlyph(68) {
		t.Error("IsComponentGlyph(68) = false, want true")
	}

	// Test negative cases
	if gdef.IsBaseGlyph(66) {
		t.Error("IsBaseGlyph(66) = true, want false")
	}
	if gdef.IsMarkGlyph(65) {
		t.Error("IsMarkGlyph(65) = true, want false")
	}
}

func TestGDEFMarkAttachClass(t *testing.T) {
	// Build GDEF with mark attachment class
	markAttachClassDefData := buildClassDefFormat1(100, []uint16{
		1, // glyph 100 - class 1
		1, // glyph 101 - class 1
		2, // glyph 102 - class 2
		2, // glyph 103 - class 2
		3, // glyph 104 - class 3
	})

	header := buildGDEFHeader(1, 0, 0, 0, 0, 12, 0)
	data := append(header, markAttachClassDefData...)

	gdef, err := ParseGDEF(data)
	if err != nil {
		t.Fatalf("ParseGDEF failed: %v", err)
	}

	if !gdef.HasMarkAttachClasses() {
		t.Error("HasMarkAttachClasses() = false, want true")
	}

	tests := []struct {
		glyph    GlyphID
		expected int
	}{
		{100, 1},
		{101, 1},
		{102, 2},
		{103, 2},
		{104, 3},
		{105, 0}, // Not in class def
	}

	for _, tt := range tests {
		got := gdef.GetMarkAttachClass(tt.glyph)
		if got != tt.expected {
			t.Errorf("GetMarkAttachClass(%d) = %d, want %d", tt.glyph, got, tt.expected)
		}
	}
}

func TestGDEFAttachList(t *testing.T) {
	attachListData := buildAttachList(
		[]GlyphID{65, 66, 67},
		[][]uint16{
			{0, 5, 10},   // glyph 65: points 0, 5, 10
			{3},          // glyph 66: point 3
			{1, 2, 3, 4}, // glyph 67: points 1, 2, 3, 4
		},
	)

	header := buildGDEFHeader(1, 0, 0, 12, 0, 0, 0)
	data := append(header, attachListData...)

	gdef, err := ParseGDEF(data)
	if err != nil {
		t.Fatalf("ParseGDEF failed: %v", err)
	}

	if !gdef.HasAttachList() {
		t.Error("HasAttachList() = false, want true")
	}

	tests := []struct {
		glyph    GlyphID
		expected []uint16
	}{
		{65, []uint16{0, 5, 10}},
		{66, []uint16{3}},
		{67, []uint16{1, 2, 3, 4}},
		{68, nil}, // Not in attach list
	}

	for _, tt := range tests {
		got := gdef.GetAttachPoints(tt.glyph)
		if len(got) != len(tt.expected) {
			t.Errorf("GetAttachPoints(%d) length = %d, want %d", tt.glyph, len(got), len(tt.expected))
			continue
		}
		for i, v := range got {
			if v != tt.expected[i] {
				t.Errorf("GetAttachPoints(%d)[%d] = %d, want %d", tt.glyph, i, v, tt.expected[i])
			}
		}
	}
}

func TestGDEFLigCaretList(t *testing.T) {
	ligCaretListData := buildLigCaretList(
		[]GlyphID{200, 201},
		[][]CaretValue{
			{
				{format: 1, coordinate: 100},
				{format: 1, coordinate: 200},
			},
			{
				{format: 2, pointIndex: 5},
			},
		},
	)

	header := buildGDEFHeader(1, 0, 0, 0, 12, 0, 0)
	data := append(header, ligCaretListData...)

	gdef, err := ParseGDEF(data)
	if err != nil {
		t.Fatalf("ParseGDEF failed: %v", err)
	}

	if !gdef.HasLigCaretList() {
		t.Error("HasLigCaretList() = false, want true")
	}

	// Test caret count
	if count := gdef.GetLigCaretCount(200); count != 2 {
		t.Errorf("GetLigCaretCount(200) = %d, want 2", count)
	}
	if count := gdef.GetLigCaretCount(201); count != 1 {
		t.Errorf("GetLigCaretCount(201) = %d, want 1", count)
	}
	if count := gdef.GetLigCaretCount(202); count != 0 {
		t.Errorf("GetLigCaretCount(202) = %d, want 0", count)
	}

	// Test caret values
	carets200 := gdef.GetLigCarets(200)
	if len(carets200) != 2 {
		t.Fatalf("GetLigCarets(200) length = %d, want 2", len(carets200))
	}
	if carets200[0].Format() != 1 || carets200[0].Coordinate() != 100 {
		t.Errorf("GetLigCarets(200)[0] = (format=%d, coord=%d), want (1, 100)",
			carets200[0].Format(), carets200[0].Coordinate())
	}
	if carets200[1].Format() != 1 || carets200[1].Coordinate() != 200 {
		t.Errorf("GetLigCarets(200)[1] = (format=%d, coord=%d), want (1, 200)",
			carets200[1].Format(), carets200[1].Coordinate())
	}

	carets201 := gdef.GetLigCarets(201)
	if len(carets201) != 1 {
		t.Fatalf("GetLigCarets(201) length = %d, want 1", len(carets201))
	}
	if carets201[0].Format() != 2 || carets201[0].PointIndex() != 5 {
		t.Errorf("GetLigCarets(201)[0] = (format=%d, pointIndex=%d), want (2, 5)",
			carets201[0].Format(), carets201[0].PointIndex())
	}
}

func TestGDEFMarkGlyphSets(t *testing.T) {
	markGlyphSetsData := buildMarkGlyphSetsDef([][]GlyphID{
		{100, 101, 102},      // Set 0
		{200, 201},           // Set 1
		{300, 301, 302, 303}, // Set 2
	})

	header := buildGDEFHeader(1, 2, 0, 0, 0, 0, 14)
	data := append(header, markGlyphSetsData...)

	gdef, err := ParseGDEF(data)
	if err != nil {
		t.Fatalf("ParseGDEF failed: %v", err)
	}

	major, minor := gdef.Version()
	if major != 1 || minor != 2 {
		t.Errorf("Version = (%d, %d), want (1, 2)", major, minor)
	}

	if !gdef.HasMarkGlyphSets() {
		t.Error("HasMarkGlyphSets() = false, want true")
	}

	if count := gdef.MarkGlyphSetCount(); count != 3 {
		t.Errorf("MarkGlyphSetCount() = %d, want 3", count)
	}

	// Test set membership
	tests := []struct {
		glyph    GlyphID
		setIndex int
		expected bool
	}{
		{100, 0, true},
		{101, 0, true},
		{102, 0, true},
		{103, 0, false},
		{200, 1, true},
		{201, 1, true},
		{202, 1, false},
		{300, 2, true},
		{301, 2, true},
		{302, 2, true},
		{303, 2, true},
		{304, 2, false},
		{100, 1, false}, // glyph 100 not in set 1
		{200, 0, false}, // glyph 200 not in set 0
	}

	for _, tt := range tests {
		got := gdef.IsInMarkGlyphSet(tt.glyph, tt.setIndex)
		if got != tt.expected {
			t.Errorf("IsInMarkGlyphSet(%d, %d) = %v, want %v",
				tt.glyph, tt.setIndex, got, tt.expected)
		}
	}

	// Test invalid set index
	if gdef.IsInMarkGlyphSet(100, -1) {
		t.Error("IsInMarkGlyphSet(100, -1) = true, want false")
	}
	if gdef.IsInMarkGlyphSet(100, 10) {
		t.Error("IsInMarkGlyphSet(100, 10) = true, want false")
	}
}

func TestGDEFComplete(t *testing.T) {
	// Build a complete GDEF table with all components
	glyphClassDefData := buildClassDefFormat1(65, []uint16{
		GlyphClassBase,
		GlyphClassBase,
		GlyphClassMark,
		GlyphClassLigature,
	})

	markAttachClassDefData := buildClassDefFormat1(67, []uint16{1, 0, 2})

	attachListData := buildAttachList(
		[]GlyphID{65, 66},
		[][]uint16{{0, 5}, {3}},
	)

	ligCaretListData := buildLigCaretList(
		[]GlyphID{68},
		[][]CaretValue{{{format: 1, coordinate: 150}}},
	)

	markGlyphSetsData := buildMarkGlyphSetsDef([][]GlyphID{{67}})

	// Calculate offsets
	headerSize := 14 // v1.2 header
	glyphClassDefOff := headerSize
	attachListOff := glyphClassDefOff + len(glyphClassDefData)
	ligCaretListOff := attachListOff + len(attachListData)
	markAttachClassDefOff := ligCaretListOff + len(ligCaretListData)
	markGlyphSetsDefOff := markAttachClassDefOff + len(markAttachClassDefData)

	header := buildGDEFHeader(1, 2,
		uint16(glyphClassDefOff),
		uint16(attachListOff),
		uint16(ligCaretListOff),
		uint16(markAttachClassDefOff),
		uint16(markGlyphSetsDefOff),
	)

	data := header
	data = append(data, glyphClassDefData...)
	data = append(data, attachListData...)
	data = append(data, ligCaretListData...)
	data = append(data, markAttachClassDefData...)
	data = append(data, markGlyphSetsData...)

	gdef, err := ParseGDEF(data)
	if err != nil {
		t.Fatalf("ParseGDEF failed: %v", err)
	}

	// Verify all components are present
	if !gdef.HasGlyphClasses() {
		t.Error("HasGlyphClasses() = false, want true")
	}
	if !gdef.HasAttachList() {
		t.Error("HasAttachList() = false, want true")
	}
	if !gdef.HasLigCaretList() {
		t.Error("HasLigCaretList() = false, want true")
	}
	if !gdef.HasMarkAttachClasses() {
		t.Error("HasMarkAttachClasses() = false, want true")
	}
	if !gdef.HasMarkGlyphSets() {
		t.Error("HasMarkGlyphSets() = false, want true")
	}

	// Verify some values
	if gdef.GetGlyphClass(65) != GlyphClassBase {
		t.Errorf("GetGlyphClass(65) = %d, want %d", gdef.GetGlyphClass(65), GlyphClassBase)
	}
	if gdef.GetMarkAttachClass(67) != 1 {
		t.Errorf("GetMarkAttachClass(67) = %d, want 1", gdef.GetMarkAttachClass(67))
	}
	if pts := gdef.GetAttachPoints(65); len(pts) != 2 {
		t.Errorf("GetAttachPoints(65) length = %d, want 2", len(pts))
	}
	if gdef.GetLigCaretCount(68) != 1 {
		t.Errorf("GetLigCaretCount(68) = %d, want 1", gdef.GetLigCaretCount(68))
	}
	if !gdef.IsInMarkGlyphSet(67, 0) {
		t.Error("IsInMarkGlyphSet(67, 0) = false, want true")
	}
}

func TestGDEFNilHandling(t *testing.T) {
	// Build a minimal GDEF with no subtables
	header := buildGDEFHeader(1, 0, 0, 0, 0, 0, 0)

	gdef, err := ParseGDEF(header)
	if err != nil {
		t.Fatalf("ParseGDEF failed: %v", err)
	}

	// All queries should return safe defaults
	if gdef.HasGlyphClasses() {
		t.Error("HasGlyphClasses() = true, want false")
	}
	if gdef.HasAttachList() {
		t.Error("HasAttachList() = true, want false")
	}
	if gdef.HasLigCaretList() {
		t.Error("HasLigCaretList() = true, want false")
	}
	if gdef.HasMarkAttachClasses() {
		t.Error("HasMarkAttachClasses() = true, want false")
	}
	if gdef.HasMarkGlyphSets() {
		t.Error("HasMarkGlyphSets() = true, want false")
	}

	// Methods should handle nil gracefully
	if class := gdef.GetGlyphClass(65); class != 0 {
		t.Errorf("GetGlyphClass(65) = %d, want 0", class)
	}
	if class := gdef.GetMarkAttachClass(65); class != 0 {
		t.Errorf("GetMarkAttachClass(65) = %d, want 0", class)
	}
	if pts := gdef.GetAttachPoints(65); pts != nil {
		t.Errorf("GetAttachPoints(65) = %v, want nil", pts)
	}
	if count := gdef.GetLigCaretCount(65); count != 0 {
		t.Errorf("GetLigCaretCount(65) = %d, want 0", count)
	}
	if carets := gdef.GetLigCarets(65); carets != nil {
		t.Errorf("GetLigCarets(65) = %v, want nil", carets)
	}
	if count := gdef.MarkGlyphSetCount(); count != 0 {
		t.Errorf("MarkGlyphSetCount() = %d, want 0", count)
	}
	if gdef.IsInMarkGlyphSet(65, 0) {
		t.Error("IsInMarkGlyphSet(65, 0) = true, want false")
	}
}

func TestGDEFInvalidVersion(t *testing.T) {
	// Test invalid major version
	data := make([]byte, 14)
	binary.BigEndian.PutUint16(data[0:], 2) // Invalid major version
	binary.BigEndian.PutUint16(data[2:], 0)

	_, err := ParseGDEF(data)
	if err == nil {
		t.Error("ParseGDEF should fail for invalid major version")
	}

	// Test invalid minor version
	binary.BigEndian.PutUint16(data[0:], 1)
	binary.BigEndian.PutUint16(data[2:], 4) // Invalid minor version

	_, err = ParseGDEF(data)
	if err == nil {
		t.Error("ParseGDEF should fail for invalid minor version")
	}
}

func TestGDEFTooShort(t *testing.T) {
	data := make([]byte, 8) // Too short for GDEF header

	_, err := ParseGDEF(data)
	if err == nil {
		t.Error("ParseGDEF should fail for too short data")
	}
}

func TestGDEFClassDefFormat2(t *testing.T) {
	// Test with ClassDef Format 2 (range-based)
	classDefData := buildClassDefFormat2([]struct {
		start, end GlyphID
		class      uint16
	}{
		{65, 70, GlyphClassBase},       // A-F are base glyphs
		{100, 105, GlyphClassMark},     // Range of marks
		{200, 200, GlyphClassLigature}, // Single ligature
	})

	header := buildGDEFHeader(1, 0, 12, 0, 0, 0, 0)
	data := append(header, classDefData...)

	gdef, err := ParseGDEF(data)
	if err != nil {
		t.Fatalf("ParseGDEF failed: %v", err)
	}

	tests := []struct {
		glyph    GlyphID
		expected int
	}{
		{65, GlyphClassBase},
		{70, GlyphClassBase},
		{67, GlyphClassBase},
		{71, GlyphClassUnclassified}, // Just after range
		{100, GlyphClassMark},
		{105, GlyphClassMark},
		{102, GlyphClassMark},
		{200, GlyphClassLigature},
		{199, GlyphClassUnclassified},
		{201, GlyphClassUnclassified},
	}

	for _, tt := range tests {
		got := gdef.GetGlyphClass(tt.glyph)
		if got != tt.expected {
			t.Errorf("GetGlyphClass(%d) = %d, want %d", tt.glyph, got, tt.expected)
		}
	}
}

// --- GDEF Integration Tests ---

func TestShouldSkipGlyph(t *testing.T) {
	// Build GDEF with glyph classes
	classDefData := buildClassDefFormat1(65, []uint16{
		GlyphClassBase,      // glyph 65 - Base
		GlyphClassMark,      // glyph 66 - Mark
		GlyphClassLigature,  // glyph 67 - Ligature
		GlyphClassComponent, // glyph 68 - Component
	})

	header := buildGDEFHeader(1, 0, 12, 0, 0, 0, 0)
	data := append(header, classDefData...)

	gdef, err := ParseGDEF(data)
	if err != nil {
		t.Fatalf("ParseGDEF failed: %v", err)
	}

	tests := []struct {
		name       string
		glyph      GlyphID
		lookupFlag uint16
		expected   bool
	}{
		{"base with no flags", 65, 0, false},
		{"mark with no flags", 66, 0, false},
		{"ligature with no flags", 67, 0, false},

		{"base with IgnoreBaseGlyphs", 65, LookupFlagIgnoreBaseGlyphs, true},
		{"mark with IgnoreBaseGlyphs", 66, LookupFlagIgnoreBaseGlyphs, false},

		{"mark with IgnoreMarks", 66, LookupFlagIgnoreMarks, true},
		{"base with IgnoreMarks", 65, LookupFlagIgnoreMarks, false},

		{"ligature with IgnoreLigatures", 67, LookupFlagIgnoreLigatures, true},
		{"base with IgnoreLigatures", 65, LookupFlagIgnoreLigatures, false},

		{"base with all ignore flags", 65, LookupFlagIgnoreBaseGlyphs | LookupFlagIgnoreMarks | LookupFlagIgnoreLigatures, true},
		{"mark with all ignore flags", 66, LookupFlagIgnoreBaseGlyphs | LookupFlagIgnoreMarks | LookupFlagIgnoreLigatures, true},
		{"ligature with all ignore flags", 67, LookupFlagIgnoreBaseGlyphs | LookupFlagIgnoreMarks | LookupFlagIgnoreLigatures, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldSkipGlyph(tt.glyph, tt.lookupFlag, gdef, -1)
			if got != tt.expected {
				t.Errorf("shouldSkipGlyph(%d, 0x%04x) = %v, want %v", tt.glyph, tt.lookupFlag, got, tt.expected)
			}
		})
	}
}

func TestShouldSkipGlyphMarkAttachClass(t *testing.T) {
	// Build GDEF with glyph classes and mark attachment classes
	glyphClassDefData := buildClassDefFormat1(65, []uint16{
		GlyphClassBase, // glyph 65 - Base
		GlyphClassMark, // glyph 66 - Mark (class 1)
		GlyphClassMark, // glyph 67 - Mark (class 2)
		GlyphClassMark, // glyph 68 - Mark (class 1)
	})

	markAttachClassDefData := buildClassDefFormat1(66, []uint16{
		1, // glyph 66 - Mark attach class 1
		2, // glyph 67 - Mark attach class 2
		1, // glyph 68 - Mark attach class 1
	})

	// Calculate offsets
	headerSize := 12
	glyphClassDefOff := headerSize
	markAttachClassDefOff := glyphClassDefOff + len(glyphClassDefData)

	header := buildGDEFHeader(1, 0, uint16(glyphClassDefOff), 0, 0, uint16(markAttachClassDefOff), 0)
	data := header
	data = append(data, glyphClassDefData...)
	data = append(data, markAttachClassDefData...)

	gdef, err := ParseGDEF(data)
	if err != nil {
		t.Fatalf("ParseGDEF failed: %v", err)
	}

	// MarkAttachmentType is bits 8-15 of LookupFlag
	markAttachType1 := uint16(1) << 8
	markAttachType2 := uint16(2) << 8

	tests := []struct {
		name       string
		glyph      GlyphID
		lookupFlag uint16
		expected   bool
	}{
		{"mark class 1, filter class 1", 66, markAttachType1, false},
		{"mark class 2, filter class 1", 67, markAttachType1, true},
		{"mark class 1, filter class 2", 66, markAttachType2, true},
		{"mark class 2, filter class 2", 67, markAttachType2, false},
		{"base glyph, filter class 1", 65, markAttachType1, false}, // Base glyphs not filtered
		{"mark class 1, no filter", 66, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldSkipGlyph(tt.glyph, tt.lookupFlag, gdef, -1)
			if got != tt.expected {
				t.Errorf("shouldSkipGlyph(%d, 0x%04x) = %v, want %v", tt.glyph, tt.lookupFlag, got, tt.expected)
			}
		})
	}
}

func TestShouldSkipGlyphMarkFilteringSet(t *testing.T) {
	// Build GDEF v1.2 with mark filtering sets
	glyphClassDefData := buildClassDefFormat1(65, []uint16{
		GlyphClassBase, // glyph 65 - Base
		GlyphClassMark, // glyph 66 - Mark (in set 0)
		GlyphClassMark, // glyph 67 - Mark (in set 1)
		GlyphClassMark, // glyph 68 - Mark (in both sets)
	})

	markGlyphSetsData := buildMarkGlyphSetsDef([][]GlyphID{
		{66, 68}, // Set 0
		{67, 68}, // Set 1
	})

	// Calculate offsets
	headerSize := 14 // v1.2 header
	glyphClassDefOff := headerSize
	markGlyphSetsDefOff := glyphClassDefOff + len(glyphClassDefData)

	header := buildGDEFHeader(1, 2, uint16(glyphClassDefOff), 0, 0, 0, uint16(markGlyphSetsDefOff))
	data := header
	data = append(data, glyphClassDefData...)
	data = append(data, markGlyphSetsData...)

	gdef, err := ParseGDEF(data)
	if err != nil {
		t.Fatalf("ParseGDEF failed: %v", err)
	}

	tests := []struct {
		name             string
		glyph            GlyphID
		lookupFlag       uint16
		markFilteringSet int
		expected         bool
	}{
		{"mark in set 0, filter set 0", 66, LookupFlagUseMarkFilteringSet, 0, false},
		{"mark not in set 0, filter set 0", 67, LookupFlagUseMarkFilteringSet, 0, true},
		{"mark in set 1, filter set 1", 67, LookupFlagUseMarkFilteringSet, 1, false},
		{"mark in both sets, filter set 0", 68, LookupFlagUseMarkFilteringSet, 0, false},
		{"mark in both sets, filter set 1", 68, LookupFlagUseMarkFilteringSet, 1, false},
		{"base glyph, filter set 0", 65, LookupFlagUseMarkFilteringSet, 0, false}, // Base glyphs not filtered
		{"mark in set 0, no filter flag", 66, 0, -1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldSkipGlyph(tt.glyph, tt.lookupFlag, gdef, tt.markFilteringSet)
			if got != tt.expected {
				t.Errorf("shouldSkipGlyph(%d, 0x%04x, set=%d) = %v, want %v",
					tt.glyph, tt.lookupFlag, tt.markFilteringSet, got, tt.expected)
			}
		})
	}
}

func TestGPOSContextNextPrevGlyph(t *testing.T) {
	// Build GDEF with glyph classes
	classDefData := buildClassDefFormat1(65, []uint16{
		GlyphClassBase, // glyph 65 - Base
		GlyphClassMark, // glyph 66 - Mark
		GlyphClassBase, // glyph 67 - Base
		GlyphClassMark, // glyph 68 - Mark
		GlyphClassBase, // glyph 69 - Base
	})

	header := buildGDEFHeader(1, 0, 12, 0, 0, 0, 0)
	data := append(header, classDefData...)

	gdef, err := ParseGDEF(data)
	if err != nil {
		t.Fatalf("ParseGDEF failed: %v", err)
	}

	// Create a glyph sequence: Base, Mark, Base, Mark, Base
	glyphs := []GlyphID{65, 66, 67, 68, 69}
	positions := make([]GlyphPosition, len(glyphs))

	ctx := &GPOSContext{
		Glyphs:           glyphs,
		Positions:        positions,
		Index:            0,
		LookupFlag:       LookupFlagIgnoreMarks,
		GDEF:             gdef,
		MarkFilteringSet: -1,
	}

	// Test NextGlyph - should skip marks
	tests := []struct {
		startIndex   int
		expectedNext int
	}{
		{0, 0},  // At base, returns 0
		{1, 2},  // At mark, next non-mark is at 2
		{2, 2},  // At base, returns 2
		{3, 4},  // At mark, next non-mark is at 4
		{4, 4},  // At base, returns 4
		{5, -1}, // Past end
	}

	for _, tt := range tests {
		got := ctx.NextGlyph(tt.startIndex)
		if got != tt.expectedNext {
			t.Errorf("NextGlyph(%d) = %d, want %d", tt.startIndex, got, tt.expectedNext)
		}
	}

	// Test PrevGlyph - should skip marks
	prevTests := []struct {
		startIndex   int
		expectedPrev int
	}{
		{5, 4},  // From end, prev non-mark is at 4
		{4, 2},  // From base at 4, prev non-mark is at 2
		{3, 2},  // From mark at 3, prev non-mark is at 2
		{2, 0},  // From base at 2, prev non-mark is at 0
		{1, 0},  // From mark at 1, prev non-mark is at 0
		{0, -1}, // No prev
	}

	for _, tt := range prevTests {
		got := ctx.PrevGlyph(tt.startIndex)
		if got != tt.expectedPrev {
			t.Errorf("PrevGlyph(%d) = %d, want %d", tt.startIndex, got, tt.expectedPrev)
		}
	}
}

func TestGSUBContextNextPrevGlyph(t *testing.T) {
	// Build GDEF with glyph classes
	classDefData := buildClassDefFormat1(65, []uint16{
		GlyphClassBase,     // glyph 65 - Base
		GlyphClassLigature, // glyph 66 - Ligature
		GlyphClassBase,     // glyph 67 - Base
		GlyphClassLigature, // glyph 68 - Ligature
		GlyphClassBase,     // glyph 69 - Base
	})

	header := buildGDEFHeader(1, 0, 12, 0, 0, 0, 0)
	data := append(header, classDefData...)

	gdef, err := ParseGDEF(data)
	if err != nil {
		t.Fatalf("ParseGDEF failed: %v", err)
	}

	// Create a glyph sequence: Base, Ligature, Base, Ligature, Base
	glyphs := []GlyphID{65, 66, 67, 68, 69}

	ctx := &GSUBContext{
		Glyphs:           glyphs,
		Index:            0,
		LookupFlag:       LookupFlagIgnoreLigatures,
		GDEF:             gdef,
		MarkFilteringSet: -1,
	}

	// Test NextGlyph - should skip ligatures
	tests := []struct {
		startIndex   int
		expectedNext int
	}{
		{0, 0},  // At base, returns 0
		{1, 2},  // At ligature, next non-ligature is at 2
		{2, 2},  // At base, returns 2
		{3, 4},  // At ligature, next non-ligature is at 4
		{4, 4},  // At base, returns 4
		{5, -1}, // Past end
	}

	for _, tt := range tests {
		got := ctx.NextGlyph(tt.startIndex)
		if got != tt.expectedNext {
			t.Errorf("NextGlyph(%d) = %d, want %d", tt.startIndex, got, tt.expectedNext)
		}
	}
}
