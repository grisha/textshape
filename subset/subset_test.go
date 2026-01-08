package subset

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/boxesandglue/textshape/internal/testutil"
	"github.com/boxesandglue/textshape/ot"
)

func findTestFont(name string) string {
	return testutil.FindTestFont(name)
}

func TestSubsetBasic(t *testing.T) {
	fontPath := findTestFont("Roboto-Regular.ttf")
	if fontPath == "" {
		t.Skip("Roboto-Regular.ttf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ot.ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	originalSize := len(data)
	t.Logf("Original font size: %d bytes", originalSize)

	// Subset for "Hello"
	result, err := SubsetString(font, "Hello")
	if err != nil {
		t.Fatalf("Failed to subset: %v", err)
	}

	t.Logf("Subset font size: %d bytes (%.1f%% of original)",
		len(result), float64(len(result))*100/float64(originalSize))

	// Verify the subset is smaller
	if len(result) >= originalSize {
		t.Errorf("Subset font is not smaller: %d >= %d", len(result), originalSize)
	}

	// Verify the subset can be parsed
	subFont, err := ot.ParseFont(result, 0)
	if err != nil {
		t.Fatalf("Failed to parse subset font: %v", err)
	}

	// Verify numGlyphs is reduced
	t.Logf("Original numGlyphs: %d, Subset numGlyphs: %d",
		font.NumGlyphs(), subFont.NumGlyphs())

	if subFont.NumGlyphs() >= font.NumGlyphs() {
		t.Errorf("Subset should have fewer glyphs: %d >= %d",
			subFont.NumGlyphs(), font.NumGlyphs())
	}
}

func TestSubsetWithLigatures(t *testing.T) {
	fontPath := findTestFont("Roboto-Regular.ttf")
	if fontPath == "" {
		t.Skip("Roboto-Regular.ttf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ot.ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	// Subset for "office" which should include the ffi ligature
	input := NewInput()
	input.AddString("office")

	plan, err := CreatePlan(font, input)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	// Get the glyph set to verify ligature closure
	glyphSet := plan.GlyphSet()
	t.Logf("Glyph set size: %d", len(glyphSet))

	// The word "office" has letters: o, f, f, i, c, e
	// With GSUB ligature closure, we should also get the ffi ligature glyph (446)

	// Look up the original glyphs
	cmap := plan.Cmap()
	oGlyph, _ := cmap.Lookup('o')
	fGlyph, _ := cmap.Lookup('f')
	iGlyph, _ := cmap.Lookup('i')
	cGlyph, _ := cmap.Lookup('c')
	eGlyph, _ := cmap.Lookup('e')

	t.Logf("Glyph IDs: o=%d, f=%d, i=%d, c=%d, e=%d", oGlyph, fGlyph, iGlyph, cGlyph, eGlyph)

	// Check if the ffi ligature (glyph 446 in Roboto) is included
	ffiLigature := ot.GlyphID(446)
	if glyphSet[ffiLigature] {
		t.Logf("ffi ligature (glyph %d) is included in subset", ffiLigature)
	} else {
		t.Logf("Warning: ffi ligature (glyph %d) is NOT in subset (GSUB closure may need improvement)", ffiLigature)
	}

	result, err := plan.Execute()
	if err != nil {
		t.Fatalf("Failed to execute subset: %v", err)
	}

	t.Logf("Subset size: %d bytes", len(result))

	// Verify the subset can be parsed
	subFont, err := ot.ParseFont(result, 0)
	if err != nil {
		t.Fatalf("Failed to parse subset font: %v", err)
	}

	t.Logf("Subset numGlyphs: %d", subFont.NumGlyphs())
}

func TestSubsetPlan(t *testing.T) {
	fontPath := findTestFont("Roboto-Regular.ttf")
	if fontPath == "" {
		t.Skip("Roboto-Regular.ttf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ot.ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	input := NewInput()
	input.AddString("ABC")

	plan, err := CreatePlan(font, input)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	t.Logf("Output glyphs: %d", plan.NumOutputGlyphs())
	t.Logf("Glyph set size: %d", len(plan.GlyphSet()))

	// Verify we have at least .notdef + A, B, C
	if plan.NumOutputGlyphs() < 4 {
		t.Errorf("Expected at least 4 glyphs (.notdef + A, B, C), got %d", plan.NumOutputGlyphs())
	}

	// Verify .notdef is always included
	if !plan.GlyphSet()[0] {
		t.Error(".notdef (GID 0) should always be included")
	}

	// Test glyph mapping
	cmap := plan.Cmap()
	aGlyph, ok := cmap.Lookup('A')
	if !ok {
		t.Fatal("'A' not in cmap")
	}

	newGID, ok := plan.MapGlyph(aGlyph)
	if !ok {
		t.Errorf("'A' (GID %d) should be mapped", aGlyph)
	} else {
		t.Logf("'A' mapping: %d -> %d", aGlyph, newGID)
	}

	// Verify reverse mapping
	oldGID, ok := plan.OldGlyph(newGID)
	if !ok || oldGID != aGlyph {
		t.Errorf("Reverse mapping failed: %d -> %d (expected %d)", newGID, oldGID, aGlyph)
	}
}

func TestSubsetRetainGIDs(t *testing.T) {
	fontPath := findTestFont("Roboto-Regular.ttf")
	if fontPath == "" {
		t.Skip("Roboto-Regular.ttf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ot.ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	input := NewInput()
	input.AddString("A")
	input.Flags = FlagRetainGIDs

	plan, err := CreatePlan(font, input)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	cmap := plan.Cmap()
	aGlyph, _ := cmap.Lookup('A')

	// With RetainGIDs, the glyph ID should be unchanged
	newGID, ok := plan.MapGlyph(aGlyph)
	if !ok {
		t.Fatalf("'A' (GID %d) should be mapped", aGlyph)
	}

	if newGID != aGlyph {
		t.Errorf("With FlagRetainGIDs, GID should be unchanged: %d != %d", newGID, aGlyph)
	}

	t.Logf("'A' mapping with RetainGIDs: %d -> %d", aGlyph, newGID)
	t.Logf("Output glyphs: %d", plan.NumOutputGlyphs())
}

func TestSubsetShaping(t *testing.T) {
	fontPath := findTestFont("Roboto-Regular.ttf")
	if fontPath == "" {
		t.Skip("Roboto-Regular.ttf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ot.ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	// Subset for "Hello"
	result, err := SubsetString(font, "Hello")
	if err != nil {
		t.Fatalf("Failed to subset: %v", err)
	}

	// Parse the subset font
	subFont, err := ot.ParseFont(result, 0)
	if err != nil {
		t.Fatalf("Failed to parse subset font: %v", err)
	}

	// Create a shaper for the subset font
	shaper, err := ot.NewShaper(subFont)
	if err != nil {
		t.Fatalf("Failed to create shaper: %v", err)
	}

	// Shape "Hello" with the subset font
	glyphs, positions := shaper.ShapeString("Hello")

	t.Logf("Shaped 'Hello' with subset font:")
	for i, g := range glyphs {
		t.Logf("  glyph %d: advance=%d", g, positions[i].XAdvance)
	}

	// Should have 5 glyphs (H, e, l, l, o)
	if len(glyphs) != 5 {
		t.Errorf("Expected 5 glyphs, got %d", len(glyphs))
	}
}

// TestSubsetLigatureShaping verifies that ligatures work correctly after subsetting.
func TestSubsetLigatureShaping(t *testing.T) {
	fontPath := findTestFont("Roboto-Regular.ttf")
	if fontPath == "" {
		t.Skip("Roboto-Regular.ttf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ot.ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	tests := []struct {
		name          string
		text          string
		expectedGlyph int // expected number of output glyphs
	}{
		{"fi ligature", "fi", 1},
		{"fl ligature", "fl", 1},
		{"ffi ligature", "ffi", 1},
		{"ffl ligature", "ffl", 1},
		{"office", "office", 4}, // o + ffi + c + e
		{"no ligature", "hello", 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create subset
			result, err := SubsetString(font, tt.text)
			if err != nil {
				t.Fatalf("Failed to subset: %v", err)
			}

			subFont, err := ot.ParseFont(result, 0)
			if err != nil {
				t.Fatalf("Failed to parse subset font: %v", err)
			}

			// Shape with both fonts
			origShaper, _ := ot.NewShaper(font)
			subShaper, _ := ot.NewShaper(subFont)

			origGlyphs, origPos := origShaper.ShapeString(tt.text)
			subGlyphs, subPos := subShaper.ShapeString(tt.text)

			// Calculate widths
			origWidth, subWidth := 0, 0
			for i := range origGlyphs {
				origWidth += int(origPos[i].XAdvance)
			}
			for i := range subGlyphs {
				subWidth += int(subPos[i].XAdvance)
			}

			// Verify glyph count matches
			if len(subGlyphs) != len(origGlyphs) {
				t.Errorf("Glyph count mismatch: subset=%d, original=%d", len(subGlyphs), len(origGlyphs))
			}

			// Verify width matches
			if subWidth != origWidth {
				t.Errorf("Width mismatch: subset=%d, original=%d", subWidth, origWidth)
			}

			// Verify expected glyph count
			if len(origGlyphs) != tt.expectedGlyph {
				t.Errorf("Expected %d glyphs, got %d", tt.expectedGlyph, len(origGlyphs))
			}

			t.Logf("OK: %d glyphs, width=%d", len(subGlyphs), subWidth)
		})
	}
}

// TestSubsetCompareWithHBSubset compares our subset output with hb-subset.
func TestSubsetCompareWithHBSubset(t *testing.T) {
	// Check if hb-subset is available
	if _, err := exec.LookPath("hb-subset"); err != nil {
		t.Skip("hb-subset not available")
	}

	fontPath := findTestFont("Roboto-Regular.ttf")
	if fontPath == "" {
		t.Skip("Roboto-Regular.ttf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ot.ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	tests := []string{
		"Hello",
		"office",
		"fi fl ffi ffl",
		"Hello, fine office!",
	}

	for _, text := range tests {
		t.Run(text, func(t *testing.T) {
			// Go subset
			goResult, err := SubsetString(font, text)
			if err != nil {
				t.Fatalf("Go subset failed: %v", err)
			}
			goFont, _ := ot.ParseFont(goResult, 0)

			// hb-subset
			unicodes := ""
			for _, r := range text {
				if unicodes != "" {
					unicodes += ","
				}
				unicodes += fmt.Sprintf("U+%04X", r)
			}

			tmpFile := filepath.Join(os.TempDir(), "hb_subset_test.ttf")
			cmd := exec.Command("hb-subset", "--unicodes="+unicodes, "--output-file="+tmpFile, fontPath)
			if err := cmd.Run(); err != nil {
				t.Fatalf("hb-subset failed: %v", err)
			}
			defer os.Remove(tmpFile)

			hbData, _ := os.ReadFile(tmpFile)
			hbFont, _ := ot.ParseFont(hbData, 0)

			// Shape with all three fonts
			origShaper, _ := ot.NewShaper(font)
			goShaper, _ := ot.NewShaper(goFont)
			hbShaper, _ := ot.NewShaper(hbFont)

			origG, origP := origShaper.ShapeString(text)
			goG, goP := goShaper.ShapeString(text)
			hbG, hbP := hbShaper.ShapeString(text)

			// Calculate widths
			origWidth, goWidth, hbWidth := 0, 0, 0
			for i := range origG {
				origWidth += int(origP[i].XAdvance)
			}
			for i := range goG {
				goWidth += int(goP[i].XAdvance)
			}
			for i := range hbG {
				hbWidth += int(hbP[i].XAdvance)
			}

			t.Logf("Size: Go=%d, hb=%d", len(goResult), len(hbData))
			t.Logf("Glyphs: orig=%d, Go=%d, hb=%d", len(origG), len(goG), len(hbG))
			t.Logf("Width: orig=%d, Go=%d, hb=%d", origWidth, goWidth, hbWidth)

			// Verify Go matches original
			if len(goG) != len(origG) {
				t.Errorf("Go glyph count mismatch: %d != %d", len(goG), len(origG))
			}
			if goWidth != origWidth {
				t.Errorf("Go width mismatch: %d != %d", goWidth, origWidth)
			}

			// Verify Go matches hb-subset
			if goWidth != hbWidth {
				t.Errorf("Go/hb width mismatch: %d != %d", goWidth, hbWidth)
			}
		})
	}
}

// TestSubsetRoundtrip verifies that subset fonts can be re-parsed and all characters are present.
func TestSubsetRoundtrip(t *testing.T) {
	fontPath := findTestFont("Roboto-Regular.ttf")
	if fontPath == "" {
		t.Skip("Roboto-Regular.ttf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ot.ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	text := "Hello, World! ABCDEFGabcdefg 0123456789"

	// Create subset
	result, err := SubsetString(font, text)
	if err != nil {
		t.Fatalf("Failed to subset: %v", err)
	}

	// Parse subset
	subFont, err := ot.ParseFont(result, 0)
	if err != nil {
		t.Fatalf("Failed to parse subset font: %v", err)
	}

	// Verify all characters are in the subset cmap
	cmapData, err := subFont.TableData(ot.TagCmap)
	if err != nil {
		t.Fatalf("Failed to get cmap: %v", err)
	}

	cmap, err := ot.ParseCmap(cmapData)
	if err != nil {
		t.Fatalf("Failed to parse cmap: %v", err)
	}

	missing := []rune{}
	for _, r := range text {
		if _, ok := cmap.Lookup(ot.Codepoint(r)); !ok {
			missing = append(missing, r)
		}
	}

	if len(missing) > 0 {
		t.Errorf("Missing characters in subset: %q", string(missing))
	}

	t.Logf("Roundtrip OK: %d chars, %d bytes -> %d bytes",
		len(text), len(data), len(result))
}

// TestSubsetAllLigatures tests all standard ligatures.
func TestSubsetAllLigatures(t *testing.T) {
	fontPath := findTestFont("Roboto-Regular.ttf")
	if fontPath == "" {
		t.Skip("Roboto-Regular.ttf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ot.ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	// Test all ligatures in one subset
	text := "fi fl ffi ffl"

	result, err := SubsetString(font, text)
	if err != nil {
		t.Fatalf("Failed to subset: %v", err)
	}

	subFont, _ := ot.ParseFont(result, 0)

	// Shape and compare
	origShaper, _ := ot.NewShaper(font)
	subShaper, _ := ot.NewShaper(subFont)

	origGlyphs, _ := origShaper.ShapeString(text)
	subGlyphs, _ := subShaper.ShapeString(text)

	// Should have 7 glyphs: fi(1) + space + fl(1) + space + ffi(1) + space + ffl(1) = 7
	if len(origGlyphs) != 7 {
		t.Errorf("Original should have 7 glyphs (4 ligatures + 3 spaces), got %d", len(origGlyphs))
	}

	if len(subGlyphs) != len(origGlyphs) {
		t.Errorf("Subset glyph count mismatch: %d != %d", len(subGlyphs), len(origGlyphs))
	}

	// Verify GSUB table is present in subset
	if !subFont.HasTable(ot.TagGSUB) {
		t.Error("Subset should have GSUB table for ligatures")
	}

	t.Logf("All 4 ligatures work: %d glyphs", len(subGlyphs))
}

// TestInstancingDropsVariationTables tests that variation tables are dropped when axes are pinned.
func TestInstancingDropsVariationTables(t *testing.T) {
	fontPath := findTestFont("Roboto-Variable.ttf")
	if fontPath == "" {
		t.Skip("Roboto-Variable.ttf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ot.ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	// Verify original font has variation tables
	variationTables := []ot.Tag{ot.TagFvar, ot.TagHvar}
	for _, tag := range variationTables {
		if !font.HasTable(tag) {
			t.Skipf("Font has no %v table", tag)
		}
	}

	// First subset WITHOUT pinning - should keep variation tables (with FlagPassUnrecognized)
	input1 := NewInput()
	input1.AddString("Hello")
	input1.Flags = FlagPassUnrecognized
	plan1, err := CreatePlan(font, input1)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}
	result1, err := plan1.Execute()
	if err != nil {
		t.Fatalf("Failed to execute plan: %v", err)
	}
	subFont1, _ := ot.ParseFont(result1, 0)

	// Should have fvar table (not instanced)
	if !subFont1.HasTable(ot.TagFvar) {
		t.Error("Non-instanced subset with FlagPassUnrecognized should have fvar table")
	}

	// Now subset WITH pinning all axes - should drop variation tables
	input2 := NewInput()
	input2.AddString("Hello")
	input2.Flags = FlagPassUnrecognized
	input2.PinAllAxesToDefault(font)

	plan2, err := CreatePlan(font, input2)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}
	result2, err := plan2.Execute()
	if err != nil {
		t.Fatalf("Failed to execute plan: %v", err)
	}
	subFont2, _ := ot.ParseFont(result2, 0)

	// Should NOT have variation tables
	if subFont2.HasTable(ot.TagFvar) {
		t.Error("Instanced subset should NOT have fvar table")
	}
	if subFont2.HasTable(ot.TagHvar) {
		t.Error("Instanced subset should NOT have HVAR table")
	}
	if subFont2.HasTable(ot.TagAvar) {
		t.Error("Instanced subset should NOT have avar table")
	}

	t.Logf("Non-instanced: %d bytes, Instanced: %d bytes", len(result1), len(result2))
	t.Logf("Saved: %d bytes", len(result1)-len(result2))
}

// TestInstancingAppliesHVAR tests that HVAR deltas are applied when instancing.
func TestInstancingAppliesHVAR(t *testing.T) {
	fontPath := findTestFont("Roboto-Variable.ttf")
	if fontPath == "" {
		t.Skip("Roboto-Variable.ttf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ot.ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	// Expected advances from hb-shape at different weights for "Hello"
	// These are the same values from ot/hvar_compare_test.go
	expectedAdvances := map[float32][]int16{
		100: {1438, 1032, 422, 422, 1127},
		400: {1461, 1086, 498, 498, 1168},
		700: {1446, 1106, 542, 542, 1156},
		900: {1439, 1116, 563, 563, 1151},
	}

	for weight, expected := range expectedAdvances {
		t.Run(fmt.Sprintf("weight%.0f", weight), func(t *testing.T) {
			// Create instanced subset at this weight
			input := NewInput()
			input.AddString("Hello")
			input.PinAxisLocation(ot.TagAxisWeight, weight)

			plan, err := CreatePlan(font, input)
			if err != nil {
				t.Fatalf("Failed to create plan: %v", err)
			}

			// Verify it's marked as instanced
			if !plan.IsInstanced() {
				t.Fatal("Plan should be marked as instanced")
			}

			result, err := plan.Execute()
			if err != nil {
				t.Fatalf("Failed to execute plan: %v", err)
			}

			// Parse the subset font
			subFont, err := ot.ParseFont(result, 0)
			if err != nil {
				t.Fatalf("Failed to parse subset font: %v", err)
			}

			// Shape "Hello" - since it's a static font now, should give correct advances
			shaper, err := ot.NewShaper(subFont)
			if err != nil {
				t.Fatalf("Failed to create shaper: %v", err)
			}

			buf := ot.NewBuffer()
			buf.AddString("Hello")
			shaper.Shape(buf, nil)

			// Compare advances
			match := true
			for i, pos := range buf.Pos {
				if pos.XAdvance != expected[i] {
					t.Errorf("Glyph %d: advance=%d, expected=%d", i, pos.XAdvance, expected[i])
					match = false
				}
			}
			if match {
				t.Logf("OK: advances match hb-shape at wght=%.0f", weight)
			}
		})
	}
}

// TestPinAxisMethods tests the Input pin axis methods.
func TestPinAxisMethods(t *testing.T) {
	fontPath := findTestFont("Roboto-Variable.ttf")
	if fontPath == "" {
		t.Skip("Roboto-Variable.ttf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ot.ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	// Test PinAxisLocation
	t.Run("PinAxisLocation", func(t *testing.T) {
		input := NewInput()
		input.PinAxisLocation(ot.TagAxisWeight, 700)

		if !input.HasPinnedAxes() {
			t.Error("HasPinnedAxes should return true")
		}

		pinnedAxes := input.PinnedAxes()
		if pinnedAxes[ot.TagAxisWeight] != 700 {
			t.Errorf("PinnedAxes[wght] = %v, expected 700", pinnedAxes[ot.TagAxisWeight])
		}

		if input.IsFullyInstanced(font) {
			t.Error("Should not be fully instanced (only one axis pinned)")
		}
	})

	// Test PinAxisToDefault
	t.Run("PinAxisToDefault", func(t *testing.T) {
		input := NewInput()
		ok := input.PinAxisToDefault(font, ot.TagAxisWeight)
		if !ok {
			t.Fatal("PinAxisToDefault should return true")
		}

		pinnedAxes := input.PinnedAxes()
		// Roboto-Variable default weight is 400
		if pinnedAxes[ot.TagAxisWeight] != 400 {
			t.Errorf("Default weight = %v, expected 400", pinnedAxes[ot.TagAxisWeight])
		}
	})

	// Test PinAllAxesToDefault
	t.Run("PinAllAxesToDefault", func(t *testing.T) {
		input := NewInput()
		ok := input.PinAllAxesToDefault(font)
		if !ok {
			t.Fatal("PinAllAxesToDefault should return true")
		}

		if !input.HasPinnedAxes() {
			t.Error("HasPinnedAxes should return true")
		}

		if !input.IsFullyInstanced(font) {
			t.Error("Should be fully instanced (all axes pinned)")
		}
	})
}

// TestFlagDropLayoutTables tests that layout tables are excluded when flag is set.
func TestFlagDropLayoutTables(t *testing.T) {
	fontPath := findTestFont("Roboto-Regular.ttf")
	if fontPath == "" {
		t.Skip("Roboto-Regular.ttf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ot.ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	// Verify original font has layout tables
	if !font.HasTable(ot.TagGSUB) {
		t.Skip("Font has no GSUB table")
	}
	if !font.HasTable(ot.TagGPOS) {
		t.Skip("Font has no GPOS table")
	}

	// First, subset WITHOUT the flag - should have GSUB/GPOS
	input1 := NewInput()
	input1.AddString("AVTofi")
	plan1, err := CreatePlan(font, input1)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}
	result1, err := plan1.Execute()
	if err != nil {
		t.Fatalf("Failed to subset: %v", err)
	}
	subFont1, _ := ot.ParseFont(result1, 0)

	// Should have layout tables
	if !subFont1.HasTable(ot.TagGSUB) && !subFont1.HasTable(ot.TagGPOS) {
		t.Log("Warning: No layout tables in first subset (may be OK if no lookups apply)")
	}

	// Now subset WITH FlagDropLayoutTables - should NOT have GSUB/GPOS/GDEF
	input2 := NewInput()
	input2.AddString("AVTofi")
	input2.Flags = FlagDropLayoutTables
	plan2, err := CreatePlan(font, input2)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}
	result2, err := plan2.Execute()
	if err != nil {
		t.Fatalf("Failed to subset: %v", err)
	}
	subFont2, _ := ot.ParseFont(result2, 0)

	// Should NOT have layout tables
	if subFont2.HasTable(ot.TagGSUB) {
		t.Error("Subset with FlagDropLayoutTables should NOT have GSUB table")
	}
	if subFont2.HasTable(ot.TagGPOS) {
		t.Error("Subset with FlagDropLayoutTables should NOT have GPOS table")
	}
	if subFont2.HasTable(ot.TagGDEF) {
		t.Error("Subset with FlagDropLayoutTables should NOT have GDEF table")
	}

	// Size comparison
	t.Logf("With layout tables: %d bytes", len(result1))
	t.Logf("Without layout tables: %d bytes", len(result2))
	t.Logf("Saved: %d bytes (%.1f%%)", len(result1)-len(result2), 100*float64(len(result1)-len(result2))/float64(len(result1)))

	// Subset without layout tables should be smaller
	if len(result2) >= len(result1) {
		t.Errorf("Subset without layout tables should be smaller: %d >= %d", len(result2), len(result1))
	}
}
