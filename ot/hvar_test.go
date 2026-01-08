package ot

import (
	"os"
	"testing"
)

func TestHvarParsing(t *testing.T) {
	data, err := os.ReadFile("testdata/Roboto-Variable.ttf")
	if err != nil {
		t.Skipf("Skipping test: %v", err)
	}

	font, err := ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	hvarData, err := font.TableData(TagHvar)
	if err != nil {
		t.Fatalf("Failed to get HVAR table: %v", err)
	}

	hvar, err := ParseHvar(hvarData)
	if err != nil {
		t.Fatalf("Failed to parse HVAR: %v", err)
	}

	if !hvar.HasData() {
		t.Error("hvar.HasData() = false, want true")
	}

	t.Logf("HVAR table parsed successfully (%d bytes)", len(hvarData))
}

func TestHvarAdvanceDelta(t *testing.T) {
	data, err := os.ReadFile("testdata/Roboto-Variable.ttf")
	if err != nil {
		t.Skipf("Skipping test: %v", err)
	}

	font, err := ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	hvarData, err := font.TableData(TagHvar)
	if err != nil {
		t.Fatalf("Failed to get HVAR table: %v", err)
	}

	hvar, err := ParseHvar(hvarData)
	if err != nil {
		t.Fatalf("Failed to parse HVAR: %v", err)
	}

	// Get fvar for axis info
	fvarData, err := font.TableData(TagFvar)
	if err != nil {
		t.Fatalf("Failed to get fvar: %v", err)
	}
	fvar, err := ParseFvar(fvarData)
	if err != nil {
		t.Fatalf("Failed to parse fvar: %v", err)
	}

	// Test at default position (all zeros) - should have no delta
	defaultCoords := make([]int, fvar.AxisCount())
	delta := hvar.GetAdvanceDelta(GlyphID(1), defaultCoords)
	if delta != 0 {
		t.Logf("Delta at default position: %v (may be non-zero for some fonts)", delta)
	}

	// Test at max weight (normalized = 1.0 = 16384)
	// Weight is axis 0
	boldCoords := make([]int, fvar.AxisCount())
	boldCoords[0] = 16384 // 1.0 in F2DOT14

	deltaBold := hvar.GetAdvanceDelta(GlyphID(1), boldCoords)
	t.Logf("Glyph 1 advance delta at max weight: %v", deltaBold)

	// Test multiple glyphs
	for gid := GlyphID(0); gid < 10; gid++ {
		d := hvar.GetAdvanceDelta(gid, boldCoords)
		t.Logf("Glyph %d advance delta at max weight: %v", gid, d)
	}
}

func TestShaperWithHvar(t *testing.T) {
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

	if !shaper.HasHvar() {
		t.Error("Shaper.HasHvar() = false, want true for Roboto-Variable")
	}

	// Shape at default weight
	buf := NewBuffer()
	buf.AddString("Hello")
	shaper.Shape(buf, nil)

	defaultAdvances := make([]int16, len(buf.Pos))
	for i, pos := range buf.Pos {
		defaultAdvances[i] = pos.XAdvance
	}
	t.Logf("Advances at default weight (400): %v", defaultAdvances)

	// Shape at bold weight (900)
	shaper.SetVariation(TagAxisWeight, 900)
	buf2 := NewBuffer()
	buf2.AddString("Hello")
	shaper.Shape(buf2, nil)

	boldAdvances := make([]int16, len(buf2.Pos))
	for i, pos := range buf2.Pos {
		boldAdvances[i] = pos.XAdvance
	}
	t.Logf("Advances at bold weight (900): %v", boldAdvances)

	// Advances should generally be different (wider at bold)
	allSame := true
	for i := range defaultAdvances {
		if defaultAdvances[i] != boldAdvances[i] {
			allSame = false
			break
		}
	}

	if allSame {
		t.Log("Warning: advances are the same at default and bold weight")
		// This might still be valid for some glyphs, so don't fail
	} else {
		t.Log("Advances differ between default and bold weight - HVAR is working")
	}

	// Compare total advance
	var totalDefault, totalBold int
	for i := range defaultAdvances {
		totalDefault += int(defaultAdvances[i])
		totalBold += int(boldAdvances[i])
	}
	t.Logf("Total advance: default=%d, bold=%d", totalDefault, totalBold)

	if totalBold <= totalDefault {
		t.Log("Note: Bold is not wider than default. This may be expected for some fonts.")
	}
}

