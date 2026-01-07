package buffer

// GlyphInfo holds information about a single glyph.
//
// Before shaping, Codepoint contains a Unicode codepoint.
// After shaping, Codepoint contains a glyph ID.
//
// The Mask field contains feature flags during shaping and
// glyph flags (GlyphFlagUnsafeToBreak, etc.) after shaping.
//
// The internal fields (glyphProps, unicodeProps, etc.) are used
// during shaping and should not be accessed directly.
type GlyphInfo struct {
	// Codepoint is either a Unicode codepoint (before shaping)
	// or a glyph ID (after shaping).
	Codepoint Codepoint

	// Mask contains feature flags during shaping.
	// After shaping, the lower bits contain GlyphFlags.
	Mask Mask

	// Cluster is the index of the character in the original text
	// that corresponds to this glyph. Multiple glyphs can share
	// the same cluster value.
	Cluster uint32

	// Internal fields - var1 and var2 in HarfBuzz
	// These are used for various purposes during shaping:
	// - Unicode properties
	// - Glyph properties (from GDEF)
	// - Ligature/component tracking
	// - Syllable information
	var1 uint32
	var2 uint32
}

// GlyphFlags returns the glyph flags from the mask.
func (g *GlyphInfo) GlyphFlags() GlyphFlags {
	return GlyphFlags(g.Mask) & GlyphFlagDefined
}

// --- Internal property accessors ---
// These mirror HarfBuzz's internal glyph property system.

// Glyph properties stored in var1 (lower 16 bits)
const (
	glyphPropsBase        uint16 = 1 << 0
	glyphPropsLigature    uint16 = 1 << 1
	glyphPropsMark        uint16 = 1 << 2
	glyphPropsComponent   uint16 = 1 << 3
	glyphPropsSubstituted uint16 = 1 << 4
	glyphPropsLigated     uint16 = 1 << 5
	glyphPropsMultiplied  uint16 = 1 << 6
)

// Unicode properties stored in var2
const (
	unicodePropGeneralCategory uint32 = 0x001F // 5 bits
	unicodePropModCombClass    uint32 = 0xFF00 // 8 bits at offset 8
)

// glyphProps returns the glyph properties.
func (g *GlyphInfo) glyphProps() uint16 {
	return uint16(g.var1)
}

// setGlyphProps sets the glyph properties.
func (g *GlyphInfo) setGlyphProps(props uint16) {
	g.var1 = (g.var1 & 0xFFFF0000) | uint32(props)
}

// unicodeProps returns the unicode properties.
func (g *GlyphInfo) unicodeProps() uint32 {
	return g.var2
}

// setUnicodeProps sets the unicode properties.
func (g *GlyphInfo) setUnicodeProps(props uint32) {
	g.var2 = props
}

// IsBase returns true if this is a base glyph.
func (g *GlyphInfo) IsBase() bool {
	return g.glyphProps()&glyphPropsBase != 0
}

// IsLigature returns true if this glyph is the result of a ligature substitution.
func (g *GlyphInfo) IsLigature() bool {
	return g.glyphProps()&glyphPropsLigature != 0
}

// IsMark returns true if this is a mark (combining) glyph.
func (g *GlyphInfo) IsMark() bool {
	return g.glyphProps()&glyphPropsMark != 0
}

// IsComponent returns true if this is a component of a ligature.
func (g *GlyphInfo) IsComponent() bool {
	return g.glyphProps()&glyphPropsComponent != 0
}

// IsSubstituted returns true if this glyph was substituted by GSUB.
func (g *GlyphInfo) IsSubstituted() bool {
	return g.glyphProps()&glyphPropsSubstituted != 0
}

// IsLigated returns true if this glyph was ligated.
func (g *GlyphInfo) IsLigated() bool {
	return g.glyphProps()&glyphPropsLigated != 0
}

// IsMultiplied returns true if this glyph was multiplied (expanded from one to many).
func (g *GlyphInfo) IsMultiplied() bool {
	return g.glyphProps()&glyphPropsMultiplied != 0
}

// setBase marks this as a base glyph.
func (g *GlyphInfo) setBase() {
	g.setGlyphProps(g.glyphProps() | glyphPropsBase)
}

// setMark marks this as a mark glyph.
func (g *GlyphInfo) setMark() {
	g.setGlyphProps(g.glyphProps() | glyphPropsMark)
}

// setSubstituted marks this glyph as substituted.
func (g *GlyphInfo) setSubstituted() {
	g.setGlyphProps(g.glyphProps() | glyphPropsSubstituted)
}

// setLigated marks this glyph as ligated.
func (g *GlyphInfo) setLigated() {
	g.setGlyphProps(g.glyphProps() | glyphPropsLigated)
}

// setMultiplied marks this glyph as multiplied.
func (g *GlyphInfo) setMultiplied() {
	g.setGlyphProps(g.glyphProps() | glyphPropsMultiplied)
}

// ligID returns the ligature ID (for tracking components).
func (g *GlyphInfo) ligID() uint8 {
	return uint8(g.var1 >> 16)
}

// setLigID sets the ligature ID.
func (g *GlyphInfo) setLigID(id uint8) {
	g.var1 = (g.var1 & 0xFF00FFFF) | (uint32(id) << 16)
}

// ligComp returns the ligature component index.
func (g *GlyphInfo) ligComp() uint8 {
	return uint8(g.var1 >> 24)
}

// setLigComp sets the ligature component index.
func (g *GlyphInfo) setLigComp(comp uint8) {
	g.var1 = (g.var1 & 0x00FFFFFF) | (uint32(comp) << 24)
}

// syllable returns the syllable index (for complex scripts).
func (g *GlyphInfo) syllable() uint8 {
	return uint8(g.var2 >> 24)
}

// setSyllable sets the syllable index.
func (g *GlyphInfo) setSyllable(s uint8) {
	g.var2 = (g.var2 & 0x00FFFFFF) | (uint32(s) << 24)
}

// GlyphPosition holds positioning information for a glyph.
type GlyphPosition struct {
	// XAdvance is how much the line advances horizontally after this glyph.
	XAdvance Position

	// YAdvance is how much the line advances vertically after this glyph.
	YAdvance Position

	// XOffset is the horizontal offset from the current position.
	XOffset Position

	// YOffset is the vertical offset from the current position.
	YOffset Position

	// Internal field for attachment information
	var_ uint32
}

// attachType returns the attachment type (for cursive positioning).
func (p *GlyphPosition) attachType() uint8 {
	return uint8(p.var_)
}

// setAttachType sets the attachment type.
func (p *GlyphPosition) setAttachType(t uint8) {
	p.var_ = (p.var_ & 0xFFFFFF00) | uint32(t)
}

// attachChain returns the attachment chain index.
func (p *GlyphPosition) attachChain() int16 {
	return int16(p.var_ >> 16)
}

// setAttachChain sets the attachment chain index.
func (p *GlyphPosition) setAttachChain(c int16) {
	p.var_ = (p.var_ & 0x0000FFFF) | (uint32(uint16(c)) << 16)
}
