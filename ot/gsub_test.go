package ot

import (
	"encoding/binary"
	"testing"
)

// Helper to build a Coverage table
func buildCoverageFormat1(glyphs []GlyphID) []byte {
	data := make([]byte, 4+len(glyphs)*2)
	binary.BigEndian.PutUint16(data[0:], 1) // format
	binary.BigEndian.PutUint16(data[2:], uint16(len(glyphs)))
	for i, g := range glyphs {
		binary.BigEndian.PutUint16(data[4+i*2:], uint16(g))
	}
	return data
}

func buildCoverageFormat2(ranges [][3]uint16) []byte {
	// ranges: [startGlyph, endGlyph, startCoverageIndex]
	data := make([]byte, 4+len(ranges)*6)
	binary.BigEndian.PutUint16(data[0:], 2) // format
	binary.BigEndian.PutUint16(data[2:], uint16(len(ranges)))
	for i, r := range ranges {
		off := 4 + i*6
		binary.BigEndian.PutUint16(data[off:], r[0])   // startGlyph
		binary.BigEndian.PutUint16(data[off+2:], r[1]) // endGlyph
		binary.BigEndian.PutUint16(data[off+4:], r[2]) // startCoverageIndex
	}
	return data
}

// Helper to build a SingleSubst Format 1 subtable
func buildSingleSubstFormat1(coverageGlyphs []GlyphID, delta int16) []byte {
	coverage := buildCoverageFormat1(coverageGlyphs)

	// SingleSubstFormat1: format(2) + coverageOffset(2) + deltaGlyphID(2)
	subtable := make([]byte, 6+len(coverage))
	binary.BigEndian.PutUint16(subtable[0:], 1) // format
	binary.BigEndian.PutUint16(subtable[2:], 6) // coverage offset (right after header)
	binary.BigEndian.PutUint16(subtable[4:], uint16(delta))
	copy(subtable[6:], coverage)
	return subtable
}

// Helper to build a SingleSubst Format 2 subtable
func buildSingleSubstFormat2(coverageGlyphs []GlyphID, substitutes []GlyphID) []byte {
	coverage := buildCoverageFormat1(coverageGlyphs)

	// SingleSubstFormat2: format(2) + coverageOffset(2) + glyphCount(2) + substituteGlyphIDs
	headerSize := 6 + len(substitutes)*2
	subtable := make([]byte, headerSize+len(coverage))
	binary.BigEndian.PutUint16(subtable[0:], 2)                  // format
	binary.BigEndian.PutUint16(subtable[2:], uint16(headerSize)) // coverage offset
	binary.BigEndian.PutUint16(subtable[4:], uint16(len(substitutes)))
	for i, s := range substitutes {
		binary.BigEndian.PutUint16(subtable[6+i*2:], uint16(s))
	}
	copy(subtable[headerSize:], coverage)
	return subtable
}

// Helper to build a Ligature subtable
func buildLigature(ligGlyph GlyphID, components []GlyphID) []byte {
	data := make([]byte, 4+len(components)*2)
	binary.BigEndian.PutUint16(data[0:], uint16(ligGlyph))
	binary.BigEndian.PutUint16(data[2:], uint16(len(components)+1)) // +1 for first glyph
	for i, c := range components {
		binary.BigEndian.PutUint16(data[4+i*2:], uint16(c))
	}
	return data
}

// Helper to build a LigatureSet
func buildLigatureSet(ligatures [][]byte) []byte {
	// LigatureSet: ligatureCount(2) + ligatureOffsets + ligatures
	headerSize := 2 + len(ligatures)*2
	totalSize := headerSize
	for _, lig := range ligatures {
		totalSize += len(lig)
	}

	data := make([]byte, totalSize)
	binary.BigEndian.PutUint16(data[0:], uint16(len(ligatures)))

	offset := headerSize
	for i, lig := range ligatures {
		binary.BigEndian.PutUint16(data[2+i*2:], uint16(offset))
		copy(data[offset:], lig)
		offset += len(lig)
	}
	return data
}

// Helper to build a LigatureSubst subtable
func buildLigatureSubst(coverageGlyphs []GlyphID, ligatureSets [][]byte) []byte {
	coverage := buildCoverageFormat1(coverageGlyphs)

	// LigatureSubstFormat1: format(2) + coverageOffset(2) + ligSetCount(2) + ligSetOffsets + ligSets + coverage
	headerSize := 6 + len(ligatureSets)*2
	totalSize := headerSize
	for _, ls := range ligatureSets {
		totalSize += len(ls)
	}
	totalSize += len(coverage)

	data := make([]byte, totalSize)
	binary.BigEndian.PutUint16(data[0:], 1) // format
	binary.BigEndian.PutUint16(data[4:], uint16(len(ligatureSets)))

	offset := headerSize
	for i, ls := range ligatureSets {
		binary.BigEndian.PutUint16(data[6+i*2:], uint16(offset))
		copy(data[offset:], ls)
		offset += len(ls)
	}

	// Coverage offset
	binary.BigEndian.PutUint16(data[2:], uint16(offset))
	copy(data[offset:], coverage)

	return data
}

func TestCoverageFormat1(t *testing.T) {
	glyphs := []GlyphID{10, 20, 30, 40, 50}
	data := buildCoverageFormat1(glyphs)

	cov, err := ParseCoverage(data, 0)
	if err != nil {
		t.Fatalf("ParseCoverage failed: %v", err)
	}

	// Test covered glyphs
	for i, g := range glyphs {
		idx := cov.GetCoverage(g)
		if idx != uint32(i) {
			t.Errorf("GetCoverage(%d) = %d, want %d", g, idx, i)
		}
	}

	// Test not covered
	for _, g := range []GlyphID{0, 5, 15, 25, 100} {
		idx := cov.GetCoverage(g)
		if idx != NotCovered {
			t.Errorf("GetCoverage(%d) = %d, want NotCovered", g, idx)
		}
	}
}

func TestCoverageFormat2(t *testing.T) {
	// Ranges: [10-15] = indices 0-5, [20-25] = indices 6-11
	ranges := [][3]uint16{
		{10, 15, 0},
		{20, 25, 6},
	}
	data := buildCoverageFormat2(ranges)

	cov, err := ParseCoverage(data, 0)
	if err != nil {
		t.Fatalf("ParseCoverage failed: %v", err)
	}

	// Test first range
	for g := GlyphID(10); g <= 15; g++ {
		idx := cov.GetCoverage(g)
		want := uint32(g - 10)
		if idx != want {
			t.Errorf("GetCoverage(%d) = %d, want %d", g, idx, want)
		}
	}

	// Test second range
	for g := GlyphID(20); g <= 25; g++ {
		idx := cov.GetCoverage(g)
		want := uint32(6 + g - 20)
		if idx != want {
			t.Errorf("GetCoverage(%d) = %d, want %d", g, idx, want)
		}
	}

	// Test not covered
	for _, g := range []GlyphID{0, 9, 16, 19, 26, 100} {
		idx := cov.GetCoverage(g)
		if idx != NotCovered {
			t.Errorf("GetCoverage(%d) = %d, want NotCovered", g, idx)
		}
	}
}

func TestSingleSubstFormat1(t *testing.T) {
	// A, B, C (65, 66, 67) -> 100 added = 165, 166, 167
	coverageGlyphs := []GlyphID{65, 66, 67}
	delta := int16(100)
	data := buildSingleSubstFormat1(coverageGlyphs, delta)

	subst, err := parseSingleSubst(data, 0)
	if err != nil {
		t.Fatalf("parseSingleSubst failed: %v", err)
	}

	tests := []struct {
		input  GlyphID
		want   GlyphID
		wantOK bool
	}{
		{65, 165, true},
		{66, 166, true},
		{67, 167, true},
		{68, 0, false},
		{0, 0, false},
	}

	for _, tt := range tests {
		ctx := &GSUBContext{
			Glyphs: []GlyphID{tt.input},
			Index:  0,
		}

		result := subst.Apply(ctx)
		if tt.wantOK {
			if result == 0 {
				t.Errorf("SingleSubst.Apply(%d) returned 0, want 1", tt.input)
				continue
			}
			if ctx.Glyphs[0] != tt.want {
				t.Errorf("SingleSubst.Apply(%d) = %d, want %d", tt.input, ctx.Glyphs[0], tt.want)
			}
		} else {
			if result != 0 {
				t.Errorf("SingleSubst.Apply(%d) returned %d, want 0", tt.input, result)
			}
		}
	}
}

