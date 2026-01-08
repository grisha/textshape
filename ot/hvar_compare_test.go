package ot

import (
	"fmt"
	"os"
	"testing"
)

// TestHvarCompareWithHarfBuzz compares glyph advances at various weights
// with hb-shape output. This test validates that:
// - HVAR table parsing is correct
// - avar table mapping is applied correctly
// - ItemVariationStore delta computation matches HarfBuzz
func TestHvarCompareWithHarfBuzz(t *testing.T) {
	data, err := os.ReadFile("testdata/Roboto-Variable.ttf")
	if err != nil {
		t.Skipf("Skipping test: %v", err)
	}

	font, err := ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	shaper, err := NewShaper(font)
	if err != nil {
		t.Fatalf("Failed to create shaper: %v", err)
	}

	// Expected values from hb-shape --font-size=2048 --variations="wght=X"
	// Note: hb-shape uses upem scaling, our advances are in font units (upem=2048 for Roboto)
	expected := map[float32][]int16{
		100: {1438, 1032, 422, 422, 1127},
		400: {1461, 1086, 498, 498, 1168},
		700: {1446, 1106, 542, 542, 1156},
		900: {1439, 1116, 563, 563, 1151},
	}

	weights := []float32{100, 400, 700, 900}

	fmt.Println("=== Comparing textshape with hb-shape ===")
	fmt.Println("Text: Hello")
	fmt.Println()

	allMatch := true
	for _, weight := range weights {
		shaper.SetVariation(TagAxisWeight, weight)

		buf := NewBuffer()
		buf.AddString("Hello")
		shaper.Shape(buf, nil)

		fmt.Printf("Weight %.0f:\n", weight)

		// Format like hb-shape
		var parts []string
		for i, info := range buf.Info {
			parts = append(parts, fmt.Sprintf("gid%d=%d+%d", info.GlyphID, info.Cluster, buf.Pos[i].XAdvance))
		}
		fmt.Printf("  textshape: [%s]\n", joinStrings(parts, "|"))

		// Compare with expected
		exp := expected[weight]
		var expParts []string
		for i, info := range buf.Info {
			expParts = append(expParts, fmt.Sprintf("gid%d=%d+%d", info.GlyphID, info.Cluster, exp[i]))
		}
		fmt.Printf("  hb-shape:  [%s]\n", joinStrings(expParts, "|"))

		// Check differences
		match := true
		for i := range buf.Pos {
			if buf.Pos[i].XAdvance != exp[i] {
				match = false
				diff := int(buf.Pos[i].XAdvance) - int(exp[i])
				fmt.Printf("  DIFF at glyph %d: got %d, want %d (diff=%+d)\n",
					i, buf.Pos[i].XAdvance, exp[i], diff)
			}
		}
		if match {
			fmt.Println("  ✓ Match!")
		} else {
			allMatch = false
		}
		fmt.Println()
	}

	if !allMatch {
		t.Error("Some advances don't match hb-shape output")
	}
}

func joinStrings(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += sep + parts[i]
	}
	return result
}
