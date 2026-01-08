package ot

import (
	"os"
	"testing"
)

func TestFvarParsing(t *testing.T) {
	data, err := os.ReadFile("testdata/Roboto-Variable.ttf")
	if err != nil {
		t.Skipf("Skipping test: %v", err)
	}

	font, err := ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	fvarData, err := font.TableData(TagFvar)
	if err != nil {
		t.Fatalf("Failed to get fvar table: %v", err)
	}

	fvar, err := ParseFvar(fvarData)
	if err != nil {
		t.Fatalf("Failed to parse fvar: %v", err)
	}

	// Roboto-Variable should have 2 axes: wght and wdth
	if !fvar.HasData() {
		t.Error("fvar.HasData() = false, want true")
	}

	axisCount := fvar.AxisCount()
	if axisCount != 2 {
		t.Errorf("AxisCount() = %d, want 2", axisCount)
	}

	axes := fvar.AxisInfos()
	if len(axes) != 2 {
		t.Fatalf("len(AxisInfos()) = %d, want 2", len(axes))
	}

	// Check weight axis
	wghtAxis := axes[0]
	if wghtAxis.Tag != TagAxisWeight {
		t.Errorf("axes[0].Tag = %v, want wght", wghtAxis.Tag)
	}
	if wghtAxis.MinValue != 100 {
		t.Errorf("wght.MinValue = %v, want 100", wghtAxis.MinValue)
	}
	if wghtAxis.DefaultValue != 400 {
		t.Errorf("wght.DefaultValue = %v, want 400", wghtAxis.DefaultValue)
	}
	if wghtAxis.MaxValue != 900 {
		t.Errorf("wght.MaxValue = %v, want 900", wghtAxis.MaxValue)
	}

	// Check width axis
	wdthAxis := axes[1]
	if wdthAxis.Tag != TagAxisWidth {
		t.Errorf("axes[1].Tag = %v, want wdth", wdthAxis.Tag)
	}
	if wdthAxis.MinValue != 75 {
		t.Errorf("wdth.MinValue = %v, want 75", wdthAxis.MinValue)
	}
	if wdthAxis.DefaultValue != 100 {
		t.Errorf("wdth.DefaultValue = %v, want 100", wdthAxis.DefaultValue)
	}
	if wdthAxis.MaxValue != 100 {
		t.Errorf("wdth.MaxValue = %v, want 100", wdthAxis.MaxValue)
	}

	// Test FindAxis
	if axis, found := fvar.FindAxis(TagAxisWeight); !found {
		t.Error("FindAxis(wght) returned false")
	} else if axis.Tag != TagAxisWeight {
		t.Errorf("FindAxis(wght).Tag = %v, want wght", axis.Tag)
	}

	if _, found := fvar.FindAxis(TagAxisItalic); found {
		t.Error("FindAxis(ital) should return false for Roboto-Variable")
	}
}

func TestFvarNamedInstances(t *testing.T) {
	data, err := os.ReadFile("testdata/Roboto-Variable.ttf")
	if err != nil {
		t.Skipf("Skipping test: %v", err)
	}

	font, err := ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	fvarData, err := font.TableData(TagFvar)
	if err != nil {
		t.Fatalf("Failed to get fvar table: %v", err)
	}

	fvar, err := ParseFvar(fvarData)
	if err != nil {
		t.Fatalf("Failed to parse fvar: %v", err)
	}

	instances := fvar.NamedInstances()
	if len(instances) == 0 {
		t.Skip("No named instances in font")
	}

	// Check that instances have valid data
	for i, inst := range instances {
		if inst.Index != i {
			t.Errorf("instances[%d].Index = %d, want %d", i, inst.Index, i)
		}
		if len(inst.Coords) != fvar.AxisCount() {
			t.Errorf("instances[%d].Coords has %d values, want %d",
				i, len(inst.Coords), fvar.AxisCount())
		}
	}

	t.Logf("Found %d named instances", len(instances))
}

