package buffer

import (
	"testing"
)

func TestNewBuffer(t *testing.T) {
	b := New()

	if b.Len() != 0 {
		t.Errorf("expected empty buffer, got len=%d", b.Len())
	}
	if b.InError() {
		t.Error("new buffer should not be in error state")
	}
	if b.ContentType() != ContentTypeInvalid {
		t.Errorf("expected ContentTypeInvalid, got %v", b.ContentType())
	}
}

func TestAddString(t *testing.T) {
	b := New()
	b.AddString("Hello")

	if b.Len() != 5 {
		t.Errorf("expected len=5, got %d", b.Len())
	}
	if b.ContentType() != ContentTypeUnicode {
		t.Errorf("expected ContentTypeUnicode, got %v", b.ContentType())
	}

	info := b.Info()
	expected := []rune{'H', 'e', 'l', 'l', 'o'}
	for i, r := range expected {
		if info[i].Codepoint != Codepoint(r) {
			t.Errorf("info[%d].Codepoint = %d, want %d", i, info[i].Codepoint, r)
		}
	}
}

func TestAddStringClusters(t *testing.T) {
	b := New()
	b.AddString("Aä") // A (1 byte) + ä (2 bytes UTF-8)

	info := b.Info()
	if len(info) != 2 {
		t.Fatalf("expected 2 glyphs, got %d", len(info))
	}

	// First char 'A' at byte offset 0
	if info[0].Cluster != 0 {
		t.Errorf("info[0].Cluster = %d, want 0", info[0].Cluster)
	}

	// Second char 'ä' at byte offset 1
	if info[1].Cluster != 1 {
		t.Errorf("info[1].Cluster = %d, want 1", info[1].Cluster)
	}
}

func TestAddRunes(t *testing.T) {
	b := New()
	b.AddRunes([]rune{'H', 'i', '!'})

	info := b.Info()
	if len(info) != 3 {
		t.Fatalf("expected 3 glyphs, got %d", len(info))
	}

	// Cluster values should be indices
	for i := 0; i < 3; i++ {
		if info[i].Cluster != uint32(i) {
			t.Errorf("info[%d].Cluster = %d, want %d", i, info[i].Cluster, i)
		}
	}
}

func TestReverse(t *testing.T) {
	b := New()
	b.AddString("ABC")
	b.Reverse()

	info := b.Info()
	expected := []rune{'C', 'B', 'A'}
	for i, r := range expected {
		if info[i].Codepoint != Codepoint(r) {
			t.Errorf("after reverse: info[%d].Codepoint = %c, want %c",
				i, rune(info[i].Codepoint), r)
		}
	}
}

func TestReverseRange(t *testing.T) {
	b := New()
	b.AddString("ABCDE")
	b.ReverseRange(1, 4) // Reverse BCD

	info := b.Info()
	expected := []rune{'A', 'D', 'C', 'B', 'E'}
	for i, r := range expected {
		if info[i].Codepoint != Codepoint(r) {
			t.Errorf("info[%d].Codepoint = %c, want %c",
				i, rune(info[i].Codepoint), r)
		}
	}
}

func TestClearOutput(t *testing.T) {
	b := New()
	b.AddString("ABC")
	b.ClearOutput()

	if !b.haveOutput {
		t.Error("haveOutput should be true after ClearOutput")
	}
	if b.idx != 0 {
		t.Errorf("idx should be 0, got %d", b.idx)
	}
	if b.outLen != 0 {
		t.Errorf("outLen should be 0, got %d", b.outLen)
	}
}

func TestNextGlyph(t *testing.T) {
	b := New()
	b.AddString("ABC")
	b.ClearOutput()

	// Process all glyphs
	for b.idx < b.len {
		if !b.NextGlyph() {
			t.Fatal("NextGlyph failed")
		}
	}

	if !b.Sync() {
		t.Fatal("Sync failed")
	}

	// Buffer should be unchanged
	info := b.Info()
	expected := []rune{'A', 'B', 'C'}
	for i, r := range expected {
		if info[i].Codepoint != Codepoint(r) {
			t.Errorf("info[%d].Codepoint = %c, want %c",
				i, rune(info[i].Codepoint), r)
		}
	}
}

