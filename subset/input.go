// Package subset provides font subsetting functionality.
package subset

import "github.com/boxesandglue/textshape/ot"

// Input configures which glyphs and tables to include in the subset.
type Input struct {
	// Unicodes specifies Unicode codepoints to retain.
	unicodes map[rune]bool

	// Glyphs specifies explicit glyph IDs to retain.
	glyphs map[ot.GlyphID]bool

	// DropTables specifies tables to exclude from output.
	dropTables map[ot.Tag]bool

	// PassThroughTables specifies tables to copy unchanged.
	passThroughTables map[ot.Tag]bool

	// LayoutFeatures specifies OpenType features to retain.
	// If empty, all features are retained.
	layoutFeatures map[ot.Tag]bool

	// pinnedAxes maps axis tags to pinned values (design-space coordinates).
	// When axes are pinned, the font is instanced (variation tables removed).
	pinnedAxes map[ot.Tag]float32

	// Flags controls subsetting behavior.
	Flags Flags
}

// Flags controls various subsetting options.
type Flags uint32

const (
	// FlagNoHinting removes hinting instructions.
	FlagNoHinting Flags = 1 << iota

	// FlagRetainGIDs keeps original glyph IDs (pads with empty glyphs).
	FlagRetainGIDs

	// FlagGlyphNames retains PostScript glyph names.
	FlagGlyphNames

	// FlagNotdefOutline retains the .notdef glyph outline.
	FlagNotdefOutline

	// FlagNoLayoutClosure skips GSUB/GPOS glyph closure.
	FlagNoLayoutClosure

	// FlagPassUnrecognized keeps unrecognized tables.
	FlagPassUnrecognized

	// FlagDropLayoutTables excludes GSUB/GPOS/GDEF tables from output.
	// Use this for PDF embedding where shaping is already done.
	FlagDropLayoutTables
)

// NewInput creates a new subset input configuration.
func NewInput() *Input {
	return &Input{
		unicodes:          make(map[rune]bool),
		glyphs:            make(map[ot.GlyphID]bool),
		dropTables:        make(map[ot.Tag]bool),
		passThroughTables: make(map[ot.Tag]bool),
		layoutFeatures:    make(map[ot.Tag]bool),
		pinnedAxes:        make(map[ot.Tag]float32),
	}
}

// AddUnicode adds a Unicode codepoint to retain.
func (i *Input) AddUnicode(cp rune) {
	i.unicodes[cp] = true
}

// AddUnicodes adds multiple Unicode codepoints.
func (i *Input) AddUnicodes(cps ...rune) {
	for _, cp := range cps {
		i.unicodes[cp] = true
	}
}

// AddUnicodeRange adds a range of Unicode codepoints [start, end].
func (i *Input) AddUnicodeRange(start, end rune) {
	for cp := start; cp <= end; cp++ {
		i.unicodes[cp] = true
	}
}

// AddString adds all codepoints from a string.
func (i *Input) AddString(s string) {
	for _, cp := range s {
		i.unicodes[cp] = true
	}
}

// AddGlyph adds a glyph ID to retain.
func (i *Input) AddGlyph(gid ot.GlyphID) {
	i.glyphs[gid] = true
}

// AddGlyphs adds multiple glyph IDs.
func (i *Input) AddGlyphs(gids ...ot.GlyphID) {
	for _, gid := range gids {
		i.glyphs[gid] = true
	}
}

// DropTable marks a table to be excluded from output.
func (i *Input) DropTable(tag ot.Tag) {
	i.dropTables[tag] = true
}

// PassThroughTable marks a table to be copied unchanged.
func (i *Input) PassThroughTable(tag ot.Tag) {
	i.passThroughTables[tag] = true
}

// KeepFeature marks an OpenType feature to retain.
// If no features are specified, all features are retained.
func (i *Input) KeepFeature(tag ot.Tag) {
	i.layoutFeatures[tag] = true
}

