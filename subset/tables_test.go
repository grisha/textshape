package subset

import (
	"os"
	"testing"

	"github.com/boxesandglue/textshape/ot"
)

func TestSubsetPDFTables(t *testing.T) {
	fontPath := findTestFont("Roboto-Regular.ttf")
	if fontPath == "" {
		t.Skip("Roboto-Regular.ttf not found")
	}

	data, err := readFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to read font: %v", err)
	}

	font, err := ot.ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	result, err := SubsetString(font, "Hello")
	if err != nil {
		t.Fatalf("Failed to subset: %v", err)
	}

	subFont, err := ot.ParseFont(result, 0)
	if err != nil {
		t.Fatalf("Failed to parse subset: %v", err)
	}

	t.Logf("Subset size: %d bytes", len(result))

	// PDF-required tables for TrueType
	required := []struct {
		tag  ot.Tag
		name string
	}{
		{ot.TagHead, "head"},
		{ot.TagHhea, "hhea"},
		{ot.TagMaxp, "maxp"},
		{ot.TagHmtx, "hmtx"},
		{ot.TagLoca, "loca"},
		{ot.TagGlyf, "glyf"},
		{ot.TagCmap, "cmap"},
		{ot.TagCvt, "cvt "},
		{ot.TagFpgm, "fpgm"},
		{ot.TagPrep, "prep"},
	}

	for _, r := range required {
		if subFont.HasTable(r.tag) {
			t.Logf("✓ %s present", r.name)
		} else {
			t.Errorf("✗ %s missing (required by PDF spec)", r.name)
		}
	}

	// Optional tables
	optional := []struct {
		tag  ot.Tag
		name string
	}{
		{ot.TagOS2, "OS/2"},
		{ot.TagGSUB, "GSUB"},
	}

	for _, o := range optional {
		if subFont.HasTable(o.tag) {
			t.Logf("✓ %s present (optional)", o.name)
		} else {
			t.Logf("- %s not present (optional)", o.name)
		}
	}
}

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
