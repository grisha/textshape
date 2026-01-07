package ot

import (
	"os"
	"testing"
)

func TestDebugLigature(t *testing.T) {
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

	// Parse GSUB
	gsubData, _ := font.TableData(TagGSUB)
	gsub, _ := ParseGSUB(gsubData)

	// Parse cmap
	cmapData, _ := font.TableData(TagCmap)
	cmap, _ := ParseCmap(cmapData)

	// Get glyph IDs for 'f' and 'i'
	fGlyph, _ := cmap.Lookup('f')
	iGlyph, _ := cmap.Lookup('i')
	t.Logf("'f' = glyph %d, 'i' = glyph %d", fGlyph, iGlyph)

	// Find the 'liga' feature
	featureList, err := gsub.ParseFeatureList()
	if err != nil {
		t.Fatalf("Failed to parse feature list: %v", err)
	}

	t.Logf("Total features: %d", featureList.Count())

	// Look for liga feature
	ligaLookups := featureList.FindFeature(TagLiga)
	t.Logf("'liga' feature lookups: %v", ligaLookups)

	if ligaLookups == nil {
		t.Fatal("No 'liga' feature found")
	}

	// Check what type of lookups these are
	for _, idx := range ligaLookups {
		lookup := gsub.GetLookup(int(idx))
		if lookup != nil {
			t.Logf("  Lookup %d: Type=%d (4=Ligature), Flag=0x%04x, Subtables=%d",
				idx, lookup.Type, lookup.Flag, len(lookup.Subtables()))
		}
	}

	// Try applying the lookup directly
	glyphs := []GlyphID{fGlyph, iGlyph}
	t.Logf("Before GSUB: %v", glyphs)

	// Apply just the liga lookup
	for _, idx := range ligaLookups {
		glyphs = gsub.ApplyLookup(int(idx), glyphs)
		t.Logf("After lookup %d: %v", idx, glyphs)
	}

	// Also try ApplyFeature
	glyphs2 := []GlyphID{fGlyph, iGlyph}
	result := gsub.ApplyFeature(TagLiga, glyphs2)
	t.Logf("ApplyFeature(liga): %v -> %v", glyphs2, result)

	// Debug: Check if 'f' is in the coverage of the ligature lookup
	lookup := gsub.GetLookup(9)
	if lookup != nil {
		t.Logf("Debugging lookup 9 subtables...")
		for i, st := range lookup.Subtables() {
			if ls, ok := st.(*LigatureSubst); ok {
				t.Logf("  Subtable %d is LigatureSubst", i)
				t.Logf("    Total LigatureSets: %d", len(ls.LigatureSets()))

				// Print ALL ligature sets
				for setIdx, ligSet := range ls.LigatureSets() {
					if len(ligSet) > 0 {
						t.Logf("    LigatureSet[%d]: %d ligatures", setIdx, len(ligSet))
						for j, lig := range ligSet {
							t.Logf("      Ligature %d: LigGlyph=%d, Components=%v", j, lig.LigGlyph, lig.Components)
						}
					}
				}

				// Check if fGlyph is covered
				covIdx := ls.Coverage().GetCoverage(fGlyph)
				t.Logf("    Coverage of 'f' (glyph %d): %d (NotCovered=%d)", fGlyph, covIdx, NotCovered)

				// Also check 'i'
				iCovIdx := ls.Coverage().GetCoverage(iGlyph)
				t.Logf("    Coverage of 'i' (glyph %d): %d", iGlyph, iCovIdx)
			}
		}
	}
}

