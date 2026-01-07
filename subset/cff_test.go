package subset

import (
	"os"
	"testing"

	"github.com/boxesandglue/textshape/internal/testutil"
	"github.com/boxesandglue/textshape/ot"
)

func TestCFFParsing(t *testing.T) {
	fontPath := testutil.FindTestFont("SourceSansPro-Regular.otf")
	if fontPath == "" {
		t.Skip("SourceSansPro-Regular.otf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ot.ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	if !font.HasTable(ot.TagCFF) {
		t.Fatal("Font does not have CFF table")
	}

	cffData, err := font.TableData(ot.TagCFF)
	if err != nil {
		t.Fatalf("Failed to get CFF table: %v", err)
	}

	cff, err := ot.ParseCFF(cffData)
	if err != nil {
		t.Fatalf("Failed to parse CFF: %v", err)
	}

	t.Logf("Font name: %s", cff.Name)
	t.Logf("Number of glyphs: %d", cff.NumGlyphs())
	t.Logf("Global subroutines: %d", len(cff.GlobalSubrs))
	t.Logf("Local subroutines: %d", len(cff.LocalSubrs))
	t.Logf("CharStrings offset: %d", cff.TopDict.CharStrings)
	t.Logf("Private DICT: size=%d, offset=%d", cff.TopDict.Private[0], cff.TopDict.Private[1])

	if cff.NumGlyphs() == 0 {
		t.Error("Expected non-zero glyph count")
	}
}

func TestCFFSubsetBasic(t *testing.T) {
	fontPath := testutil.FindTestFont("SourceSansPro-Regular.otf")
	if fontPath == "" {
		t.Skip("SourceSansPro-Regular.otf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ot.ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	// Subset to "Hello"
	result, err := SubsetString(font, "Hello")
	if err != nil {
		t.Fatalf("Failed to subset: %v", err)
	}

	t.Logf("Original size: %d bytes", len(data))
	t.Logf("Subset size: %d bytes", len(result))
	t.Logf("Reduction: %.1f%%", 100*(1-float64(len(result))/float64(len(data))))

	// Parse the subset font
	subFont, err := ot.ParseFont(result, 0)
	if err != nil {
		t.Fatalf("Failed to parse subset font: %v", err)
	}

	// Verify CFF table exists
	if !subFont.HasTable(ot.TagCFF) {
		t.Error("Subset font missing CFF table")
	}

	// Parse subset CFF
	subCFFData, err := subFont.TableData(ot.TagCFF)
	if err != nil {
		t.Fatalf("Failed to get subset CFF table: %v", err)
	}

	subCFF, err := ot.ParseCFF(subCFFData)
	if err != nil {
		t.Fatalf("Failed to parse subset CFF: %v", err)
	}

	t.Logf("Subset glyph count: %d", subCFF.NumGlyphs())

	// "Hello" has 4 unique characters (H, e, l, o) + .notdef
	// GSUB closure may add more glyphs (ligatures, alternates, etc.)
	minExpectedGlyphs := 5
	if subCFF.NumGlyphs() < minExpectedGlyphs {
		t.Errorf("Expected at least %d glyphs, got %d", minExpectedGlyphs, subCFF.NumGlyphs())
	}

	// Verify size reduction
	if len(result) >= len(data) {
		t.Error("Subset should be smaller than original")
	}
}

func TestCFFSubsetWithSubroutines(t *testing.T) {
	fontPath := testutil.FindTestFont("SourceSansPro-Regular.otf")
	if fontPath == "" {
		t.Skip("SourceSansPro-Regular.otf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ot.ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	// Get original CFF to check subroutine counts
	origCFFData, _ := font.TableData(ot.TagCFF)
	origCFF, _ := ot.ParseCFF(origCFFData)

	t.Logf("Original global subrs: %d", len(origCFF.GlobalSubrs))
	t.Logf("Original local subrs: %d", len(origCFF.LocalSubrs))

	// Subset to a larger set of characters to test subroutine handling
	result, err := SubsetString(font, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789")
	if err != nil {
		t.Fatalf("Failed to subset: %v", err)
	}

	// Parse subset
	subFont, err := ot.ParseFont(result, 0)
	if err != nil {
		t.Fatalf("Failed to parse subset: %v", err)
	}

	subCFFData, _ := subFont.TableData(ot.TagCFF)
	subCFF, err := ot.ParseCFF(subCFFData)
	if err != nil {
		t.Fatalf("Failed to parse subset CFF: %v", err)
	}

	t.Logf("Subset global subrs: %d", len(subCFF.GlobalSubrs))
	t.Logf("Subset local subrs: %d", len(subCFF.LocalSubrs))
	t.Logf("Subset glyph count: %d", subCFF.NumGlyphs())

	// Subset should have fewer or equal subroutines
	if len(subCFF.GlobalSubrs) > len(origCFF.GlobalSubrs) {
		t.Error("Subset has more global subroutines than original")
	}
	if len(subCFF.LocalSubrs) > len(origCFF.LocalSubrs) {
		t.Error("Subset has more local subroutines than original")
	}

	t.Logf("Original size: %d bytes", len(data))
	t.Logf("Subset size: %d bytes", len(result))
}

func TestCFFCharStringInterpreter(t *testing.T) {
	fontPath := testutil.FindTestFont("SourceSansPro-Regular.otf")
	if fontPath == "" {
		t.Skip("SourceSansPro-Regular.otf not found")
	}

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ot.ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	cffData, _ := font.TableData(ot.TagCFF)
	cff, _ := ot.ParseCFF(cffData)

	// Test interpreter on first few glyphs
	interp := ot.NewCharStringInterpreter(cff.GlobalSubrs, cff.LocalSubrs)

	for i := 0; i < min(10, len(cff.CharStrings)); i++ {
		err := interp.FindUsedSubroutines(cff.CharStrings[i])
		if err != nil {
			t.Errorf("Failed to interpret glyph %d: %v", i, err)
		}
	}

	t.Logf("Used global subrs: %d", len(interp.UsedGlobalSubrs))
	t.Logf("Used local subrs: %d", len(interp.UsedLocalSubrs))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
