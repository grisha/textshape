package ot

import (
	"encoding/binary"
	"testing"
)

// Helper to build a minimal cmap table for testing
func buildCmapTable(subtables ...[]byte) []byte {
	// cmap header: version (2) + numTables (2)
	numTables := len(subtables)
	headerSize := 4 + numTables*8 // 8 bytes per encoding record

	data := make([]byte, headerSize)
	binary.BigEndian.PutUint16(data[0:], 0)                 // version
	binary.BigEndian.PutUint16(data[2:], uint16(numTables)) // numTables

	offset := headerSize
	for i, st := range subtables {
		recordOff := 4 + i*8
		binary.BigEndian.PutUint16(data[recordOff:], 3)   // platformID = Windows
		binary.BigEndian.PutUint16(data[recordOff+2:], 1) // encodingID = BMP
		binary.BigEndian.PutUint32(data[recordOff+4:], uint32(offset))

		data = append(data, st...)
		offset += len(st)
	}

	return data
}

// Build a format 4 subtable for testing
func buildFormat4(mappings map[uint16]uint16) []byte {
	// Collect and sort codepoints
	cps := make([]uint16, 0, len(mappings))
	for cp := range mappings {
		cps = append(cps, cp)
	}
	// Sort
	for i := 0; i < len(cps); i++ {
		for j := i + 1; j < len(cps); j++ {
			if cps[i] > cps[j] {
				cps[i], cps[j] = cps[j], cps[i]
			}
		}
	}

	// Build segments
	type segment struct {
		startCode, endCode uint16
		delta              int16
	}
	var segments []segment

	if len(cps) > 0 {
		start := cps[0]
		end := cps[0]
		delta := int16(mappings[start]) - int16(start)

		for i := 1; i < len(cps); i++ {
			cp := cps[i]
			expectedGid := int16(end) + 1 + delta

			if cp == end+1 && int16(mappings[cp]) == expectedGid {
				// Continue segment
				end = cp
			} else {
				// End current segment, start new one
				segments = append(segments, segment{start, end, delta})
				start = cp
				end = cp
				delta = int16(mappings[cp]) - int16(cp)
			}
		}
		segments = append(segments, segment{start, end, delta})
	}

	// Add sentinel segment
	segments = append(segments, segment{0xFFFF, 0xFFFF, 1})

	segCount := len(segments)
	segCountX2 := segCount * 2

	// Calculate sizes
	headerSize := 14
	arraySize := segCountX2 * 4             // endCode, reservedPad, startCode, idDelta, idRangeOffset
	totalSize := headerSize + arraySize + 2 // +2 for reservedPad

	data := make([]byte, totalSize)

	// Header
	binary.BigEndian.PutUint16(data[0:], 4)                 // format
	binary.BigEndian.PutUint16(data[2:], uint16(totalSize)) // length
	binary.BigEndian.PutUint16(data[4:], 0)                 // language
	binary.BigEndian.PutUint16(data[6:], uint16(segCountX2))
	// searchRange, entrySelector, rangeShift - simplified
	binary.BigEndian.PutUint16(data[8:], uint16(segCountX2))
	binary.BigEndian.PutUint16(data[10:], 0)
	binary.BigEndian.PutUint16(data[12:], 0)

	// Arrays
	endCodeOff := 14
	startCodeOff := endCodeOff + segCountX2 + 2
	idDeltaOff := startCodeOff + segCountX2
	idRangeOffOff := idDeltaOff + segCountX2

	for i, seg := range segments {
		binary.BigEndian.PutUint16(data[endCodeOff+i*2:], seg.endCode)
		binary.BigEndian.PutUint16(data[startCodeOff+i*2:], seg.startCode)
		binary.BigEndian.PutUint16(data[idDeltaOff+i*2:], uint16(seg.delta))
		binary.BigEndian.PutUint16(data[idRangeOffOff+i*2:], 0) // Use delta, not rangeOffset
	}

	return data
}

