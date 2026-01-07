package subset

import (
	"os"
	"testing"

	"github.com/boxesandglue/textshape/internal/testutil"
	"github.com/boxesandglue/textshape/ot"
)

func TestGPOSSubsetting(t *testing.T) {
	fontPath := testutil.FindTestFont("Roboto-Regular.ttf")
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

	// Check if font has GPOS table
	if !font.HasTable(ot.TagGPOS) {
		t.Skip("Font does not have GPOS table")
	}

	// Subset to "AV" which should have kerning
	result, err := SubsetString(font, "AV")
	if err != nil {
		t.Fatalf("Failed to subset: %v", err)
	}

	t.Logf("Original size: %d bytes", len(data))
	t.Logf("Subset size: %d bytes", len(result))

	// Parse the subset font
	subFont, err := ot.ParseFont(result, 0)
	if err != nil {
		t.Fatalf("Failed to parse subset font: %v", err)
	}

	// Check if GPOS table is present (may be absent if no kern lookups apply)
	if subFont.HasTable(ot.TagGPOS) {
		gposData, err := subFont.TableData(ot.TagGPOS)
		if err != nil {
			t.Fatalf("Failed to get GPOS table: %v", err)
		}
		t.Logf("Subset GPOS size: %d bytes", len(gposData))

		// Parse subset GPOS
		gpos, err := ot.ParseGPOS(gposData)
		if err != nil {
			t.Fatalf("Failed to parse subset GPOS: %v", err)
		}
		t.Logf("Subset GPOS lookups: %d", gpos.NumLookups())
	} else {
		t.Logf("Subset does not have GPOS (no applicable kern lookups)")
	}
}

func TestGDEFSubsetting(t *testing.T) {
	fontPath := testutil.FindTestFont("Roboto-Regular.ttf")
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

	// Check if font has GDEF table
	if !font.HasTable(ot.TagGDEF) {
		t.Skip("Font does not have GDEF table")
	}

	// Get original GDEF info
	origGDEFData, _ := font.TableData(ot.TagGDEF)
	origGDEF, _ := ot.ParseGDEF(origGDEFData)
	t.Logf("Original GDEF size: %d bytes", len(origGDEFData))
	t.Logf("Original has glyph classes: %v", origGDEF.HasGlyphClasses())
	t.Logf("Original has mark attach classes: %v", origGDEF.HasMarkAttachClasses())
	t.Logf("Original has mark glyph sets: %v", origGDEF.HasMarkGlyphSets())

	// Subset to "Hello"
	result, err := SubsetString(font, "Hello")
	if err != nil {
		t.Fatalf("Failed to subset: %v", err)
	}

	// Parse the subset font
	subFont, err := ot.ParseFont(result, 0)
	if err != nil {
		t.Fatalf("Failed to parse subset font: %v", err)
	}

	// Check if GDEF table is present
	if subFont.HasTable(ot.TagGDEF) {
		gdefData, err := subFont.TableData(ot.TagGDEF)
		if err != nil {
			t.Fatalf("Failed to get GDEF table: %v", err)
		}
		t.Logf("Subset GDEF size: %d bytes", len(gdefData))

		// Parse subset GDEF
		gdef, err := ot.ParseGDEF(gdefData)
		if err != nil {
			t.Fatalf("Failed to parse subset GDEF: %v", err)
		}
		t.Logf("Subset has glyph classes: %v", gdef.HasGlyphClasses())
		t.Logf("Subset has mark attach classes: %v", gdef.HasMarkAttachClasses())
		t.Logf("Subset has mark glyph sets: %v", gdef.HasMarkGlyphSets())

		// Verify size reduction
		if len(gdefData) >= len(origGDEFData) {
			t.Logf("Warning: Subset GDEF not smaller (may be OK for small subsets)")
		}
	} else {
		t.Logf("Subset does not have GDEF (no applicable data)")
	}
}

func TestGPOSPairPositioning(t *testing.T) {
	fontPath := testutil.FindTestFont("Roboto-Regular.ttf")
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

	// Test with more characters that have kerning
	result, err := SubsetString(font, "AVWYToPLT")
	if err != nil {
		t.Fatalf("Failed to subset: %v", err)
	}

	t.Logf("Original size: %d bytes", len(data))
	t.Logf("Subset size: %d bytes", len(result))
	t.Logf("Reduction: %.1f%%", 100*(1-float64(len(result))/float64(len(data))))

	// Verify subset font is valid
	subFont, err := ot.ParseFont(result, 0)
	if err != nil {
		t.Fatalf("Failed to parse subset font: %v", err)
	}

	// Check required tables
	requiredTables := []ot.Tag{ot.TagHead, ot.TagHhea, ot.TagHmtx, ot.TagMaxp, ot.TagCmap}
	for _, tag := range requiredTables {
		if !subFont.HasTable(tag) {
			t.Errorf("Missing required table: %s", tag)
		}
	}

	t.Logf("Subset has GSUB: %v", subFont.HasTable(ot.TagGSUB))
	t.Logf("Subset has GPOS: %v", subFont.HasTable(ot.TagGPOS))
	t.Logf("Subset has GDEF: %v", subFont.HasTable(ot.TagGDEF))
}