func TestFvarNormalization(t *testing.T) {
	data, err := os.ReadFile("testdata/Roboto-Variable.ttf")
	if err != nil {
		t.Skipf("Skipping test: %v", err)
	}

	font, err := ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	fvarData, err := font.TableData(TagFvar)
	if err != nil {
		t.Fatalf("Failed to get fvar table: %v", err)
	}

	fvar, err := ParseFvar(fvarData)
	if err != nil {
		t.Fatalf("Failed to parse fvar: %v", err)
	}

	// Weight axis: min=100, default=400, max=900
	// Normalized: 100 -> -1, 400 -> 0, 900 -> 1

	tests := []struct {
		axisIdx int
		value   float32
		want    float32
	}{
		{0, 100, -1.0},  // min
		{0, 400, 0.0},   // default
		{0, 900, 1.0},   // max
		{0, 250, -0.5},  // halfway between min and default
		{0, 650, 0.5},   // halfway between default and max
		{0, 50, -1.0},   // below min, clamped
		{0, 1000, 1.0},  // above max, clamped
	}

	for _, tt := range tests {
		got := fvar.NormalizeAxisValue(tt.axisIdx, tt.value)
		if abs(got-tt.want) > 0.001 {
			t.Errorf("NormalizeAxisValue(%d, %v) = %v, want %v",
				tt.axisIdx, tt.value, got, tt.want)
		}
	}
}

func TestFvarNormalizeVariations(t *testing.T) {
	data, err := os.ReadFile("testdata/Roboto-Variable.ttf")
	if err != nil {
		t.Skipf("Skipping test: %v", err)
	}

	font, err := ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	fvarData, err := font.TableData(TagFvar)
	if err != nil {
		t.Fatalf("Failed to get fvar table: %v", err)
	}

	fvar, err := ParseFvar(fvarData)
	if err != nil {
		t.Fatalf("Failed to parse fvar: %v", err)
	}

	variations := []Variation{
		{Tag: TagAxisWeight, Value: 700}, // Bold
	}

	coords := fvar.NormalizeVariations(variations)
	if len(coords) != 2 {
		t.Fatalf("NormalizeVariations returned %d coords, want 2", len(coords))
	}

	// Weight 700 should normalize to 0.6 (700-400)/(900-400) = 300/500 = 0.6
	if abs(coords[0]-0.6) > 0.001 {
		t.Errorf("coords[0] (wght) = %v, want 0.6", coords[0])
	}

	// Width was not specified, should be 0 (default)
	if coords[1] != 0 {
		t.Errorf("coords[1] (wdth) = %v, want 0", coords[1])
	}
}

func TestFaceFvar(t *testing.T) {
	data, err := os.ReadFile("testdata/Roboto-Variable.ttf")
	if err != nil {
		t.Skipf("Skipping test: %v", err)
	}

	font, err := ParseFont(data, 0)
	if err != nil {
		t.Fatalf("Failed to parse font: %v", err)
	}

	face, err := NewFace(font)
	if err != nil {
		t.Fatalf("Failed to create face: %v", err)
	}

	if !face.HasVariations() {
		t.Error("HasVariations() = false, want true")
	}

	axes := face.VariationAxes()
	if len(axes) != 2 {
		t.Errorf("len(VariationAxes()) = %d, want 2", len(axes))
	}

	if axis, found := face.FindVariationAxis(TagAxisWeight); !found {
		t.Error("FindVariationAxis(wght) = false, want true")
	} else if axis.DefaultValue != 400 {
		t.Errorf("wght.DefaultValue = %v, want 400", axis.DefaultValue)
	}

	instances := face.NamedInstances()
	t.Logf("Face has %d named instances", len(instances))
}