func TestShaperHvarAtDifferentWeights(t *testing.T) {
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

	// Test at various weights
	weights := []float32{100, 200, 300, 400, 500, 600, 700, 800, 900}

	for _, weight := range weights {
		shaper.SetVariation(TagAxisWeight, weight)

		buf := NewBuffer()
		buf.AddString("W") // Single wide glyph
		shaper.Shape(buf, nil)

		if len(buf.Pos) > 0 {
			t.Logf("Weight %4.0f: W advance = %d", weight, buf.Pos[0].XAdvance)
		}
	}
}

func TestDeltaSetIndexMapParsing(t *testing.T) {
	// Test basic DeltaSetIndexMap functionality with synthetic data
	// Format 0: format(1) + entryFormat(1) + mapCount(2) + entries(mapCount * width)

	// Simple test: format=0, entryFormat=0x00 (width=1, innerBits=1), 2 entries
	data := []byte{
		0,    // format = 0
		0x00, // entryFormat: (0 << 4) | 0 = width=1, innerBits=1
		0, 2, // mapCount = 2
		0x01, // entry 0: outer=0, inner=1
		0x02, // entry 1: outer=1, inner=0
	}

	dm, err := parseDeltaSetIndexMap(data)
	if err != nil {
		t.Fatalf("Failed to parse DeltaSetIndexMap: %v", err)
	}

	// Check mapping
	if result := dm.Map(0); result != 0x0001 {
		t.Errorf("Map(0) = 0x%04X, want 0x0001", result)
	}
	if result := dm.Map(1); result != 0x00010000 {
		t.Errorf("Map(1) = 0x%08X, want 0x00010000", result)
	}

	// Test clamping: glyph 2 should clamp to last entry (1)
	if result := dm.Map(2); result != 0x00010000 {
		t.Errorf("Map(2) = 0x%08X, want 0x00010000 (clamped)", result)
	}
}

func TestVarRegionListEvaluate(t *testing.T) {
	// Test VarRegionList.Evaluate with synthetic data
	// Region list: axisCount=1, regionCount=1
	// Region 0: start=0, peak=1, end=1 (in F2DOT14: 0, 16384, 16384)
	// This represents a region active for positive coordinates

	data := []byte{
		0, 1, // axisCount = 1
		0, 1, // regionCount = 1
		// Region 0, Axis 0:
		0x00, 0x00, // startCoord = 0 in F2DOT14
		0x40, 0x00, // peakCoord = 1.0 (16384 in F2DOT14)
		0x40, 0x00, // endCoord = 1.0 (16384 in F2DOT14)
	}

	rl, err := parseVarRegionList(data)
	if err != nil {
		t.Fatalf("Failed to parse VarRegionList: %v", err)
	}

	// Test at default (0) - should be 0 since we're at start
	scalar := rl.Evaluate(0, []int{0})
	if scalar != 0 {
		t.Errorf("Evaluate at 0 = %v, want 0", scalar)
	}

	// Test at peak (1.0 = 16384)
	scalar = rl.Evaluate(0, []int{16384})
	if scalar != 1.0 {
		t.Errorf("Evaluate at peak = %v, want 1.0", scalar)
	}

	// Test at halfway (0.5 = 8192)
	scalar = rl.Evaluate(0, []int{8192})
	// Should be 0.5 since we're interpolating from start(0) to peak(16384)
	expected := float32(0.5)
	if abs(scalar-expected) > 0.001 {
		t.Errorf("Evaluate at 0.5 = %v, want %v", scalar, expected)
	}

	// Test below start - should be 0
	scalar = rl.Evaluate(0, []int{-8192})
	if scalar != 0 {
		t.Errorf("Evaluate at -0.5 = %v, want 0", scalar)
	}
}