// Build a format 12 subtable for testing
func buildFormat12(mappings map[uint32]uint16) []byte {
	// Collect and sort codepoints
	cps := make([]uint32, 0, len(mappings))
	for cp := range mappings {
		cps = append(cps, cp)
	}
	for i := 0; i < len(cps); i++ {
		for j := i + 1; j < len(cps); j++ {
			if cps[i] > cps[j] {
				cps[i], cps[j] = cps[j], cps[i]
			}
		}
	}

	// Build groups
	type group struct {
		startCharCode, endCharCode, startGlyphID uint32
	}
	var groups []group

	if len(cps) > 0 {
		start := cps[0]
		end := cps[0]
		startGid := uint32(mappings[start])

		for i := 1; i < len(cps); i++ {
			cp := cps[i]
			expectedGid := startGid + (end - start) + 1

			if cp == end+1 && uint32(mappings[cp]) == expectedGid {
				end = cp
			} else {
				groups = append(groups, group{start, end, startGid})
				start = cp
				end = cp
				startGid = uint32(mappings[cp])
			}
		}
		groups = append(groups, group{start, end, startGid})
	}

	numGroups := len(groups)
	totalSize := 16 + numGroups*12

	data := make([]byte, totalSize)

	// Header
	binary.BigEndian.PutUint16(data[0:], 12)                // format
	binary.BigEndian.PutUint16(data[2:], 0)                 // reserved
	binary.BigEndian.PutUint32(data[4:], uint32(totalSize)) // length
	binary.BigEndian.PutUint32(data[8:], 0)                 // language
	binary.BigEndian.PutUint32(data[12:], uint32(numGroups))

	// Groups
	off := 16
	for _, g := range groups {
		binary.BigEndian.PutUint32(data[off:], g.startCharCode)
		binary.BigEndian.PutUint32(data[off+4:], g.endCharCode)
		binary.BigEndian.PutUint32(data[off+8:], g.startGlyphID)
		off += 12
	}

	return data
}

func TestCmapFormat4Basic(t *testing.T) {
	mappings := map[uint16]uint16{
		'A': 1,
		'B': 2,
		'C': 3,
	}

	subtable := buildFormat4(mappings)
	cmapData := buildCmapTable(subtable)

	cmap, err := ParseCmap(cmapData)
	if err != nil {
		t.Fatalf("ParseCmap failed: %v", err)
	}

	tests := []struct {
		cp        Codepoint
		wantGid   GlyphID
		wantFound bool
	}{
		{'A', 1, true},
		{'B', 2, true},
		{'C', 3, true},
		{'D', 0, false},
		{0, 0, false},
	}

	for _, tt := range tests {
		gid, found := cmap.Lookup(tt.cp)
		if found != tt.wantFound || gid != tt.wantGid {
			t.Errorf("Lookup(%q) = (%d, %v), want (%d, %v)",
				rune(tt.cp), gid, found, tt.wantGid, tt.wantFound)
		}
	}
}

func TestCmapFormat4Range(t *testing.T) {
	// Test a contiguous range
	mappings := map[uint16]uint16{
		'a': 10,
		'b': 11,
		'c': 12,
		'd': 13,
		'e': 14,
	}

	subtable := buildFormat4(mappings)
	cmapData := buildCmapTable(subtable)

	cmap, err := ParseCmap(cmapData)
	if err != nil {
		t.Fatalf("ParseCmap failed: %v", err)
	}

	for cp, wantGid := range mappings {
		gid, found := cmap.Lookup(Codepoint(cp))
		if !found {
			t.Errorf("Lookup(%q) not found, want %d", rune(cp), wantGid)
		} else if gid != GlyphID(wantGid) {
			t.Errorf("Lookup(%q) = %d, want %d", rune(cp), gid, wantGid)
		}
	}
}