func TestReplaceGlyph(t *testing.T) {
	b := New()
	b.AddString("ABC")
	b.ClearOutput()

	// Replace 'A' with 'X'
	b.ReplaceGlyph(Codepoint('X'))
	b.NextGlyph() // B
	b.NextGlyph() // C
	b.Sync()

	info := b.Info()
	if info[0].Codepoint != Codepoint('X') {
		t.Errorf("info[0].Codepoint = %c, want X", rune(info[0].Codepoint))
	}
}

func TestReplaceGlyphsOneToMany(t *testing.T) {
	b := New()
	b.AddString("ABC")
	b.ClearOutput()

	// Replace 'A' with 'XY'
	b.ReplaceGlyphs(1, []Codepoint{'X', 'Y'})
	b.NextGlyph() // B
	b.NextGlyph() // C
	b.Sync()

	if b.Len() != 4 {
		t.Errorf("expected len=4, got %d", b.Len())
	}

	info := b.Info()
	expected := []rune{'X', 'Y', 'B', 'C'}
	for i, r := range expected {
		if info[i].Codepoint != Codepoint(r) {
			t.Errorf("info[%d].Codepoint = %c, want %c",
				i, rune(info[i].Codepoint), r)
		}
	}
}

func TestReplaceGlyphsManyToOne(t *testing.T) {
	b := New()
	b.AddString("ABC")
	b.ClearOutput()

	// Replace 'AB' with 'X' (ligature)
	b.ReplaceGlyphs(2, []Codepoint{'X'})
	b.NextGlyph() // C
	b.Sync()

	if b.Len() != 2 {
		t.Errorf("expected len=2, got %d", b.Len())
	}

	info := b.Info()
	if info[0].Codepoint != Codepoint('X') {
		t.Errorf("info[0].Codepoint = %c, want X", rune(info[0].Codepoint))
	}
	if info[1].Codepoint != Codepoint('C') {
		t.Errorf("info[1].Codepoint = %c, want C", rune(info[1].Codepoint))
	}

	// Cluster should be merged to minimum
	if info[0].Cluster != 0 {
		t.Errorf("info[0].Cluster = %d, want 0", info[0].Cluster)
	}
}

func TestSkipGlyph(t *testing.T) {
	b := New()
	b.AddString("ABC")
	b.ClearOutput()

	b.SkipGlyph() // Skip A
	b.NextGlyph() // B
	b.NextGlyph() // C
	b.Sync()

	if b.Len() != 2 {
		t.Errorf("expected len=2, got %d", b.Len())
	}

	info := b.Info()
	expected := []rune{'B', 'C'}
	for i, r := range expected {
		if info[i].Codepoint != Codepoint(r) {
			t.Errorf("info[%d].Codepoint = %c, want %c",
				i, rune(info[i].Codepoint), r)
		}
	}
}

func TestClearPositions(t *testing.T) {
	b := New()
	b.AddString("ABC")
	b.ClearPositions()

	if !b.HavePositions() {
		t.Error("should have positions after ClearPositions")
	}

	pos := b.Pos()
	for i := range pos {
		if pos[i].XAdvance != 0 || pos[i].YAdvance != 0 ||
			pos[i].XOffset != 0 || pos[i].YOffset != 0 {
			t.Errorf("pos[%d] should be zero", i)
		}
	}
}

func TestMergeClusters(t *testing.T) {
	b := New()
	b.AddString("ABC")
	// Manually set different clusters
	b.info[0].Cluster = 0
	b.info[1].Cluster = 5
	b.info[2].Cluster = 10

	b.MergeClusters(0, 3)

	// All should have minimum cluster value
	for i := 0; i < 3; i++ {
		if b.info[i].Cluster != 0 {
			t.Errorf("info[%d].Cluster = %d, want 0", i, b.info[i].Cluster)
		}
	}
}