func TestShaperVariations(t *testing.T) {
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

	if !shaper.HasVariations() {
		t.Error("HasVariations() = false, want true")
	}

	// Test default state
	coords := shaper.DesignCoords()
	if len(coords) != 2 {
		t.Fatalf("DesignCoords() has %d values, want 2", len(coords))
	}

	// Default weight should be 400, width 100
	if coords[0] != 400 {
		t.Errorf("Default weight = %v, want 400", coords[0])
	}
	if coords[1] != 100 {
		t.Errorf("Default width = %v, want 100", coords[1])
	}

	// Test SetVariation
	shaper.SetVariation(TagAxisWeight, 700)
	coords = shaper.DesignCoords()
	if coords[0] != 700 {
		t.Errorf("After SetVariation, weight = %v, want 700", coords[0])
	}
	if coords[1] != 100 {
		t.Errorf("Width should remain 100, got %v", coords[1])
	}

	// Test normalized coords
	normalized := shaper.NormalizedCoords()
	// 700 normalizes to 0.6: (700-400)/(900-400) = 300/500 = 0.6
	if abs(normalized[0]-0.6) > 0.001 {
		t.Errorf("Normalized weight = %v, want 0.6", normalized[0])
	}
	if normalized[1] != 0 {
		t.Errorf("Normalized width = %v, want 0", normalized[1])
	}
}

func TestShaperSetVariations(t *testing.T) {
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

	// Set both axes at once
	shaper.SetVariations([]Variation{
		{Tag: TagAxisWeight, Value: 900},
		{Tag: TagAxisWidth, Value: 75},
	})

	coords := shaper.DesignCoords()
	if coords[0] != 900 {
		t.Errorf("Weight = %v, want 900", coords[0])
	}
	if coords[1] != 75 {
		t.Errorf("Width = %v, want 75", coords[1])
	}

	normalized := shaper.NormalizedCoords()
	if normalized[0] != 1.0 {
		t.Errorf("Normalized weight = %v, want 1.0", normalized[0])
	}
	// Width 75 is min, should normalize to -1.0
	// But width axis is 75-100 with default 100, so 75 = (75-100)/(100-75) = -25/25 = -1.0
	if normalized[1] != -1.0 {
		t.Errorf("Normalized width = %v, want -1.0", normalized[1])
	}

	// Reset with only weight
	shaper.SetVariations([]Variation{
		{Tag: TagAxisWeight, Value: 200},
	})

	coords = shaper.DesignCoords()
	if coords[0] != 200 {
		t.Errorf("Weight = %v, want 200", coords[0])
	}
	// Width should reset to default
	if coords[1] != 100 {
		t.Errorf("Width should reset to 100, got %v", coords[1])
	}
}

func TestShaperSetNamedInstance(t *testing.T) {
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

	fvar := shaper.Fvar()
	if fvar == nil {
		t.Fatal("Shaper.Fvar() = nil")
	}

	instances := fvar.NamedInstances()
	if len(instances) == 0 {
		t.Skip("No named instances in font")
	}

	// Set to first instance
	shaper.SetNamedInstance(0)
	coords := shaper.DesignCoords()

	// Verify coords match the instance
	inst := instances[0]
	for i, expected := range inst.Coords {
		if coords[i] != expected {
			t.Errorf("coords[%d] = %v, want %v", i, coords[i], expected)
		}
	}

	t.Logf("Set to instance 0: weight=%v, width=%v", coords[0], coords[1])
}

func TestShaperVariationsClamping(t *testing.T) {
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

	// Try to set weight beyond max (900)
	shaper.SetVariation(TagAxisWeight, 1000)
	coords := shaper.DesignCoords()
	if coords[0] != 900 {
		t.Errorf("Weight should be clamped to 900, got %v", coords[0])
	}

	// Try to set weight below min (100)
	shaper.SetVariation(TagAxisWeight, 50)
	coords = shaper.DesignCoords()
	if coords[0] != 100 {
		t.Errorf("Weight should be clamped to 100, got %v", coords[0])
	}
}

func abs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}
