// Package buffer implements HarfBuzz-compatible text buffer for shaping.
//
// This is a pure Go implementation designed for maximum compatibility
// with HarfBuzz's buffer behavior.
package buffer

// Direction represents text direction.
type Direction uint8

const (
	DirectionInvalid Direction = iota
	DirectionLTR               // Left-to-right
	DirectionRTL               // Right-to-left
	DirectionTTB               // Top-to-bottom
	DirectionBTT               // Bottom-to-top
)

// IsHorizontal returns true if direction is horizontal (LTR or RTL).
func (d Direction) IsHorizontal() bool {
	return d == DirectionLTR || d == DirectionRTL
}

// IsVertical returns true if direction is vertical (TTB or BTT).
func (d Direction) IsVertical() bool {
	return d == DirectionTTB || d == DirectionBTT
}

// IsForward returns true if direction is forward (LTR or TTB).
func (d Direction) IsForward() bool {
	return d == DirectionLTR || d == DirectionTTB
}

// IsBackward returns true if direction is backward (RTL or BTT).
func (d Direction) IsBackward() bool {
	return d == DirectionRTL || d == DirectionBTT
}

// IsValid returns true if direction is valid (not Invalid).
func (d Direction) IsValid() bool {
	return d != DirectionInvalid
}

// Reverse returns the reverse direction.
func (d Direction) Reverse() Direction {
	switch d {
	case DirectionLTR:
		return DirectionRTL
	case DirectionRTL:
		return DirectionLTR
	case DirectionTTB:
		return DirectionBTT
	case DirectionBTT:
		return DirectionTTB
	default:
		return DirectionInvalid
	}
}

// Script represents a Unicode script.
// Values correspond to ISO 15924 script codes.
type Script uint32

// MakeScript creates a Script from a 4-character tag.
func MakeScript(a, b, c, d byte) Script {
	return Script(uint32(a)<<24 | uint32(b)<<16 | uint32(c)<<8 | uint32(d))
}

// Common script values.
var (
	ScriptInvalid = Script(0)
	ScriptCommon  = MakeScript('Z', 'y', 'y', 'y')
	ScriptLatin   = MakeScript('L', 'a', 't', 'n')
	ScriptArabic  = MakeScript('A', 'r', 'a', 'b')
	ScriptHebrew  = MakeScript('H', 'e', 'b', 'r')
	ScriptGreek   = MakeScript('G', 'r', 'e', 'k')
	ScriptHan     = MakeScript('H', 'a', 'n', 'i')
)

// Language represents a BCP 47 language tag.
// Stored as a pointer for efficient comparison (interned strings).
type Language string

const LanguageInvalid Language = ""

// SegmentProperties holds the properties of a text segment.
type SegmentProperties struct {
	Direction Direction
	Script    Script
	Language  Language
}

// Equal returns true if both segment properties are equal.
func (p SegmentProperties) Equal(other SegmentProperties) bool {
	return p.Direction == other.Direction &&
		p.Script == other.Script &&
		p.Language == other.Language
}

// Hash returns a hash value for the segment properties.
func (p SegmentProperties) Hash() uint32 {
	h := uint32(p.Direction) * 31
	h = (h + uint32(p.Script)) * 31
	// Simple string hash for language
	for i := 0; i < len(p.Language); i++ {
		h = h*31 + uint32(p.Language[i])
	}
	return h
}

// ContentType indicates whether buffer contains characters or glyphs.
type ContentType uint8

const (
	ContentTypeInvalid ContentType = iota
	ContentTypeUnicode             // Buffer contains Unicode codepoints (before shaping)
	ContentTypeGlyphs              // Buffer contains glyph IDs (after shaping)
)

// ClusterLevel controls how clusters are handled during shaping.
type ClusterLevel uint8

const (
	// ClusterLevelMonotoneGraphemes groups clusters by graphemes in monotone order.
	// This is the default and maintains backward compatibility.
	ClusterLevelMonotoneGraphemes ClusterLevel = iota

	// ClusterLevelMonotoneCharacters assigns separate cluster values to
	// non-base characters but maintains monotone order.
	ClusterLevelMonotoneCharacters

	// ClusterLevelCharacters assigns separate cluster values without
	// enforcing monotone order. Most granular level.
	ClusterLevelCharacters

	// ClusterLevelGraphemes groups by graphemes without enforcing monotone order.
	ClusterLevelGraphemes

	// ClusterLevelDefault is the default cluster level.
	ClusterLevelDefault = ClusterLevelMonotoneGraphemes
)