func TestSingleSubstFormat2(t *testing.T) {
	// A->X, B->Y, C->Z
	coverageGlyphs := []GlyphID{65, 66, 67}
	substitutes := []GlyphID{88, 89, 90} // X, Y, Z
	data := buildSingleSubstFormat2(coverageGlyphs, substitutes)

	subst, err := parseSingleSubst(data, 0)
	if err != nil {
		t.Fatalf("parseSingleSubst failed: %v", err)
	}

	tests := []struct {
		input  GlyphID
		want   GlyphID
		wantOK bool
	}{
		{65, 88, true},
		{66, 89, true},
		{67, 90, true},
		{68, 0, false},
	}

	for _, tt := range tests {
		ctx := &GSUBContext{
			Glyphs: []GlyphID{tt.input},
			Index:  0,
		}

		result := subst.Apply(ctx)
		if tt.wantOK {
			if result == 0 {
				t.Errorf("SingleSubst.Apply(%d) returned 0, want 1", tt.input)
				continue
			}
			if ctx.Glyphs[0] != tt.want {
				t.Errorf("SingleSubst.Apply(%d) = %d, want %d", tt.input, ctx.Glyphs[0], tt.want)
			}
		} else {
			if result != 0 {
				t.Errorf("SingleSubst.Apply(%d) returned %d, want 0", tt.input, result)
			}
		}
	}
}

func TestLigatureSubst(t *testing.T) {
	// f + i -> fi (ligature)
	// f + l -> fl (ligature)
	// First glyph: f (102)
	// Components: i (105), l (108)
	// Ligatures: fi (200), fl (201)

	lig1 := buildLigature(200, []GlyphID{105}) // fi
	lig2 := buildLigature(201, []GlyphID{108}) // fl
	ligSet := buildLigatureSet([][]byte{lig1, lig2})

	coverageGlyphs := []GlyphID{102} // f
	data := buildLigatureSubst(coverageGlyphs, [][]byte{ligSet})

	subst, err := parseLigatureSubst(data, 0)
	if err != nil {
		t.Fatalf("parseLigatureSubst failed: %v", err)
	}

	tests := []struct {
		input []GlyphID
		want  []GlyphID
		desc  string
	}{
		{[]GlyphID{102, 105}, []GlyphID{200}, "f+i -> fi"},
		{[]GlyphID{102, 108}, []GlyphID{201}, "f+l -> fl"},
		{[]GlyphID{102, 120}, []GlyphID{102, 120}, "f+x -> f+x (no match)"},
		{[]GlyphID{100, 105}, []GlyphID{100, 105}, "other+i -> no change"},
	}

	for _, tt := range tests {
		ctx := &GSUBContext{
			Glyphs: append([]GlyphID{}, tt.input...),
			Index:  0,
		}

		subst.Apply(ctx)

		if len(ctx.Glyphs) != len(tt.want) {
			t.Errorf("%s: got %d glyphs, want %d", tt.desc, len(ctx.Glyphs), len(tt.want))
			continue
		}

		for i := range tt.want {
			if ctx.Glyphs[i] != tt.want[i] {
				t.Errorf("%s: glyph[%d] = %d, want %d", tt.desc, i, ctx.Glyphs[i], tt.want[i])
			}
		}
	}
}

func TestLigatureSubstMultiple(t *testing.T) {
	// ffi -> ffi_lig (3-glyph ligature)
	// f (102), f (102), i (105) -> ffi (202)

	lig1 := buildLigature(202, []GlyphID{102, 105}) // f + f + i = ffi
	ligSet := buildLigatureSet([][]byte{lig1})

	coverageGlyphs := []GlyphID{102} // f
	data := buildLigatureSubst(coverageGlyphs, [][]byte{ligSet})

	subst, err := parseLigatureSubst(data, 0)
	if err != nil {
		t.Fatalf("parseLigatureSubst failed: %v", err)
	}

	ctx := &GSUBContext{
		Glyphs: []GlyphID{102, 102, 105}, // f f i
		Index:  0,
	}

	result := subst.Apply(ctx)
	if result == 0 {
		t.Fatal("LigatureSubst.Apply returned 0, want 1")
	}

	if len(ctx.Glyphs) != 1 {
		t.Fatalf("got %d glyphs, want 1", len(ctx.Glyphs))
	}
	if ctx.Glyphs[0] != 202 {
		t.Errorf("got glyph %d, want 202", ctx.Glyphs[0])
	}
}

func TestGSUBContextReplaceGlyphs(t *testing.T) {
	ctx := &GSUBContext{
		Glyphs: []GlyphID{1, 2, 3, 4},
		Index:  1,
	}

	// Replace glyph at index 1 with [10, 11, 12]
	ctx.ReplaceGlyphs([]GlyphID{10, 11, 12})

	want := []GlyphID{1, 10, 11, 12, 3, 4}
	if len(ctx.Glyphs) != len(want) {
		t.Fatalf("got %d glyphs, want %d", len(ctx.Glyphs), len(want))
	}
	for i := range want {
		if ctx.Glyphs[i] != want[i] {
			t.Errorf("glyph[%d] = %d, want %d", i, ctx.Glyphs[i], want[i])
		}
	}
	if ctx.Index != 4 { // Should advance past all new glyphs
		t.Errorf("Index = %d, want 4", ctx.Index)
	}
}

func TestGSUBContextLigate(t *testing.T) {
	ctx := &GSUBContext{
		Glyphs: []GlyphID{1, 2, 3, 4, 5},
		Index:  1,
	}

	// Ligate 3 glyphs (2, 3, 4) at index 1 into glyph 100
	ctx.Ligate(100, 3)

	want := []GlyphID{1, 100, 5}
	if len(ctx.Glyphs) != len(want) {
		t.Fatalf("got %d glyphs, want %d", len(ctx.Glyphs), len(want))
	}
	for i := range want {
		if ctx.Glyphs[i] != want[i] {
			t.Errorf("glyph[%d] = %d, want %d", i, ctx.Glyphs[i], want[i])
		}
	}
	if ctx.Index != 2 {
		t.Errorf("Index = %d, want 2", ctx.Index)
	}
}