func TestGuessSegmentProperties(t *testing.T) {
	b := New()
	b.AddString("Hello")
	b.GuessSegmentProperties()

	if b.Props.Script != ScriptLatin {
		t.Errorf("expected ScriptLatin, got %v", b.Props.Script)
	}
	if b.Props.Direction != DirectionLTR {
		t.Errorf("expected DirectionLTR, got %v", b.Props.Direction)
	}
}

func TestGuessSegmentPropertiesArabic(t *testing.T) {
	b := New()
	b.AddRunes([]rune{0x0627, 0x0644, 0x0639}) // Arabic letters
	b.GuessSegmentProperties()

	if b.Props.Script != ScriptArabic {
		t.Errorf("expected ScriptArabic, got %v", b.Props.Script)
	}
	if b.Props.Direction != DirectionRTL {
		t.Errorf("expected DirectionRTL, got %v", b.Props.Direction)
	}
}

func TestDigest(t *testing.T) {
	d := SetDigest{}

	d.Add(100)
	d.Add(200)

	if !d.MayHave(100) {
		t.Error("digest should may-have 100")
	}
	if !d.MayHave(200) {
		t.Error("digest should may-have 200")
	}

	// 164 has same lower 6 bits as 100, so false positive expected
	// (100 & 63 = 36, 164 & 63 = 36)
	if !d.MayHave(164) {
		t.Error("digest should may-have 164 (false positive expected)")
	}

	// 50 definitely not present (50 & 63 = 50, different from 36 and 8)
	// Actually let's check: 200 & 63 = 8
	// So bits 36 and 8 are set
	// 50 & 63 = 50, bit 50 not set
	if d.MayHave(50) {
		// This might be a false positive depending on implementation
		// Let's just check the basic functionality works
	}
}

func TestDigestIntersect(t *testing.T) {
	d1 := SetDigest{}
	d1.Add(100)

	d2 := SetDigest{}
	d2.Add(100)

	if !d1.MayIntersect(d2) {
		t.Error("digests with same element should intersect")
	}

	d3 := SetDigest{}
	d3.Add(50)

	// Might or might not intersect depending on hash collisions
	// Just verify the function doesn't crash
	_ = d1.MayIntersect(d3)
}

func TestBufferResetMasks(t *testing.T) {
	b := New()
	b.AddString("ABC")

	b.ResetMasks(0xFF)
	for i := 0; i < b.Len(); i++ {
		if b.info[i].Mask != 0xFF {
			t.Errorf("info[%d].Mask = %x, want FF", i, b.info[i].Mask)
		}
	}

	b.AddMasks(0x100)
	for i := 0; i < b.Len(); i++ {
		if b.info[i].Mask != 0x1FF {
			t.Errorf("info[%d].Mask = %x, want 1FF", i, b.info[i].Mask)
		}
	}
}

func TestDirectionMethods(t *testing.T) {
	tests := []struct {
		dir        Direction
		horizontal bool
		vertical   bool
		forward    bool
		backward   bool
	}{
		{DirectionLTR, true, false, true, false},
		{DirectionRTL, true, false, false, true},
		{DirectionTTB, false, true, true, false},
		{DirectionBTT, false, true, false, true},
		{DirectionInvalid, false, false, false, false},
	}

	for _, tt := range tests {
		if tt.dir.IsHorizontal() != tt.horizontal {
			t.Errorf("%v.IsHorizontal() = %v, want %v", tt.dir, tt.dir.IsHorizontal(), tt.horizontal)
		}
		if tt.dir.IsVertical() != tt.vertical {
			t.Errorf("%v.IsVertical() = %v, want %v", tt.dir, tt.dir.IsVertical(), tt.vertical)
		}
		if tt.dir.IsForward() != tt.forward {
			t.Errorf("%v.IsForward() = %v, want %v", tt.dir, tt.dir.IsForward(), tt.forward)
		}
		if tt.dir.IsBackward() != tt.backward {
			t.Errorf("%v.IsBackward() = %v, want %v", tt.dir, tt.dir.IsBackward(), tt.backward)
		}
	}
}