func TestCmapFormat12Basic(t *testing.T) {
	mappings := map[uint32]uint16{
		'A':     1,
		'B':     2,
		0x1F600: 100, // Emoji (beyond BMP)
	}

	subtable := buildFormat12(mappings)

	// Build cmap with format 12 subtable
	// Need platform 3, encoding 10 for format 12
	headerSize := 4 + 8
	data := make([]byte, headerSize)
	binary.BigEndian.PutUint16(data[0:], 0)  // version
	binary.BigEndian.PutUint16(data[2:], 1)  // numTables
	binary.BigEndian.PutUint16(data[4:], 3)  // platformID = Windows
	binary.BigEndian.PutUint16(data[6:], 10) // encodingID = UCS-4
	binary.BigEndian.PutUint32(data[8:], uint32(headerSize))
	data = append(data, subtable...)

	cmap, err := ParseCmap(data)
	if err != nil {
		t.Fatalf("ParseCmap failed: %v", err)
	}

	tests := []struct {
		cp        Codepoint
		wantGid   GlyphID
		wantFound bool
	}{
		{'A', 1, true},
		{'B', 2, true},
		{0x1F600, 100, true},
		{'C', 0, false},
	}

	for _, tt := range tests {
		gid, found := cmap.Lookup(tt.cp)
		if found != tt.wantFound || gid != tt.wantGid {
			t.Errorf("Lookup(0x%X) = (%d, %v), want (%d, %v)",
				tt.cp, gid, found, tt.wantGid, tt.wantFound)
		}
	}
}

func TestCmapFormat12Range(t *testing.T) {
	// Test contiguous range in supplementary plane
	mappings := map[uint32]uint16{
		0x10000: 500,
		0x10001: 501,
		0x10002: 502,
		0x10003: 503,
	}

	subtable := buildFormat12(mappings)

	headerSize := 4 + 8
	data := make([]byte, headerSize)
	binary.BigEndian.PutUint16(data[0:], 0)
	binary.BigEndian.PutUint16(data[2:], 1)
	binary.BigEndian.PutUint16(data[4:], 3)
	binary.BigEndian.PutUint16(data[6:], 10)
	binary.BigEndian.PutUint32(data[8:], uint32(headerSize))
	data = append(data, subtable...)

	cmap, err := ParseCmap(data)
	if err != nil {
		t.Fatalf("ParseCmap failed: %v", err)
	}

	for cp, wantGid := range mappings {
		gid, found := cmap.Lookup(Codepoint(cp))
		if !found {
			t.Errorf("Lookup(0x%X) not found, want %d", cp, wantGid)
		} else if gid != GlyphID(wantGid) {
			t.Errorf("Lookup(0x%X) = %d, want %d", cp, gid, wantGid)
		}
	}

	// Check not found
	if gid, found := cmap.Lookup(0x10004); found {
		t.Errorf("Lookup(0x10004) = %d, want not found", gid)
	}
}

func TestSubtablePriority(t *testing.T) {
	// Symbol should have highest priority
	if p := getSubtablePriority(3, 0); p != 100 {
		t.Errorf("Symbol priority = %d, want 100", p)
	}

	// Windows UCS-4 should be high
	if p := getSubtablePriority(3, 10); p < 80 {
		t.Errorf("Windows UCS-4 priority = %d, want >= 80", p)
	}

	// Windows BMP should be medium
	if p := getSubtablePriority(3, 1); p < 70 {
		t.Errorf("Windows BMP priority = %d, want >= 70", p)
	}

	// UCS-4 should be higher than BMP
	ucs4 := getSubtablePriority(3, 10)
	bmp := getSubtablePriority(3, 1)
	if ucs4 <= bmp {
		t.Errorf("UCS-4 priority (%d) should be > BMP priority (%d)", ucs4, bmp)
	}
}

func TestParserBasic(t *testing.T) {
	data := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07}
	p := NewParser(data)

	// Read U16
	v16, err := p.U16()
	if err != nil {
		t.Fatalf("U16 failed: %v", err)
	}
	if v16 != 0x0001 {
		t.Errorf("U16 = 0x%04X, want 0x0001", v16)
	}

	// Read U32
	v32, err := p.U32()
	if err != nil {
		t.Fatalf("U32 failed: %v", err)
	}
	if v32 != 0x02030405 {
		t.Errorf("U32 = 0x%08X, want 0x02030405", v32)
	}

	// Remaining
	if p.Remaining() != 2 {
		t.Errorf("Remaining = %d, want 2", p.Remaining())
	}
}

func TestTag(t *testing.T) {
	tag := MakeTag('c', 'm', 'a', 'p')
	if tag != TagCmap {
		t.Errorf("MakeTag('c','m','a','p') = %v, want %v", tag, TagCmap)
	}

	if tag.String() != "cmap" {
		t.Errorf("Tag.String() = %q, want %q", tag.String(), "cmap")
	}
}