// Build a minimal GSUB table for testing
func buildGSUBTable(lookups [][]byte) []byte {
	// GSUB header: version(4) + scriptListOffset(2) + featureListOffset(2) + lookupListOffset(2)
	headerSize := 10

	// Empty ScriptList: count(2)
	scriptListSize := 2

	// Empty FeatureList: count(2)
	featureListSize := 2

	// LookupList: count(2) + offsets + lookups
	lookupListHeaderSize := 2 + len(lookups)*2
	lookupListSize := lookupListHeaderSize
	for _, l := range lookups {
		lookupListSize += len(l)
	}

	totalSize := headerSize + scriptListSize + featureListSize + lookupListSize
	data := make([]byte, totalSize)

	// Header
	binary.BigEndian.PutUint16(data[0:], 1)                                                 // version major
	binary.BigEndian.PutUint16(data[2:], 0)                                                 // version minor
	binary.BigEndian.PutUint16(data[4:], uint16(headerSize))                                // scriptList offset
	binary.BigEndian.PutUint16(data[6:], uint16(headerSize+scriptListSize))                 // featureList offset
	binary.BigEndian.PutUint16(data[8:], uint16(headerSize+scriptListSize+featureListSize)) // lookupList offset

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

// Build a GSUB lookup wrapper
func buildGSUBLookup(lookupType uint16, subtables [][]byte) []byte {
	// Lookup: type(2) + flag(2) + subtableCount(2) + offsets + subtables
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

func TestParseGSUB(t *testing.T) {
	// Build a simple GSUB with one SingleSubst lookup
	subtable := buildSingleSubstFormat1([]GlyphID{65, 66}, 10)
	lookup := buildGSUBLookup(GSUBTypeSingle, [][]byte{subtable})
	gsubData := buildGSUBTable([][]byte{lookup})

	gsub, err := ParseGSUB(gsubData)
	if err != nil {
		t.Fatalf("ParseGSUB failed: %v", err)
	}

	if gsub.NumLookups() != 1 {
		t.Errorf("NumLookups = %d, want 1", gsub.NumLookups())
	}

	// Apply lookup
	glyphs := []GlyphID{65, 66, 67}
	result := gsub.ApplyLookup(0, glyphs)

	want := []GlyphID{75, 76, 67} // 65+10, 66+10, 67 unchanged
	if len(result) != len(want) {
		t.Fatalf("got %d glyphs, want %d", len(result), len(want))
	}
	for i := range want {
		if result[i] != want[i] {
			t.Errorf("result[%d] = %d, want %d", i, result[i], want[i])
		}
	}
}

func TestGSUBApplyLigature(t *testing.T) {
	// f+i -> fi ligature
	lig := buildLigature(200, []GlyphID{105})
	ligSet := buildLigatureSet([][]byte{lig})
	subtable := buildLigatureSubst([]GlyphID{102}, [][]byte{ligSet})
	lookup := buildGSUBLookup(GSUBTypeLigature, [][]byte{subtable})
	gsubData := buildGSUBTable([][]byte{lookup})

	gsub, err := ParseGSUB(gsubData)
	if err != nil {
		t.Fatalf("ParseGSUB failed: %v", err)
	}

	glyphs := []GlyphID{102, 105, 102, 105} // f i f i
	result := gsub.ApplyLookup(0, glyphs)

	want := []GlyphID{200, 200} // fi fi
	if len(result) != len(want) {
		t.Fatalf("got %d glyphs (%v), want %d (%v)", len(result), result, len(want), want)
	}
	for i := range want {
		if result[i] != want[i] {
			t.Errorf("result[%d] = %d, want %d", i, result[i], want[i])
		}
	}
}

// --- ChainContextSubst tests ---

// buildChainRule builds a ChainRule for Format 1/2.
// backtrack, input (without first glyph), lookahead, lookupRecords
func buildChainRule(backtrack []GlyphID, input []GlyphID, lookahead []GlyphID, lookups []LookupRecord) []byte {
	// ChainRule: backtrackCount + backtrack[] + inputCount + input[] +
	//            lookaheadCount + lookahead[] + lookupCount + lookupRecords[]
	size := 2 + len(backtrack)*2 + 2 + len(input)*2 + 2 + len(lookahead)*2 + 2 + len(lookups)*4
	data := make([]byte, size)
	off := 0

	// Backtrack
	binary.BigEndian.PutUint16(data[off:], uint16(len(backtrack)))
	off += 2
	for _, g := range backtrack {
		binary.BigEndian.PutUint16(data[off:], uint16(g))
		off += 2
	}

	// Input (count includes first glyph, but array doesn't contain it)
	binary.BigEndian.PutUint16(data[off:], uint16(len(input)+1))
	off += 2
	for _, g := range input {
		binary.BigEndian.PutUint16(data[off:], uint16(g))
		off += 2
	}

	// Lookahead
	binary.BigEndian.PutUint16(data[off:], uint16(len(lookahead)))
	off += 2
	for _, g := range lookahead {
		binary.BigEndian.PutUint16(data[off:], uint16(g))
		off += 2
	}

	// Lookup records
	binary.BigEndian.PutUint16(data[off:], uint16(len(lookups)))
	off += 2
	for _, lr := range lookups {
		binary.BigEndian.PutUint16(data[off:], lr.SequenceIndex)
		binary.BigEndian.PutUint16(data[off+2:], lr.LookupIndex)
		off += 4
	}

	return data
}

// buildChainRuleSet builds a ChainRuleSet from multiple ChainRules.
func buildChainRuleSet(rules [][]byte) []byte {
	headerSize := 2 + len(rules)*2
	totalSize := headerSize
	for _, r := range rules {
		totalSize += len(r)
	}

	data := make([]byte, totalSize)
	binary.BigEndian.PutUint16(data[0:], uint16(len(rules)))

	offset := headerSize
	for i, r := range rules {
		binary.BigEndian.PutUint16(data[2+i*2:], uint16(offset))
		copy(data[offset:], r)
		offset += len(r)
	}
	return data
}

// buildChainContextFormat1 builds a ChainContextSubstFormat1 subtable.
func buildChainContextFormat1(coverageGlyphs []GlyphID, ruleSets [][]byte) []byte {
	coverage := buildCoverageFormat1(coverageGlyphs)

	// Format1: format(2) + coverageOffset(2) + ruleSetCount(2) + ruleSetOffsets + ruleSets + coverage
	headerSize := 6 + len(ruleSets)*2
	totalSize := headerSize
	for _, rs := range ruleSets {
		totalSize += len(rs)
	}
	totalSize += len(coverage)

	data := make([]byte, totalSize)
	binary.BigEndian.PutUint16(data[0:], 1) // format
	binary.BigEndian.PutUint16(data[4:], uint16(len(ruleSets)))

	offset := headerSize
	for i, rs := range ruleSets {
		if len(rs) > 0 {
			binary.BigEndian.PutUint16(data[6+i*2:], uint16(offset))
			copy(data[offset:], rs)
			offset += len(rs)
		}
	}

	// Coverage offset
	binary.BigEndian.PutUint16(data[2:], uint16(offset))
	copy(data[offset:], coverage)

	return data
}

// buildChainContextFormat3 builds a ChainContextSubstFormat3 subtable.
func buildChainContextFormat3(backtrackCovs, inputCovs, lookaheadCovs [][]byte, lookups []LookupRecord) []byte {
	// Calculate offsets
	headerSize := 2 + // format
		2 + len(backtrackCovs)*2 + // backtrackCount + offsets
		2 + len(inputCovs)*2 + // inputCount + offsets
		2 + len(lookaheadCovs)*2 + // lookaheadCount + offsets
		2 + len(lookups)*4 // lookupCount + records

	totalSize := headerSize
	for _, c := range backtrackCovs {
		totalSize += len(c)
	}
	for _, c := range inputCovs {
		totalSize += len(c)
	}
	for _, c := range lookaheadCovs {
		totalSize += len(c)
	}

	data := make([]byte, totalSize)
	off := 0

	// Format
	binary.BigEndian.PutUint16(data[off:], 3)
	off += 2

	// Calculate coverage data start
	covDataOff := headerSize

	// Backtrack coverages
	binary.BigEndian.PutUint16(data[off:], uint16(len(backtrackCovs)))
	off += 2
	for _, c := range backtrackCovs {
		binary.BigEndian.PutUint16(data[off:], uint16(covDataOff))
		off += 2
		copy(data[covDataOff:], c)
		covDataOff += len(c)
	}

	// Input coverages
	binary.BigEndian.PutUint16(data[off:], uint16(len(inputCovs)))
	off += 2
	for _, c := range inputCovs {
		binary.BigEndian.PutUint16(data[off:], uint16(covDataOff))
		off += 2
		copy(data[covDataOff:], c)
		covDataOff += len(c)
	}

	// Lookahead coverages
	binary.BigEndian.PutUint16(data[off:], uint16(len(lookaheadCovs)))
	off += 2
	for _, c := range lookaheadCovs {
		binary.BigEndian.PutUint16(data[off:], uint16(covDataOff))
		off += 2
		copy(data[covDataOff:], c)
		covDataOff += len(c)
	}

	// Lookup records
	binary.BigEndian.PutUint16(data[off:], uint16(len(lookups)))
	off += 2
	for _, lr := range lookups {
		binary.BigEndian.PutUint16(data[off:], lr.SequenceIndex)
		binary.BigEndian.PutUint16(data[off+2:], lr.LookupIndex)
		off += 4
	}

	return data
}

// --- Context Substitution tests ---

// buildContextRule builds a Rule for ContextSubst Format 1/2.
func buildContextRule(input []GlyphID, lookups []LookupRecord) []byte {
	// Rule: inputCount(2) + lookupCount(2) + input[] + lookupRecords[]
	size := 4 + len(input)*2 + len(lookups)*4
	data := make([]byte, size)

	// inputCount includes first glyph
	binary.BigEndian.PutUint16(data[0:], uint16(len(input)+1))
	binary.BigEndian.PutUint16(data[2:], uint16(len(lookups)))

	off := 4
	for _, g := range input {
		binary.BigEndian.PutUint16(data[off:], uint16(g))
		off += 2
	}

	for _, lr := range lookups {
		binary.BigEndian.PutUint16(data[off:], lr.SequenceIndex)
		binary.BigEndian.PutUint16(data[off+2:], lr.LookupIndex)
		off += 4
	}

	return data
}

// buildContextRuleSet builds a RuleSet from multiple Rules.
func buildContextRuleSet(rules [][]byte) []byte {
	headerSize := 2 + len(rules)*2
	totalSize := headerSize
	for _, r := range rules {
		totalSize += len(r)
	}

	data := make([]byte, totalSize)
	binary.BigEndian.PutUint16(data[0:], uint16(len(rules)))

	offset := headerSize
	for i, r := range rules {
		binary.BigEndian.PutUint16(data[2+i*2:], uint16(offset))
		copy(data[offset:], r)
		offset += len(r)
	}
	return data
}

// buildContextFormat1 builds a ContextSubstFormat1 subtable.
func buildContextFormat1(coverageGlyphs []GlyphID, ruleSets [][]byte) []byte {
	coverage := buildCoverageFormat1(coverageGlyphs)

	headerSize := 6 + len(ruleSets)*2
	totalSize := headerSize
	for _, rs := range ruleSets {
		totalSize += len(rs)
	}
	totalSize += len(coverage)

	data := make([]byte, totalSize)
	binary.BigEndian.PutUint16(data[0:], 1) // format
	binary.BigEndian.PutUint16(data[4:], uint16(len(ruleSets)))

	offset := headerSize
	for i, rs := range ruleSets {
		if len(rs) > 0 {
			binary.BigEndian.PutUint16(data[6+i*2:], uint16(offset))
			copy(data[offset:], rs)
			offset += len(rs)
		}
	}

	binary.BigEndian.PutUint16(data[2:], uint16(offset))
	copy(data[offset:], coverage)

	return data
}

// buildContextFormat3 builds a ContextSubstFormat3 subtable.
func buildContextFormat3(inputCovs [][]byte, lookups []LookupRecord) []byte {
	headerSize := 6 + len(inputCovs)*2 + len(lookups)*4

	totalSize := headerSize
	for _, c := range inputCovs {
		totalSize += len(c)
	}

	data := make([]byte, totalSize)
	binary.BigEndian.PutUint16(data[0:], 3) // format
	binary.BigEndian.PutUint16(data[2:], uint16(len(inputCovs)))
	binary.BigEndian.PutUint16(data[4:], uint16(len(lookups)))

	covDataOff := headerSize
	off := 6
	for _, c := range inputCovs {
		binary.BigEndian.PutUint16(data[off:], uint16(covDataOff))
		off += 2
		copy(data[covDataOff:], c)
		covDataOff += len(c)
	}

	for _, lr := range lookups {
		binary.BigEndian.PutUint16(data[off:], lr.SequenceIndex)
		binary.BigEndian.PutUint16(data[off+2:], lr.LookupIndex)
		off += 4
	}

	return data
}

func TestContextSubstFormat1Basic(t *testing.T) {
	// Test: If 'a'+'b' sequence found, substitute 'a' with 'A'

	// Lookup 0: SingleSubst a(65) -> A(97)
	singleSubst := buildSingleSubstFormat2([]GlyphID{65}, []GlyphID{97})
	lookup0 := buildGSUBLookup(GSUBTypeSingle, [][]byte{singleSubst})

	// Lookup 1: ContextSubst Format 1
	// Rule: input='a'+'b', apply lookup 0 at position 0
	rule := buildContextRule(
		[]GlyphID{66}, // additional input: 'b'
		[]LookupRecord{{SequenceIndex: 0, LookupIndex: 0}},
	)
	ruleSet := buildContextRuleSet([][]byte{rule})
	contextSubst := buildContextFormat1([]GlyphID{65}, [][]byte{ruleSet})
	lookup1 := buildGSUBLookup(GSUBTypeContext, [][]byte{contextSubst})

	gsubData := buildGSUBTable([][]byte{lookup0, lookup1})
	gsub, err := ParseGSUB(gsubData)
	if err != nil {
		t.Fatalf("ParseGSUB failed: %v", err)
	}

	tests := []struct {
		input []GlyphID
		want  []GlyphID
		desc  string
	}{
		{[]GlyphID{65, 66}, []GlyphID{97, 66}, "'a'+'b' -> 'A'+'b'"},
		{[]GlyphID{65, 67}, []GlyphID{65, 67}, "'a'+'c' -> no change"},
		{[]GlyphID{65}, []GlyphID{65}, "'a' alone -> no change"},
		{[]GlyphID{65, 66, 65, 66}, []GlyphID{97, 66, 97, 66}, "'a'+'b'+'a'+'b' -> 'A'+'b'+'A'+'b'"},
	}

	for _, tt := range tests {
		glyphs := append([]GlyphID{}, tt.input...)
		result := gsub.ApplyLookup(1, glyphs)

		if len(result) != len(tt.want) {
			t.Errorf("%s: got %d glyphs (%v), want %d (%v)", tt.desc, len(result), result, len(tt.want), tt.want)
			continue
		}
		for i := range tt.want {
			if result[i] != tt.want[i] {
				t.Errorf("%s: result[%d] = %d, want %d", tt.desc, i, result[i], tt.want[i])
			}
		}
	}
}

func TestContextSubstFormat3Basic(t *testing.T) {
	// Test Format 3: Coverage-based
	// If 'a'+'b' sequence found, substitute 'b' with 'B'

	// Lookup 0: SingleSubst b(66) -> B(98)
	singleSubst := buildSingleSubstFormat2([]GlyphID{66}, []GlyphID{98})
	lookup0 := buildGSUBLookup(GSUBTypeSingle, [][]byte{singleSubst})

	// Lookup 1: ContextSubst Format 3
	inputCov1 := buildCoverageFormat1([]GlyphID{65}) // 'a'
	inputCov2 := buildCoverageFormat1([]GlyphID{66}) // 'b'

	contextSubst := buildContextFormat3(
		[][]byte{inputCov1, inputCov2},
		[]LookupRecord{{SequenceIndex: 1, LookupIndex: 0}}, // apply at position 1 (the 'b')
	)
	lookup1 := buildGSUBLookup(GSUBTypeContext, [][]byte{contextSubst})

	gsubData := buildGSUBTable([][]byte{lookup0, lookup1})
	gsub, err := ParseGSUB(gsubData)
	if err != nil {
		t.Fatalf("ParseGSUB failed: %v", err)
	}

	tests := []struct {
		input []GlyphID
		want  []GlyphID
		desc  string
	}{
		{[]GlyphID{65, 66}, []GlyphID{65, 98}, "'a'+'b' -> 'a'+'B'"},
		{[]GlyphID{65, 67}, []GlyphID{65, 67}, "'a'+'c' -> no change"},
		{[]GlyphID{66, 66}, []GlyphID{66, 66}, "'b'+'b' -> no change"},
	}

	for _, tt := range tests {
		glyphs := append([]GlyphID{}, tt.input...)
		result := gsub.ApplyLookup(1, glyphs)

		if len(result) != len(tt.want) {
			t.Errorf("%s: got %d glyphs (%v), want %d (%v)", tt.desc, len(result), result, len(tt.want), tt.want)
			continue
		}
		for i := range tt.want {
			if result[i] != tt.want[i] {
				t.Errorf("%s: result[%d] = %d, want %d", tt.desc, i, result[i], tt.want[i])
			}
		}
	}
}

func TestChainContextSubstFormat1Basic(t *testing.T) {
	// Test: If 'a' (65) is followed by 'b' (66), substitute 'a' with 'A' (97)
	// We need two lookups:
	// Lookup 0: SingleSubst that replaces 'a' -> 'A'
	// Lookup 1: ChainContextSubst that triggers lookup 0 when 'a' is followed by 'b'

	// Lookup 0: SingleSubst a(65) -> A(97)
	singleSubst := buildSingleSubstFormat2([]GlyphID{65}, []GlyphID{97})
	lookup0 := buildGSUBLookup(GSUBTypeSingle, [][]byte{singleSubst})

	// Lookup 1: ChainContextSubst Format 1
	// Rule: no backtrack, input='a', lookahead='b', apply lookup 0 at position 0
	rule := buildChainRule(
		[]GlyphID{},   // no backtrack
		[]GlyphID{},   // no additional input (just the first glyph from coverage)
		[]GlyphID{66}, // lookahead: 'b'
		[]LookupRecord{{SequenceIndex: 0, LookupIndex: 0}},
	)
	ruleSet := buildChainRuleSet([][]byte{rule})
	chainContext := buildChainContextFormat1([]GlyphID{65}, [][]byte{ruleSet})
	lookup1 := buildGSUBLookup(GSUBTypeChainContext, [][]byte{chainContext})

	gsubData := buildGSUBTable([][]byte{lookup0, lookup1})
	gsub, err := ParseGSUB(gsubData)
	if err != nil {
		t.Fatalf("ParseGSUB failed: %v", err)
	}

	tests := []struct {
		input []GlyphID
		want  []GlyphID
		desc  string
	}{
		{[]GlyphID{65, 66}, []GlyphID{97, 66}, "'a' followed by 'b' -> 'A'+'b'"},
		{[]GlyphID{65, 67}, []GlyphID{65, 67}, "'a' followed by 'c' -> no change"},
		{[]GlyphID{65}, []GlyphID{65}, "'a' alone -> no change (no lookahead)"},
		{[]GlyphID{66, 65, 66}, []GlyphID{66, 97, 66}, "'b'+'a'+'b' -> 'b'+'A'+'b'"},
	}

	for _, tt := range tests {
		glyphs := append([]GlyphID{}, tt.input...)
		result := gsub.ApplyLookup(1, glyphs)

		if len(result) != len(tt.want) {
			t.Errorf("%s: got %d glyphs (%v), want %d (%v)", tt.desc, len(result), result, len(tt.want), tt.want)
			continue
		}
		for i := range tt.want {
			if result[i] != tt.want[i] {
				t.Errorf("%s: result[%d] = %d, want %d", tt.desc, i, result[i], tt.want[i])
			}
		}
	}
}

func TestChainContextSubstFormat1WithBacktrack(t *testing.T) {
	// Test: If 'a' (65) is preceded by 'x' (120) and followed by 'b' (66),
	// substitute 'a' with 'A' (97)

	// Lookup 0: SingleSubst a(65) -> A(97)
	singleSubst := buildSingleSubstFormat2([]GlyphID{65}, []GlyphID{97})
	lookup0 := buildGSUBLookup(GSUBTypeSingle, [][]byte{singleSubst})

	// Lookup 1: ChainContextSubst Format 1
	// Rule: backtrack='x', input='a', lookahead='b', apply lookup 0 at position 0
	rule := buildChainRule(
		[]GlyphID{120}, // backtrack: 'x'
		[]GlyphID{},    // no additional input
		[]GlyphID{66},  // lookahead: 'b'
		[]LookupRecord{{SequenceIndex: 0, LookupIndex: 0}},
	)
	ruleSet := buildChainRuleSet([][]byte{rule})
	chainContext := buildChainContextFormat1([]GlyphID{65}, [][]byte{ruleSet})
	lookup1 := buildGSUBLookup(GSUBTypeChainContext, [][]byte{chainContext})

	gsubData := buildGSUBTable([][]byte{lookup0, lookup1})
	gsub, err := ParseGSUB(gsubData)
	if err != nil {
		t.Fatalf("ParseGSUB failed: %v", err)
	}

	tests := []struct {
		input []GlyphID
		want  []GlyphID
		desc  string
	}{
		{[]GlyphID{120, 65, 66}, []GlyphID{120, 97, 66}, "'x'+'a'+'b' -> 'x'+'A'+'b'"},
		{[]GlyphID{121, 65, 66}, []GlyphID{121, 65, 66}, "'y'+'a'+'b' -> no change (wrong backtrack)"},
		{[]GlyphID{65, 66}, []GlyphID{65, 66}, "'a'+'b' -> no change (no backtrack)"},
		{[]GlyphID{120, 65, 67}, []GlyphID{120, 65, 67}, "'x'+'a'+'c' -> no change (wrong lookahead)"},
	}

	for _, tt := range tests {
		glyphs := append([]GlyphID{}, tt.input...)
		result := gsub.ApplyLookup(1, glyphs)

		if len(result) != len(tt.want) {
			t.Errorf("%s: got %d glyphs (%v), want %d (%v)", tt.desc, len(result), result, len(tt.want), tt.want)
			continue
		}
		for i := range tt.want {
			if result[i] != tt.want[i] {
				t.Errorf("%s: result[%d] = %d, want %d", tt.desc, i, result[i], tt.want[i])
			}
		}
	}
}

func TestChainContextSubstFormat3Basic(t *testing.T) {
	// Test Format 3: Coverage-based context
	// If glyph covered by inputCov is preceded by glyph in backtrackCov
	// and followed by glyph in lookaheadCov, apply substitution

	// Lookup 0: SingleSubst a(65) -> A(97)
	singleSubst := buildSingleSubstFormat2([]GlyphID{65}, []GlyphID{97})
	lookup0 := buildGSUBLookup(GSUBTypeSingle, [][]byte{singleSubst})

	// Lookup 1: ChainContextSubst Format 3
	backtrackCov := buildCoverageFormat1([]GlyphID{120}) // 'x'
	inputCov := buildCoverageFormat1([]GlyphID{65})      // 'a'
	lookaheadCov := buildCoverageFormat1([]GlyphID{66})  // 'b'

	chainContext := buildChainContextFormat3(
		[][]byte{backtrackCov},
		[][]byte{inputCov},
		[][]byte{lookaheadCov},
		[]LookupRecord{{SequenceIndex: 0, LookupIndex: 0}},
	)
	lookup1 := buildGSUBLookup(GSUBTypeChainContext, [][]byte{chainContext})

	gsubData := buildGSUBTable([][]byte{lookup0, lookup1})
	gsub, err := ParseGSUB(gsubData)
	if err != nil {
		t.Fatalf("ParseGSUB failed: %v", err)
	}

	tests := []struct {
		input []GlyphID
		want  []GlyphID
		desc  string
	}{
		{[]GlyphID{120, 65, 66}, []GlyphID{120, 97, 66}, "'x'+'a'+'b' -> 'x'+'A'+'b'"},
		{[]GlyphID{121, 65, 66}, []GlyphID{121, 65, 66}, "'y'+'a'+'b' -> no change"},
		{[]GlyphID{120, 65, 67}, []GlyphID{120, 65, 67}, "'x'+'a'+'c' -> no change"},
	}

	for _, tt := range tests {
		glyphs := append([]GlyphID{}, tt.input...)
		result := gsub.ApplyLookup(1, glyphs)

		if len(result) != len(tt.want) {
			t.Errorf("%s: got %d glyphs (%v), want %d (%v)", tt.desc, len(result), result, len(tt.want), tt.want)
			continue
		}
		for i := range tt.want {
			if result[i] != tt.want[i] {
				t.Errorf("%s: result[%d] = %d, want %d", tt.desc, i, result[i], tt.want[i])
			}
		}
	}
}

func TestChainContextSubstMultipleInputGlyphs(t *testing.T) {
	// Test with multiple input glyphs
	// If 'a'+'b' sequence is found, substitute 'b' with 'B'

	// Lookup 0: SingleSubst b(66) -> B(98)
	singleSubst := buildSingleSubstFormat2([]GlyphID{66}, []GlyphID{98})
	lookup0 := buildGSUBLookup(GSUBTypeSingle, [][]byte{singleSubst})

	// Lookup 1: ChainContextSubst Format 1
	// Rule: no backtrack, input='a'+'b', no lookahead, apply lookup 0 at position 1
	rule := buildChainRule(
		[]GlyphID{},   // no backtrack
		[]GlyphID{66}, // additional input: 'b' (first glyph 'a' is from coverage)
		[]GlyphID{},   // no lookahead
		[]LookupRecord{{SequenceIndex: 1, LookupIndex: 0}}, // apply at position 1 (the 'b')
	)
	ruleSet := buildChainRuleSet([][]byte{rule})
	chainContext := buildChainContextFormat1([]GlyphID{65}, [][]byte{ruleSet})
	lookup1 := buildGSUBLookup(GSUBTypeChainContext, [][]byte{chainContext})

	gsubData := buildGSUBTable([][]byte{lookup0, lookup1})
	gsub, err := ParseGSUB(gsubData)
	if err != nil {
		t.Fatalf("ParseGSUB failed: %v", err)
	}

	tests := []struct {
		input []GlyphID
		want  []GlyphID
		desc  string
	}{
		{[]GlyphID{65, 66}, []GlyphID{65, 98}, "'a'+'b' -> 'a'+'B'"},
		{[]GlyphID{65, 67}, []GlyphID{65, 67}, "'a'+'c' -> no change"},
		{[]GlyphID{65, 66, 65, 66}, []GlyphID{65, 98, 65, 98}, "'a'+'b'+'a'+'b' -> 'a'+'B'+'a'+'B'"},
	}

	for _, tt := range tests {
		glyphs := append([]GlyphID{}, tt.input...)
		result := gsub.ApplyLookup(1, glyphs)

		if len(result) != len(tt.want) {
			t.Errorf("%s: got %d glyphs (%v), want %d (%v)", tt.desc, len(result), result, len(tt.want), tt.want)
			continue
		}
		for i := range tt.want {
			if result[i] != tt.want[i] {
				t.Errorf("%s: result[%d] = %d, want %d", tt.desc, i, result[i], tt.want[i])
			}
		}
	}
}

// buildChainContextFormat2 builds a ChainContextSubstFormat2 subtable.
func buildChainContextFormat2(coverageGlyphs []GlyphID, backtrackClassDef, inputClassDef, lookaheadClassDef []byte, ruleSets [][]byte) []byte {
	coverage := buildCoverageFormat1(coverageGlyphs)

	// Format2: format(2) + coverageOffset(2) + backtrackClassDefOffset(2) + inputClassDefOffset(2) +
	//          lookaheadClassDefOffset(2) + ruleSetCount(2) + ruleSetOffsets + ruleSets + classDefs + coverage
	headerSize := 12 + len(ruleSets)*2
	totalSize := headerSize
	for _, rs := range ruleSets {
		totalSize += len(rs)
	}
	totalSize += len(backtrackClassDef) + len(inputClassDef) + len(lookaheadClassDef) + len(coverage)

	data := make([]byte, totalSize)
	binary.BigEndian.PutUint16(data[0:], 2) // format
	binary.BigEndian.PutUint16(data[10:], uint16(len(ruleSets)))

	offset := headerSize

	// RuleSets
	for i, rs := range ruleSets {
		if len(rs) > 0 {
			binary.BigEndian.PutUint16(data[12+i*2:], uint16(offset))
			copy(data[offset:], rs)
			offset += len(rs)
		}
	}

	// Backtrack ClassDef
	binary.BigEndian.PutUint16(data[4:], uint16(offset))
	copy(data[offset:], backtrackClassDef)
	offset += len(backtrackClassDef)

	// Input ClassDef
	binary.BigEndian.PutUint16(data[6:], uint16(offset))
	copy(data[offset:], inputClassDef)
	offset += len(inputClassDef)

	// Lookahead ClassDef
	binary.BigEndian.PutUint16(data[8:], uint16(offset))
	copy(data[offset:], lookaheadClassDef)
	offset += len(lookaheadClassDef)

	// Coverage
	binary.BigEndian.PutUint16(data[2:], uint16(offset))
	copy(data[offset:], coverage)

	return data
}

// --- Alternate Substitution tests ---

// buildAlternateSet builds an AlternateSet (array of alternate glyphs).
func buildAlternateSet(alternates []GlyphID) []byte {
	data := make([]byte, 2+len(alternates)*2)
	binary.BigEndian.PutUint16(data[0:], uint16(len(alternates)))
	for i, g := range alternates {
		binary.BigEndian.PutUint16(data[2+i*2:], uint16(g))
	}
	return data
}

// buildAlternateSubst builds an AlternateSubst subtable.
func buildAlternateSubst(coverageGlyphs []GlyphID, alternateSets [][]byte) []byte {
	coverage := buildCoverageFormat1(coverageGlyphs)

	// AlternateSubstFormat1: format(2) + coverageOffset(2) + altSetCount(2) + altSetOffsets + altSets + coverage
	headerSize := 6 + len(alternateSets)*2
	totalSize := headerSize
	for _, as := range alternateSets {
		totalSize += len(as)
	}
	totalSize += len(coverage)

	data := make([]byte, totalSize)
	binary.BigEndian.PutUint16(data[0:], 1) // format
	binary.BigEndian.PutUint16(data[4:], uint16(len(alternateSets)))

	offset := headerSize
	for i, as := range alternateSets {
		binary.BigEndian.PutUint16(data[6+i*2:], uint16(offset))
		copy(data[offset:], as)
		offset += len(as)
	}

	// Coverage offset
	binary.BigEndian.PutUint16(data[2:], uint16(offset))
	copy(data[offset:], coverage)

	return data
}

func TestAlternateSubstBasic(t *testing.T) {
	// Test: glyph 'a' (65) has alternates 'A' (97), 'α' (200), 'ä' (201)
	altSet := buildAlternateSet([]GlyphID{97, 200, 201})
	subtable := buildAlternateSubst([]GlyphID{65}, [][]byte{altSet})
	lookup := buildGSUBLookup(GSUBTypeAlternate, [][]byte{subtable})
	gsubData := buildGSUBTable([][]byte{lookup})

	gsub, err := ParseGSUB(gsubData)
	if err != nil {
		t.Fatalf("ParseGSUB failed: %v", err)
	}

	// Default apply (first alternate)
	glyphs := []GlyphID{65, 66, 65}
	result := gsub.ApplyLookup(0, glyphs)

	want := []GlyphID{97, 66, 97} // 'a' -> first alternate (97)
	if len(result) != len(want) {
		t.Fatalf("got %d glyphs (%v), want %d (%v)", len(result), result, len(want), want)
	}
	for i := range want {
		if result[i] != want[i] {
			t.Errorf("result[%d] = %d, want %d", i, result[i], want[i])
		}
	}
}

func TestAlternateSubstGetAlternates(t *testing.T) {
	// Test GetAlternates method
	altSet1 := buildAlternateSet([]GlyphID{97, 200, 201}) // alternates for 'a'
	altSet2 := buildAlternateSet([]GlyphID{98, 202})      // alternates for 'b'
	subtable := buildAlternateSubst([]GlyphID{65, 66}, [][]byte{altSet1, altSet2})

	subst, err := parseAlternateSubst(subtable, 0)
	if err != nil {
		t.Fatalf("parseAlternateSubst failed: %v", err)
	}

	// Check alternates for 'a' (65)
	alts := subst.GetAlternates(65)
	wantAlts := []GlyphID{97, 200, 201}
	if len(alts) != len(wantAlts) {
		t.Fatalf("GetAlternates(65): got %d alternates, want %d", len(alts), len(wantAlts))
	}
	for i, g := range wantAlts {
		if alts[i] != g {
			t.Errorf("GetAlternates(65)[%d] = %d, want %d", i, alts[i], g)
		}
	}

	// Check alternates for 'b' (66)
	alts = subst.GetAlternates(66)
	wantAlts = []GlyphID{98, 202}
	if len(alts) != len(wantAlts) {
		t.Fatalf("GetAlternates(66): got %d alternates, want %d", len(alts), len(wantAlts))
	}

	// Check uncovered glyph
	alts = subst.GetAlternates(67)
	if alts != nil {
		t.Errorf("GetAlternates(67): got %v, want nil", alts)
	}
}

func TestAlternateSubstWithIndex(t *testing.T) {
	// Test ApplyWithIndex
	altSet := buildAlternateSet([]GlyphID{97, 200, 201}) // 3 alternates
	subtable := buildAlternateSubst([]GlyphID{65}, [][]byte{altSet})

	subst, err := parseAlternateSubst(subtable, 0)
	if err != nil {
		t.Fatalf("parseAlternateSubst failed: %v", err)
	}

	tests := []struct {
		altIndex int
		want     GlyphID
		desc     string
	}{
		{0, 97, "first alternate"},
		{1, 200, "second alternate"},
		{2, 201, "third alternate"},
		{-1, 97, "negative index -> first"},
		{10, 201, "out of range -> last"},
	}

	for _, tt := range tests {
		ctx := &GSUBContext{
			Glyphs: []GlyphID{65},
			Index:  0,
		}
		subst.ApplyWithIndex(ctx, tt.altIndex)
		if ctx.Glyphs[0] != tt.want {
			t.Errorf("%s: got %d, want %d", tt.desc, ctx.Glyphs[0], tt.want)
		}
	}
}

func TestChainContextSubstFormat2Basic(t *testing.T) {
	// Test Format 2: Class-based context
	// Define classes:
	// Class 0: default (everything else)
	// Class 1: vowels (a=65, e=69, i=73, o=79, u=85)
	// Class 2: consonants (b=66, c=67, d=68)
	//
	// Rule: If a vowel is preceded by class 2 and followed by class 2, substitute it

	// Lookup 0: SingleSubst that converts vowels to uppercase equivalents
	// a(65)->A(97), e(69)->E(101), i(73)->I(105)
	singleSubst := buildSingleSubstFormat2(
		[]GlyphID{65, 69, 73},
		[]GlyphID{97, 101, 105},
	)
	lookup0 := buildGSUBLookup(GSUBTypeSingle, [][]byte{singleSubst})

	// ClassDef: startGlyph=65, classes for glyphs 65-73
	// 65(a)=1, 66(b)=2, 67(c)=2, 68(d)=2, 69(e)=1, 70(f)=0, 71(g)=0, 72(h)=0, 73(i)=1
	inputClassDef := buildClassDefFormat1(65, []uint16{1, 2, 2, 2, 1, 0, 0, 0, 1})
	backtrackClassDef := buildClassDefFormat1(65, []uint16{1, 2, 2, 2, 1, 0, 0, 0, 1})
	lookaheadClassDef := buildClassDefFormat1(65, []uint16{1, 2, 2, 2, 1, 0, 0, 0, 1})

	// Rule for class 1 (vowels): backtrack=class2, input=class1, lookahead=class2
	// In class-based rules, the values in the rule are class IDs, not glyph IDs
	rule := buildChainRule(
		[]GlyphID{2}, // backtrack: class 2 (consonant)
		[]GlyphID{},  // no additional input (just the first glyph - a vowel)
		[]GlyphID{2}, // lookahead: class 2 (consonant)
		[]LookupRecord{{SequenceIndex: 0, LookupIndex: 0}},
	)
	ruleSet := buildChainRuleSet([][]byte{rule})

	// RuleSets array: indexed by class. We need ruleSet for class 1 (vowels)
	// Class 0: no rules (nil)
	// Class 1: our rule
	ruleSets := [][]byte{nil, ruleSet}

	// Coverage includes all vowels we want to match
	chainContext := buildChainContextFormat2(
		[]GlyphID{65, 69, 73}, // vowels a, e, i
		backtrackClassDef,
		inputClassDef,
		lookaheadClassDef,
		ruleSets,
	)
	lookup1 := buildGSUBLookup(GSUBTypeChainContext, [][]byte{chainContext})

	gsubData := buildGSUBTable([][]byte{lookup0, lookup1})
	gsub, err := ParseGSUB(gsubData)
	if err != nil {
		t.Fatalf("ParseGSUB failed: %v", err)
	}

	tests := []struct {
		input []GlyphID
		want  []GlyphID
		desc  string
	}{
		{[]GlyphID{66, 65, 67}, []GlyphID{66, 97, 67}, "b+a+c -> b+A+c (consonant-vowel-consonant)"},
		{[]GlyphID{66, 69, 68}, []GlyphID{66, 101, 68}, "b+e+d -> b+E+d (consonant-vowel-consonant)"},
		{[]GlyphID{65, 65, 67}, []GlyphID{65, 65, 67}, "a+a+c -> no change (vowel before)"},
		{[]GlyphID{66, 65, 65}, []GlyphID{66, 65, 65}, "b+a+a -> no change (vowel after)"},
		{[]GlyphID{66, 65}, []GlyphID{66, 65}, "b+a -> no change (no lookahead)"},
	}

	for _, tt := range tests {
		glyphs := append([]GlyphID{}, tt.input...)
		result := gsub.ApplyLookup(1, glyphs)

		if len(result) != len(tt.want) {
			t.Errorf("%s: got %d glyphs (%v), want %d (%v)", tt.desc, len(result), result, len(tt.want), tt.want)
			continue
		}
		for i := range tt.want {
			if result[i] != tt.want[i] {
				t.Errorf("%s: result[%d] = %d, want %d", tt.desc, i, result[i], tt.want[i])
			}
		}
	}
}

// --- ReverseChainSingleSubst Tests ---

// Helper to build a ReverseChainSingleSubst subtable
func buildReverseChainSingleSubst(
	coverageGlyphs []GlyphID,
	backtrackCoverages [][]GlyphID,
	lookaheadCoverages [][]GlyphID,
	substitutes []GlyphID,
) []byte {
	// Build all coverage tables first
	mainCoverage := buildCoverageFormat1(coverageGlyphs)

	backtrackCovs := make([][]byte, len(backtrackCoverages))
	for i, glyphs := range backtrackCoverages {
		backtrackCovs[i] = buildCoverageFormat1(glyphs)
	}

	lookaheadCovs := make([][]byte, len(lookaheadCoverages))
	for i, glyphs := range lookaheadCoverages {
		lookaheadCovs[i] = buildCoverageFormat1(glyphs)
	}

	// Calculate header size
	// format(2) + coverageOffset(2) + backtrackCount(2) + backtrackOffsets + lookaheadCount(2) + lookaheadOffsets + substituteCount(2) + substitutes
	headerSize := 2 + 2 + 2 + len(backtrackCoverages)*2 + 2 + len(lookaheadCoverages)*2 + 2 + len(substitutes)*2

	// Calculate total size
	totalSize := headerSize + len(mainCoverage)
	for _, cov := range backtrackCovs {
		totalSize += len(cov)
	}
	for _, cov := range lookaheadCovs {
		totalSize += len(cov)
	}

	data := make([]byte, totalSize)
	off := 0

	// Format
	binary.BigEndian.PutUint16(data[off:], 1)
	off += 2

	// Coverage offset (after all inline data)
	covOffset := headerSize
	binary.BigEndian.PutUint16(data[off:], uint16(covOffset))
	off += 2
	covOffset += len(mainCoverage)

	// Backtrack count and offsets
	binary.BigEndian.PutUint16(data[off:], uint16(len(backtrackCoverages)))
	off += 2
	for _, cov := range backtrackCovs {
		binary.BigEndian.PutUint16(data[off:], uint16(covOffset))
		off += 2
		covOffset += len(cov)
	}

	// Lookahead count and offsets
	binary.BigEndian.PutUint16(data[off:], uint16(len(lookaheadCoverages)))
	off += 2
	for _, cov := range lookaheadCovs {
		binary.BigEndian.PutUint16(data[off:], uint16(covOffset))
		off += 2
		covOffset += len(cov)
	}

	// Substitute count and glyphs
	binary.BigEndian.PutUint16(data[off:], uint16(len(substitutes)))
	off += 2
	for _, s := range substitutes {
		binary.BigEndian.PutUint16(data[off:], uint16(s))
		off += 2
	}

	// Copy coverage tables
	copy(data[off:], mainCoverage)
	off += len(mainCoverage)
	for _, cov := range backtrackCovs {
		copy(data[off:], cov)
		off += len(cov)
	}
	for _, cov := range lookaheadCovs {
		copy(data[off:], cov)
		off += len(cov)
	}

	return data
}

func TestReverseChainSingleSubstBasic(t *testing.T) {
	// Test basic reverse chain single substitution
	// Substitute glyph 65 (a) with 97 (A) when preceded by glyph 66 (b)

	reverseSubst := buildReverseChainSingleSubst(
		[]GlyphID{65},     // coverage: glyph 65 (a)
		[][]GlyphID{{66}}, // backtrack: glyph 66 (b)
		[][]GlyphID{},     // no lookahead
		[]GlyphID{97},     // substitutes: 97 (A)
	)

	lookup := buildGSUBLookup(GSUBTypeReverseChainSingle, [][]byte{reverseSubst})
	gsubData := buildGSUBTable([][]byte{lookup})

	gsub, err := ParseGSUB(gsubData)
	if err != nil {
		t.Fatalf("ParseGSUB failed: %v", err)
	}

	tests := []struct {
		input []GlyphID
		want  []GlyphID
		desc  string
	}{
		{[]GlyphID{66, 65}, []GlyphID{66, 97}, "b+a -> b+A (preceded by b)"},
		{[]GlyphID{67, 65}, []GlyphID{67, 65}, "c+a -> c+a (not preceded by b)"},
		{[]GlyphID{65}, []GlyphID{65}, "a alone -> no change (no backtrack)"},
		{[]GlyphID{66, 65, 65}, []GlyphID{66, 97, 65}, "b+a+a -> b+A+a (only second a matched in reverse)"},
	}

	for _, tt := range tests {
		glyphs := append([]GlyphID{}, tt.input...)
		result := gsub.ApplyLookupReverse(0, glyphs)

		if len(result) != len(tt.want) {
			t.Errorf("%s: got %d glyphs (%v), want %d (%v)", tt.desc, len(result), result, len(tt.want), tt.want)
			continue
		}
		for i := range tt.want {
			if result[i] != tt.want[i] {
				t.Errorf("%s: result[%d] = %d, want %d", tt.desc, i, result[i], tt.want[i])
			}
		}
	}
}

func TestReverseChainSingleSubstLookahead(t *testing.T) {
	// Test reverse chain single substitution with lookahead
	// Substitute glyph 65 (a) with 97 (A) when followed by glyph 67 (c)

	reverseSubst := buildReverseChainSingleSubst(
		[]GlyphID{65},     // coverage: glyph 65 (a)
		[][]GlyphID{},     // no backtrack
		[][]GlyphID{{67}}, // lookahead: glyph 67 (c)
		[]GlyphID{97},     // substitutes: 97 (A)
	)

	lookup := buildGSUBLookup(GSUBTypeReverseChainSingle, [][]byte{reverseSubst})
	gsubData := buildGSUBTable([][]byte{lookup})

	gsub, err := ParseGSUB(gsubData)
	if err != nil {
		t.Fatalf("ParseGSUB failed: %v", err)
	}

	tests := []struct {
		input []GlyphID
		want  []GlyphID
		desc  string
	}{
		{[]GlyphID{65, 67}, []GlyphID{97, 67}, "a+c -> A+c (followed by c)"},
		{[]GlyphID{65, 68}, []GlyphID{65, 68}, "a+d -> a+d (not followed by c)"},
		{[]GlyphID{65}, []GlyphID{65}, "a alone -> no change (no lookahead)"},
	}

	for _, tt := range tests {
		glyphs := append([]GlyphID{}, tt.input...)
		result := gsub.ApplyLookupReverse(0, glyphs)

		if len(result) != len(tt.want) {
			t.Errorf("%s: got %d glyphs (%v), want %d (%v)", tt.desc, len(result), result, len(tt.want), tt.want)
			continue
		}
		for i := range tt.want {
			if result[i] != tt.want[i] {
				t.Errorf("%s: result[%d] = %d, want %d", tt.desc, i, result[i], tt.want[i])
			}
		}
	}
}

func TestReverseChainSingleSubstMultiple(t *testing.T) {
	// Test reverse chain single substitution with multiple substitutes
	// Substitute glyphs 65,66,67 (a,b,c) with 97,98,99 (A,B,C) when followed by glyph 90 (Z)

	reverseSubst := buildReverseChainSingleSubst(
		[]GlyphID{65, 66, 67}, // coverage: a, b, c
		[][]GlyphID{},         // no backtrack
		[][]GlyphID{{90}},     // lookahead: Z
		[]GlyphID{97, 98, 99}, // substitutes: A, B, C
	)

	lookup := buildGSUBLookup(GSUBTypeReverseChainSingle, [][]byte{reverseSubst})
	gsubData := buildGSUBTable([][]byte{lookup})

	gsub, err := ParseGSUB(gsubData)
	if err != nil {
		t.Fatalf("ParseGSUB failed: %v", err)
	}

	tests := []struct {
		input []GlyphID
		want  []GlyphID
		desc  string
	}{
		{[]GlyphID{65, 90}, []GlyphID{97, 90}, "a+Z -> A+Z"},
		{[]GlyphID{66, 90}, []GlyphID{98, 90}, "b+Z -> B+Z"},
		{[]GlyphID{67, 90}, []GlyphID{99, 90}, "c+Z -> C+Z"},
		{[]GlyphID{65, 66, 67, 90}, []GlyphID{65, 66, 99, 90}, "a+b+c+Z -> a+b+C+Z (only last before Z matches)"},
	}

	for _, tt := range tests {
		glyphs := append([]GlyphID{}, tt.input...)
		result := gsub.ApplyLookupReverse(0, glyphs)

		if len(result) != len(tt.want) {
			t.Errorf("%s: got %d glyphs (%v), want %d (%v)", tt.desc, len(result), result, len(tt.want), tt.want)
			continue
		}
		for i := range tt.want {
			if result[i] != tt.want[i] {
				t.Errorf("%s: result[%d] = %d, want %d", tt.desc, i, result[i], tt.want[i])
			}
		}
	}
}

// --- Extension Lookup Tests ---

// buildExtensionSubtable builds an Extension subtable that wraps another subtable
// Extension format: format(2) + extensionLookupType(2) + extensionOffset(4) + actual subtable
func buildExtensionSubtable(extensionLookupType uint16, subtable []byte) []byte {
	// Extension header is 8 bytes, subtable follows immediately after
	data := make([]byte, 8+len(subtable))
	binary.BigEndian.PutUint16(data[0:], 1)                   // format = 1
	binary.BigEndian.PutUint16(data[2:], extensionLookupType) // actual lookup type
	binary.BigEndian.PutUint32(data[4:], 8)                   // offset to subtable (right after header)
	copy(data[8:], subtable)
	return data
}

func TestExtensionSubstWithSingleSubst(t *testing.T) {
	// Test Extension lookup wrapping a SingleSubst
	// SingleSubst: substitute glyph 65 -> 97 (a -> A)
	singleSubst := buildSingleSubstFormat1([]GlyphID{65, 66, 67}, 32) // +32: lowercase to uppercase

	// Wrap in Extension
	extensionSubtable := buildExtensionSubtable(GSUBTypeSingle, singleSubst)

	// Build lookup with Extension type
	lookup := buildGSUBLookup(GSUBTypeExtension, [][]byte{extensionSubtable})
	gsubData := buildGSUBTable([][]byte{lookup})

	gsub, err := ParseGSUB(gsubData)
	if err != nil {
		t.Fatalf("ParseGSUB failed: %v", err)
	}

	if gsub.NumLookups() != 1 {
		t.Fatalf("NumLookups = %d, want 1", gsub.NumLookups())
	}

	// Apply lookup
	glyphs := []GlyphID{65, 66, 67, 68}
	result := gsub.ApplyLookup(0, glyphs)

	// 65, 66, 67 should be substituted to 97, 98, 99
	// 68 is not in coverage, should remain unchanged
	expected := []GlyphID{97, 98, 99, 68}
	if len(result) != len(expected) {
		t.Fatalf("got %d glyphs (%v), want %d (%v)", len(result), result, len(expected), expected)
	}
	for i, want := range expected {
		if result[i] != want {
			t.Errorf("result[%d] = %d, want %d", i, result[i], want)
		}
	}
}

func TestExtensionSubstWithLigature(t *testing.T) {
	// Test Extension lookup wrapping a LigatureSubst
	// LigatureSubst: f + i -> fi ligature (glyph 500)
	fiLig := buildLigature(500, []GlyphID{105})                      // fi ligature: 500, requires 'i' (105) after first glyph
	ligSet := buildLigatureSet([][]byte{fiLig})                      // LigatureSet for 'f'
	ligSubst := buildLigatureSubst([]GlyphID{102}, [][]byte{ligSet}) // coverage: 'f' (102)

	// Wrap in Extension
	extensionSubtable := buildExtensionSubtable(GSUBTypeLigature, ligSubst)

	// Build lookup with Extension type
	lookup := buildGSUBLookup(GSUBTypeExtension, [][]byte{extensionSubtable})
	gsubData := buildGSUBTable([][]byte{lookup})

	gsub, err := ParseGSUB(gsubData)
	if err != nil {
		t.Fatalf("ParseGSUB failed: %v", err)
	}

	// Apply lookup
	glyphs := []GlyphID{102, 105, 110} // f, i, n
	result := gsub.ApplyLookup(0, glyphs)

	// f + i should become fi ligature
	expected := []GlyphID{500, 110}
	if len(result) != len(expected) {
		t.Fatalf("got %d glyphs (%v), want %d (%v)", len(result), result, len(expected), expected)
	}
	for i, want := range expected {
		if result[i] != want {
			t.Errorf("result[%d] = %d, want %d", i, result[i], want)
		}
	}
}

func TestExtensionSubstInvalidFormat(t *testing.T) {
	// Test that invalid extension format is handled gracefully
	singleSubst := buildSingleSubstFormat1([]GlyphID{65}, 32)

	// Build extension with invalid format (2 instead of 1)
	data := make([]byte, 8+len(singleSubst))
	binary.BigEndian.PutUint16(data[0:], 2) // format = 2 (INVALID)
	binary.BigEndian.PutUint16(data[2:], GSUBTypeSingle)
	binary.BigEndian.PutUint32(data[4:], 8)
	copy(data[8:], singleSubst)

	lookup := buildGSUBLookup(GSUBTypeExtension, [][]byte{data})
	gsubData := buildGSUBTable([][]byte{lookup})

	gsub, err := ParseGSUB(gsubData)
	if err != nil {
		t.Fatalf("ParseGSUB failed: %v", err)
	}

	// The lookup should have no subtables (invalid extension was skipped)
	glyphs := []GlyphID{65}
	result := gsub.ApplyLookup(0, glyphs)

	// Should be unchanged (no valid subtable to apply)
	if len(result) != 1 || result[0] != 65 {
		t.Errorf("got %v, expected [65] (unchanged)", result)
	}
}