// Unicodes returns the set of Unicode codepoints to retain.
func (i *Input) Unicodes() map[rune]bool {
	return i.unicodes
}

// Glyphs returns the set of glyph IDs to retain.
func (i *Input) Glyphs() map[ot.GlyphID]bool {
	return i.glyphs
}

// ShouldDropTable returns true if the table should be excluded.
func (i *Input) ShouldDropTable(tag ot.Tag) bool {
	return i.dropTables[tag]
}

// ShouldPassThrough returns true if the table should be copied unchanged.
func (i *Input) ShouldPassThrough(tag ot.Tag) bool {
	return i.passThroughTables[tag]
}

// HasLayoutFeatures returns true if specific features were requested.
func (i *Input) HasLayoutFeatures() bool {
	return len(i.layoutFeatures) > 0
}

// ShouldKeepFeature returns true if the feature should be retained.
func (i *Input) ShouldKeepFeature(tag ot.Tag) bool {
	if len(i.layoutFeatures) == 0 {
		return true // Keep all if none specified
	}
	return i.layoutFeatures[tag]
}

// --- Variation Axis Pinning (Instancing) ---

// PinAxisLocation pins a variation axis to a specific value.
// When all axes are pinned, the font is "instanced" to a static font.
// The value should be in design-space coordinates (e.g., 700 for Bold weight).
// This is similar to HarfBuzz's hb_subset_input_pin_axis_location().
func (i *Input) PinAxisLocation(axisTag ot.Tag, value float32) {
	i.pinnedAxes[axisTag] = value
}

// PinAxisToDefault pins a variation axis to its default value.
// The font's fvar table is needed to determine the default.
// This is similar to HarfBuzz's hb_subset_input_pin_axis_to_default().
func (i *Input) PinAxisToDefault(font *ot.Font, axisTag ot.Tag) bool {
	if !font.HasTable(ot.TagFvar) {
		return false
	}
	fvarData, err := font.TableData(ot.TagFvar)
	if err != nil {
		return false
	}
	fvar, err := ot.ParseFvar(fvarData)
	if err != nil {
		return false
	}
	axis, found := fvar.FindAxis(axisTag)
	if !found {
		return false
	}
	i.pinnedAxes[axisTag] = axis.DefaultValue
	return true
}

// PinAllAxesToDefault pins all variation axes to their default values.
// This creates a static font at the default instance.
// This is similar to HarfBuzz's hb_subset_input_pin_all_axes_to_default().
func (i *Input) PinAllAxesToDefault(font *ot.Font) bool {
	if !font.HasTable(ot.TagFvar) {
		return false
	}
	fvarData, err := font.TableData(ot.TagFvar)
	if err != nil {
		return false
	}
	fvar, err := ot.ParseFvar(fvarData)
	if err != nil {
		return false
	}
	for _, axis := range fvar.AxisInfos() {
		i.pinnedAxes[axis.Tag] = axis.DefaultValue
	}
	return true
}

// HasPinnedAxes returns true if any axes have been pinned.
func (i *Input) HasPinnedAxes() bool {
	return len(i.pinnedAxes) > 0
}

// PinnedAxes returns the map of pinned axis tags to values.
func (i *Input) PinnedAxes() map[ot.Tag]float32 {
	return i.pinnedAxes
}

// IsFullyInstanced returns true if all axes in the font have been pinned.
func (i *Input) IsFullyInstanced(font *ot.Font) bool {
	if !font.HasTable(ot.TagFvar) {
		return true // Non-variable font is always "fully instanced"
	}
	fvarData, err := font.TableData(ot.TagFvar)
	if err != nil {
		return true
	}
	fvar, err := ot.ParseFvar(fvarData)
	if err != nil {
		return true
	}
	// Check if all axes are pinned
	for _, axis := range fvar.AxisInfos() {
		if _, pinned := i.pinnedAxes[axis.Tag]; !pinned {
			return false
		}
	}
	return true
}