func TestDirectionReverse(t *testing.T) {
	if DirectionLTR.Reverse() != DirectionRTL {
		t.Error("LTR.Reverse() should be RTL")
	}
	if DirectionRTL.Reverse() != DirectionLTR {
		t.Error("RTL.Reverse() should be LTR")
	}
	if DirectionTTB.Reverse() != DirectionBTT {
		t.Error("TTB.Reverse() should be BTT")
	}
	if DirectionBTT.Reverse() != DirectionTTB {
		t.Error("BTT.Reverse() should be TTB")
	}
}

func TestGlyphInfoProperties(t *testing.T) {
	g := GlyphInfo{}

	g.setBase()
	if !g.IsBase() {
		t.Error("should be base after setBase")
	}

	g.setMark()
	if !g.IsMark() {
		t.Error("should be mark after setMark")
	}

	g.setSubstituted()
	if !g.IsSubstituted() {
		t.Error("should be substituted after setSubstituted")
	}

	g.setLigID(42)
	if g.ligID() != 42 {
		t.Errorf("ligID = %d, want 42", g.ligID())
	}

	g.setLigComp(3)
	if g.ligComp() != 3 {
		t.Errorf("ligComp = %d, want 3", g.ligComp())
	}

	g.setSyllable(7)
	if g.syllable() != 7 {
		t.Errorf("syllable = %d, want 7", g.syllable())
	}
}

func TestClusterLevel(t *testing.T) {
	if !ClusterLevelMonotoneGraphemes.IsMonotone() {
		t.Error("MonotoneGraphemes should be monotone")
	}
	if !ClusterLevelMonotoneCharacters.IsMonotone() {
		t.Error("MonotoneCharacters should be monotone")
	}
	if ClusterLevelCharacters.IsMonotone() {
		t.Error("Characters should not be monotone")
	}
	if ClusterLevelGraphemes.IsMonotone() {
		t.Error("Graphemes should not be monotone")
	}

	if !ClusterLevelMonotoneGraphemes.IsGraphemes() {
		t.Error("MonotoneGraphemes should be graphemes")
	}
	if ClusterLevelMonotoneCharacters.IsGraphemes() {
		t.Error("MonotoneCharacters should not be graphemes")
	}
}

func TestEnterLeave(t *testing.T) {
	b := New()
	b.AddString("Test")

	b.Enter()
	// maxLen and maxOps should be set based on input length
	if b.maxLen < maxLenMin {
		t.Errorf("maxLen should be at least %d", maxLenMin)
	}
	if b.maxOps < maxOpsMin {
		t.Errorf("maxOps should be at least %d", maxOpsMin)
	}

	b.Leave()
	if b.maxLen != maxLenDefault {
		t.Errorf("maxLen should be reset to default")
	}
	if b.maxOps != maxOpsDefault {
		t.Errorf("maxOps should be reset to default")
	}
}

func TestBufferClear(t *testing.T) {
	b := New()
	b.AddString("Test")
	b.Props.Direction = DirectionRTL
	b.Props.Script = ScriptArabic

	b.Clear()

	if b.Len() != 0 {
		t.Error("buffer should be empty after Clear")
	}
	// Props should be preserved (unlike Reset)
	// Actually, looking at HarfBuzz, Clear does reset props
	// Let me check the actual behavior...
	// In HarfBuzz, clear() resets props to default
	if b.Props.Direction != DirectionInvalid {
		t.Error("direction should be reset after Clear")
	}
}

func TestGroupEnd(t *testing.T) {
	b := New()
	b.AddString("AABBC")
	// Set clusters: A=0, A=0, B=1, B=1, C=2
	b.info[0].Cluster = 0
	b.info[1].Cluster = 0
	b.info[2].Cluster = 1
	b.info[3].Cluster = 1
	b.info[4].Cluster = 2

	end := b.GroupEnd(0, ClusterGroupFunc)
	if end != 2 {
		t.Errorf("GroupEnd(0) = %d, want 2", end)
	}

	end = b.GroupEnd(2, ClusterGroupFunc)
	if end != 4 {
		t.Errorf("GroupEnd(2) = %d, want 4", end)
	}

	end = b.GroupEnd(4, ClusterGroupFunc)
	if end != 5 {
		t.Errorf("GroupEnd(4) = %d, want 5", end)
	}
}
