package ot

import (
	"os"
	"testing"
)

func TestParseHhea(t *testing.T) {
	fontPath := findTestFont("Roboto-Regular.ttf")
	if fontPath == "" {
		t.Skip("Roboto-Regular.ttf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	hheaData, err := font.TableData(TagHhea)
	if err != nil {
		t.Fatalf("Failed to get hhea table: %v", err)
	}

	hhea, err := ParseHhea(hheaData)
	if err != nil {
		t.Fatalf("Failed to parse hhea: %v", err)
	}

	t.Logf("hhea:")
	t.Logf("  Ascender: %d", hhea.Ascender)
	t.Logf("  Descender: %d", hhea.Descender)
	t.Logf("  LineGap: %d", hhea.LineGap)
	t.Logf("  AdvanceWidthMax: %d", hhea.AdvanceWidthMax)
	t.Logf("  NumberOfHMetrics: %d", hhea.NumberOfHMetrics)

	if hhea.NumberOfHMetrics == 0 {
		t.Error("NumberOfHMetrics should not be 0")
	}
}

func TestParseHmtx(t *testing.T) {
	fontPath := findTestFont("Roboto-Regular.ttf")
	if fontPath == "" {
		t.Skip("Roboto-Regular.ttf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	hmtx, err := ParseHmtxFromFont(font)
	if err != nil {
		t.Fatalf("Failed to parse hmtx: %v", err)
	}

	// Test some known glyphs
	// Parse cmap to get glyph IDs
	cmapData, _ := font.TableData(TagCmap)
	cmap, _ := ParseCmap(cmapData)

	testChars := []rune{'A', 'V', 'f', 'i', ' '}
	for _, ch := range testChars {
		glyph, ok := cmap.Lookup(Codepoint(ch))
		if !ok {
			continue
		}
		adv := hmtx.GetAdvanceWidth(glyph)
		lsb := hmtx.GetLsb(glyph)
		t.Logf("'%c' (glyph %d): advanceWidth=%d, lsb=%d", ch, glyph, adv, lsb)
	}
}

func TestHmtxCompareWithHBShape(t *testing.T) {
	fontPath := findTestFont("Roboto-Regular.ttf")
	if fontPath == "" {
		t.Skip("Roboto-Regular.ttf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	hmtx, err := ParseHmtxFromFont(font)
	if err != nil {
		t.Fatalf("Failed to parse hmtx: %v", err)
	}

	// hb-shape shows advances like: [gid37=0+1249|gid58=1+1303]
	// The +N is the advance width
	// Let's verify some known values

	// From hb-shape "AV": [gid37=0+1249|gid58=1+1303]
	// But wait, that includes kerning. Let's check raw advance.
	// hb-shape --no-glyph-names --no-positions shows: [37|58]
	// hb-shape --features="-kern" "AV" shows: [gid37=0+1336|gid58=1+1303]
	// So A has advance 1336, V has advance 1303

	tests := []struct {
		glyph    GlyphID
		expected uint16
	}{
		{37, 1336}, // 'A'
		{58, 1303}, // 'V'
	}

	for _, tt := range tests {
		adv := hmtx.GetAdvanceWidth(tt.glyph)
		if adv != tt.expected {
			t.Errorf("Glyph %d: advanceWidth=%d, want %d", tt.glyph, adv, tt.expected)
		} else {
			t.Logf("Glyph %d: advanceWidth=%d ✓", tt.glyph, adv)
		}
	}
}

func TestHmtxGlyphBeyondHMetrics(t *testing.T) {
	fontPath := findTestFont("Roboto-Regular.ttf")
	if fontPath == "" {
		t.Skip("Roboto-Regular.ttf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	// Get numberOfHMetrics
	hheaData, _ := font.TableData(TagHhea)
	hhea, _ := ParseHhea(hheaData)

	hmtx, err := ParseHmtxFromFont(font)
	if err != nil {
		t.Fatalf("Failed to parse hmtx: %v", err)
	}

	numGlyphs := font.NumGlyphs()
	t.Logf("numGlyphs=%d, numberOfHMetrics=%d", numGlyphs, hhea.NumberOfHMetrics)

	// Test a glyph beyond numberOfHMetrics if any exist
	if int(hhea.NumberOfHMetrics) < numGlyphs {
		testGlyph := GlyphID(hhea.NumberOfHMetrics)
		adv := hmtx.GetAdvanceWidth(testGlyph)
		lastAdv := hmtx.GetAdvanceWidth(GlyphID(hhea.NumberOfHMetrics - 1))
		t.Logf("Glyph %d (beyond hMetrics): advanceWidth=%d (should equal last=%d)",
			testGlyph, adv, lastAdv)
		if adv != lastAdv {
			t.Errorf("Advance width should match last hMetrics entry")
		}
	}
}