func TestDebugKerning(t *testing.T) {
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

	shaper, err := NewShaper(font)
	if err != nil {
		t.Fatalf("Failed to create shaper: %v", err)
	}

	// Test "AV" - should have kerning
	// hb-shape shows: [gid37=0+1249|gid58=1+1303]
	glyphs, positions := shaper.ShapeString("AV")

	t.Logf("Shaped 'AV':")
	for i, g := range glyphs {
		t.Logf("  gid%d: XAdvance=%d", g, positions[i].XAdvance)
	}

	// Check against hb-shape output
	// A should have advance 1249 (1336 base - 87 kerning)
	// V should have advance 1303
	if len(glyphs) != 2 {
		t.Fatalf("Expected 2 glyphs, got %d", len(glyphs))
	}

	if glyphs[0] != 37 || glyphs[1] != 58 {
		t.Errorf("Wrong glyphs: got [%d, %d], want [37, 58]", glyphs[0], glyphs[1])
	}

	if positions[0].XAdvance != 1249 {
		t.Errorf("'A' advance: got %d, want 1249", positions[0].XAdvance)
	} else {
		t.Logf("'A' advance: %d ✓", positions[0].XAdvance)
	}

	if positions[1].XAdvance != 1303 {
		t.Errorf("'V' advance: got %d, want 1303", positions[1].XAdvance)
	} else {
		t.Logf("'V' advance: %d ✓", positions[1].XAdvance)
	}
}

func TestCompareWithHBShape(t *testing.T) {
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

	shaper, err := NewShaper(font)
	if err != nil {
		t.Fatalf("Failed to create shaper: %v", err)
	}

	tests := []struct {
		input    string
		expected []GlyphID // From hb-shape
	}{
		{"Hello", []GlyphID{44, 73, 80, 80, 83}},
		{"AV", []GlyphID{37, 58}},
		{"Test", []GlyphID{56, 73, 87, 88}},
		{"fi", []GlyphID{444}},                 // fi ligature
		{"fl", []GlyphID{445}},                 // fl ligature
		{"ffi", []GlyphID{446}},                // ffi ligature
		{"ffl", []GlyphID{447}},                // ffl ligature
		{"office", []GlyphID{83, 446, 71, 73}}, // o + ffi + c + e
	}

	for _, tt := range tests {
		glyphs, _ := shaper.ShapeString(tt.input)

		match := len(glyphs) == len(tt.expected)
		if match {
			for i, g := range glyphs {
				if g != tt.expected[i] {
					match = false
					break
				}
			}
		}

		if match {
			t.Logf("✓ %q: %v", tt.input, glyphs)
		} else {
			t.Errorf("✗ %q: got %v, want %v", tt.input, glyphs, tt.expected)
		}
	}
}

func TestComparePositionsWithHBShape(t *testing.T) {
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

	shaper, err := NewShaper(font)
	if err != nil {
		t.Fatalf("Failed to create shaper: %v", err)
	}

	// Tests with full positioning from hb-shape output
	tests := []struct {
		input    string
		glyphs   []GlyphID
		advances []int16
	}{
		// Hello: [gid44=0+1460|gid73=1+1085|gid80=2+497|gid80=3+497|gid83=4+1168]
		{"Hello", []GlyphID{44, 73, 80, 80, 83}, []int16{1460, 1085, 497, 497, 1168}},
		// fi: [gid444=0+1134]
		{"fi", []GlyphID{444}, []int16{1134}},
		// office: [gid83=0+1168|gid446=1+1748|gid71=4+1072|gid73=5+1085]
		{"office", []GlyphID{83, 446, 71, 73}, []int16{1168, 1748, 1072, 1085}},
		// AV: [gid37=0+1249|gid58=1+1303]
		{"AV", []GlyphID{37, 58}, []int16{1249, 1303}},
		// To: [gid56=0+1123|gid83=1+1168] (T and o have kerning)
		{"To", []GlyphID{56, 83}, []int16{1123, 1168}},
	}

	for _, tt := range tests {
		glyphs, positions := shaper.ShapeString(tt.input)

		// Check glyphs
		glyphMatch := len(glyphs) == len(tt.glyphs)
		if glyphMatch {
			for i, g := range glyphs {
				if g != tt.glyphs[i] {
					glyphMatch = false
					break
				}
			}
		}

		if !glyphMatch {
			t.Errorf("✗ %q glyphs: got %v, want %v", tt.input, glyphs, tt.glyphs)
			continue
		}

		// Check advances
		advMatch := true
		for i, pos := range positions {
			if pos.XAdvance != tt.advances[i] {
				advMatch = false
				t.Errorf("✗ %q[%d] advance: got %d, want %d", tt.input, i, pos.XAdvance, tt.advances[i])
			}
		}

		if advMatch {
			t.Logf("✓ %q: glyphs=%v advances=%v", tt.input, glyphs, tt.advances)
		}
	}
}