// IsMonotone returns true if the cluster level enforces monotone order.
func (c ClusterLevel) IsMonotone() bool {
	return c == ClusterLevelMonotoneGraphemes || c == ClusterLevelMonotoneCharacters
}

// IsGraphemes returns true if the cluster level groups by graphemes.
func (c ClusterLevel) IsGraphemes() bool {
	return c == ClusterLevelMonotoneGraphemes || c == ClusterLevelGraphemes
}

// Flags controls buffer behavior during shaping.
type Flags uint32

const (
	FlagDefault Flags = 0

	// FlagBOT indicates beginning of text (enables special handling).
	FlagBOT Flags = 1 << iota

	// FlagEOT indicates end of text (enables special handling).
	FlagEOT

	// FlagPreserveDefaultIgnorables keeps default ignorable characters
	// visible instead of hiding them.
	FlagPreserveDefaultIgnorables

	// FlagRemoveDefaultIgnorables removes default ignorable characters
	// from the output instead of hiding them.
	FlagRemoveDefaultIgnorables

	// FlagDoNotInsertDottedCircle prevents insertion of dotted circle
	// for incorrect character sequences.
	FlagDoNotInsertDottedCircle

	// FlagVerify enables verification of shaping results.
	FlagVerify

	// FlagProduceUnsafeToConcat enables generation of the
	// GlyphFlagUnsafeToConcat flag during shaping.
	FlagProduceUnsafeToConcat

	// FlagProduceSafeToInsertTatweel enables generation of the
	// GlyphFlagSafeToInsertTatweel flag during shaping.
	FlagProduceSafeToInsertTatweel
)

// GlyphFlags are per-glyph flags set during shaping.
type GlyphFlags uint32

const (
	// GlyphFlagUnsafeToBreak indicates that breaking at this cluster
	// boundary requires reshaping both sides.
	GlyphFlagUnsafeToBreak GlyphFlags = 1 << iota

	// GlyphFlagUnsafeToConcat indicates that changing text on either
	// side of this cluster may change shaping results.
	GlyphFlagUnsafeToConcat

	// GlyphFlagSafeToInsertTatweel indicates it's safe to insert
	// a tatweel (U+0640) before this cluster for elongation.
	GlyphFlagSafeToInsertTatweel

	// GlyphFlagDefined is a mask of all defined glyph flags.
	GlyphFlagDefined = GlyphFlagUnsafeToBreak | GlyphFlagUnsafeToConcat | GlyphFlagSafeToInsertTatweel
)

// ScratchFlags are internal flags used during shaping.
type ScratchFlags uint32

const (
	ScratchFlagDefault ScratchFlags = 0

	ScratchFlagHasFractionSlash             ScratchFlags = 1 << 0
	ScratchFlagHasDefaultIgnorables         ScratchFlags = 1 << 1
	ScratchFlagHasSpaceFallback             ScratchFlags = 1 << 2
	ScratchFlagHasGPOSAttachment            ScratchFlags = 1 << 3
	ScratchFlagHasCGJ                       ScratchFlags = 1 << 4
	ScratchFlagHasBrokenSyllable            ScratchFlags = 1 << 5
	ScratchFlagHasVariationSelectorFallback ScratchFlags = 1 << 6
	ScratchFlagHasContinuations             ScratchFlags = 1 << 7

	// Reserved for shapers
	ScratchFlagShaper0 ScratchFlags = 1 << 24
	ScratchFlagShaper1 ScratchFlags = 1 << 25
	ScratchFlagShaper2 ScratchFlags = 1 << 26
	ScratchFlagShaper3 ScratchFlags = 1 << 27
)

// Position is a font unit value (typically 1/64th of a pixel at 72dpi).
type Position = int32

// Codepoint represents either a Unicode codepoint or a glyph ID.
type Codepoint = uint32

// Mask is a bitmask used for feature application.
type Mask = uint32

// ReplacementCodepoint is the default replacement for invalid characters (U+FFFD).
const ReplacementCodepoint Codepoint = 0xFFFD

// CodepointInvalid represents an invalid codepoint value.
const CodepointInvalid Codepoint = 0xFFFFFFFF
