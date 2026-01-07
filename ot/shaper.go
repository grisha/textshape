package ot

import (
	"sync"
	"unicode"
)

// Note: Direction, DirectionLTR, DirectionRTL are defined in gpos.go

// GlyphInfo holds information about a shaped glyph.
type GlyphInfo struct {
	Codepoint  Codepoint // Original Unicode codepoint (0 if synthetic)
	GlyphID    GlyphID   // Glyph index in the font
	Cluster    int       // Cluster index (maps back to original text position)
	GlyphClass int       // GDEF glyph class (if available)
}

// GlyphPos holds positioning information for a shaped glyph.
type GlyphPos struct {
	XAdvance int16 // Horizontal advance
	YAdvance int16 // Vertical advance
	XOffset  int16 // Horizontal offset
	YOffset  int16 // Vertical offset
}

// BufferFlags controls buffer behavior during shaping.
// These match HarfBuzz's hb_buffer_flags_t.
type BufferFlags uint32

const (
	// BufferFlagDefault is the default buffer flag.
	BufferFlagDefault BufferFlags = 0
	// BufferFlagBOT indicates beginning of text paragraph.
	BufferFlagBOT BufferFlags = 1 << iota
	// BufferFlagEOT indicates end of text paragraph.
	BufferFlagEOT
	// BufferFlagPreserveDefaultIgnorables keeps default ignorable characters visible.
	BufferFlagPreserveDefaultIgnorables
	// BufferFlagRemoveDefaultIgnorables removes default ignorable characters from output.
	BufferFlagRemoveDefaultIgnorables
	// BufferFlagDoNotInsertDottedCircle prevents dotted circle insertion for invalid sequences.
	BufferFlagDoNotInsertDottedCircle
)

// Buffer holds a sequence of glyphs being shaped.
type Buffer struct {
	Info      []GlyphInfo
	Pos       []GlyphPos
	Direction Direction
	Flags     BufferFlags

	// Script and Language for shaping (optional, can be auto-detected)
	Script   Tag
	Language Tag
}

// NewBuffer creates a new empty buffer.
func NewBuffer() *Buffer {
	return &Buffer{
		Direction: DirectionLTR,
	}
}

// AddCodepoints adds Unicode codepoints to the buffer.
func (b *Buffer) AddCodepoints(codepoints []Codepoint) {
	for i, cp := range codepoints {
		b.Info = append(b.Info, GlyphInfo{
			Codepoint: cp,
			Cluster:   i,
		})
	}
	b.Pos = make([]GlyphPos, len(b.Info))
}

// AddString adds a string to the buffer.
func (b *Buffer) AddString(s string) {
	runes := []rune(s)
	for i, r := range runes {
		b.Info = append(b.Info, GlyphInfo{
			Codepoint: Codepoint(r),
			Cluster:   i,
		})
	}
	b.Pos = make([]GlyphPos, len(b.Info))
}

// SetDirection sets the text direction.
func (b *Buffer) SetDirection(dir Direction) {
	b.Direction = dir
}

// Len returns the number of glyphs in the buffer.
func (b *Buffer) Len() int {
	return len(b.Info)
}

// Clear removes all glyphs from the buffer.
func (b *Buffer) Clear() {
	b.Info = b.Info[:0]
	b.Pos = b.Pos[:0]
}

// Reset clears the buffer and resets all properties to defaults.
func (b *Buffer) Reset() {
	b.Info = b.Info[:0]
	b.Pos = b.Pos[:0]
	b.Direction = DirectionLTR
	b.Flags = BufferFlagDefault
	b.Script = 0
	b.Language = 0
}

// GuessSegmentProperties guesses direction, script, and language from buffer content.
// This is similar to HarfBuzz's hb_buffer_guess_segment_properties().
func (b *Buffer) GuessSegmentProperties() {
	if len(b.Info) == 0 {
		return
	}

	// Guess direction from content
	if b.Direction == 0 {
		for _, info := range b.Info {
			r := rune(info.Codepoint)
			if unicode.Is(unicode.Arabic, r) ||
				unicode.Is(unicode.Hebrew, r) ||
				unicode.Is(unicode.Syriac, r) ||
				unicode.Is(unicode.Thaana, r) {
				b.Direction = DirectionRTL
				break
			}
			if unicode.IsLetter(r) {
				b.Direction = DirectionLTR
				break
			}
		}
		if b.Direction == 0 {
			b.Direction = DirectionLTR
		}
	}

	// TODO: Guess script and language from content
}

// GlyphIDs returns just the glyph IDs.
func (b *Buffer) GlyphIDs() []GlyphID {
	ids := make([]GlyphID, len(b.Info))
	for i, info := range b.Info {
		ids[i] = info.GlyphID
	}
	return ids
}

// Shaper holds font data and performs text shaping.
type Shaper struct {
	font *Font
	cmap *Cmap
	gdef *GDEF
	gsub *GSUB
	gpos *GPOS
	hmtx *Hmtx

	// Default features to apply when nil is passed to Shape
	defaultFeatures []Feature
}

// NewShaper creates a shaper from a parsed font.
func NewShaper(font *Font) (*Shaper, error) {
	s := &Shaper{
		font: font,
	}

	// Parse cmap (required)
	if font.HasTable(TagCmap) {
		data, err := font.TableData(TagCmap)
		if err != nil {
			return nil, err
		}
		s.cmap, err = ParseCmap(data)
		if err != nil {
			return nil, err
		}
	}

	// Parse GDEF (optional)
	if font.HasTable(TagGDEF) {
		data, err := font.TableData(TagGDEF)
		if err == nil {
			s.gdef, _ = ParseGDEF(data)
		}
	}

	// Parse GSUB (optional)
	if font.HasTable(TagGSUB) {
		data, err := font.TableData(TagGSUB)
		if err == nil {
			s.gsub, _ = ParseGSUB(data)
		}
	}

	// Parse GPOS (optional)
	if font.HasTable(TagGPOS) {
		data, err := font.TableData(TagGPOS)
		if err == nil {
			s.gpos, _ = ParseGPOS(data)
		}
	}

	// Parse hmtx (optional but important for positioning)
	if font.HasTable(TagHmtx) && font.HasTable(TagHhea) {
		s.hmtx, _ = ParseHmtxFromFont(font)
	}

	// Set default features
	s.defaultFeatures = DefaultFeatures()

	return s, nil
}

// Note: TagKern, TagMark, TagMkmk are defined in gpos.go

// Shape shapes the text in the buffer using the specified features.
// If features is nil, default features are used.
// This is the main shaping entry point, similar to HarfBuzz's hb_shape().
func (s *Shaper) Shape(buf *Buffer, features []Feature) {
	if buf.Len() == 0 {
		return
	}

	// Use default features if none specified
	if features == nil {
		features = s.defaultFeatures
	}

	// Separate features into GSUB and GPOS
	// (In HarfBuzz this is more sophisticated with feature ordering)
	gsubFeatures, gposFeatures := s.categorizeFeatures(features)

	// Step 1: Map codepoints to glyphs
	s.mapCodepointsToGlyphs(buf)

	// Step 2: Set glyph classes from GDEF
	s.setGlyphClasses(buf)

	// Step 3: Apply GSUB features
	s.applyGSUB(buf, gsubFeatures)

	// Step 4: Set base advances from hmtx (after GSUB, since glyphs may have changed)
	s.setBaseAdvances(buf)

	// Step 5: Apply GPOS features (adjustments to base advances)
	s.applyGPOS(buf, gposFeatures)
}

// categorizeFeatures separates features into GSUB and GPOS categories.
// Features are categorized based on whether they exist in the font's GSUB or GPOS table.
func (s *Shaper) categorizeFeatures(features []Feature) (gsub, gpos []Feature) {
	for _, f := range features {
		if f.Value == 0 {
			continue // Disabled feature
		}

		// Check if feature exists in GSUB
		if s.gsub != nil {
			if featureList, err := s.gsub.ParseFeatureList(); err == nil {
				if featureList.FindFeature(f.Tag) != nil {
					gsub = append(gsub, f)
				}
			}
		}

		// Check if feature exists in GPOS
		if s.gpos != nil {
			if featureList, err := s.gpos.ParseFeatureList(); err == nil {
				if featureList.FindFeature(f.Tag) != nil {
					gpos = append(gpos, f)
				}
			}
		}
	}
	return
}

// mapCodepointsToGlyphs converts Unicode codepoints to glyph IDs.
func (s *Shaper) mapCodepointsToGlyphs(buf *Buffer) {
	if s.cmap == nil {
		return
	}

	for i := range buf.Info {
		glyph, ok := s.cmap.Lookup(buf.Info[i].Codepoint)
		if ok {
			buf.Info[i].GlyphID = glyph
		} else {
			// Use .notdef glyph (0)
			buf.Info[i].GlyphID = 0
		}
	}
}

// setGlyphClasses sets GDEF glyph classes.
func (s *Shaper) setGlyphClasses(buf *Buffer) {
	if s.gdef == nil || !s.gdef.HasGlyphClasses() {
		return
	}

	for i := range buf.Info {
		buf.Info[i].GlyphClass = s.gdef.GetGlyphClass(buf.Info[i].GlyphID)
	}
}

// setBaseAdvances sets the base advance widths from hmtx.
func (s *Shaper) setBaseAdvances(buf *Buffer) {
	if s.hmtx == nil {
		return
	}

	for i := range buf.Info {
		adv := s.hmtx.GetAdvanceWidth(buf.Info[i].GlyphID)
		buf.Pos[i].XAdvance = int16(adv)
	}
}

// applyGSUB applies GSUB features to the buffer.
func (s *Shaper) applyGSUB(buf *Buffer, features []Feature) {
	if s.gsub == nil || len(features) == 0 {
		return
	}

	// Extract glyph IDs for GSUB processing
	glyphs := buf.GlyphIDs()

	// Apply each feature
	for _, f := range features {
		if f.Value == 0 {
			continue // Feature disabled
		}
		// TODO: Respect f.Start/f.End for partial feature application
		glyphs = s.gsub.ApplyFeatureWithGDEF(f.Tag, glyphs, s.gdef)
	}

	// Update buffer with new glyphs
	// Note: This is simplified - a full implementation would track clusters
	s.updateBufferFromGlyphs(buf, glyphs)
}

// updateBufferFromGlyphs updates the buffer after GSUB processing.
func (s *Shaper) updateBufferFromGlyphs(buf *Buffer, glyphs []GlyphID) {
	// If length changed, we need to rebuild the buffer
	if len(glyphs) != len(buf.Info) {
		// Create new info array
		newInfo := make([]GlyphInfo, len(glyphs))

		// Simple cluster preservation: try to maintain clusters
		// This is a simplified approach
		oldLen := len(buf.Info)
		for i, glyph := range glyphs {
			newInfo[i].GlyphID = glyph
			// Map cluster from original position (simplified)
			if i < oldLen {
				newInfo[i].Cluster = buf.Info[i].Cluster
				newInfo[i].Codepoint = buf.Info[i].Codepoint
			} else if oldLen > 0 {
				// For added glyphs, use last cluster
				newInfo[i].Cluster = buf.Info[oldLen-1].Cluster
			}
			// Update glyph class
			if s.gdef != nil && s.gdef.HasGlyphClasses() {
				newInfo[i].GlyphClass = s.gdef.GetGlyphClass(glyph)
			}
		}

		buf.Info = newInfo
		buf.Pos = make([]GlyphPos, len(glyphs))
	} else {
		// Same length, just update glyph IDs and classes
		for i, glyph := range glyphs {
			buf.Info[i].GlyphID = glyph
			if s.gdef != nil && s.gdef.HasGlyphClasses() {
				buf.Info[i].GlyphClass = s.gdef.GetGlyphClass(glyph)
			}
		}
	}
}

// applyGPOS applies GPOS features to the buffer.
func (s *Shaper) applyGPOS(buf *Buffer, features []Feature) {
	if s.gpos == nil || len(features) == 0 {
		return
	}

	// Get glyph IDs
	glyphs := buf.GlyphIDs()

	// Create position array for GPOS adjustments (starts at zero)
	positions := make([]GlyphPosition, len(glyphs))

	// Apply each feature
	for _, f := range features {
		if f.Value == 0 {
			continue // Feature disabled
		}
		// TODO: Respect f.Start/f.End for partial feature application
		s.applyGPOSFeature(f.Tag, glyphs, positions)
	}

	// Add GPOS adjustments to existing base advances
	for i := range positions {
		buf.Pos[i].XAdvance += positions[i].XAdvance
		buf.Pos[i].YAdvance += positions[i].YAdvance
		buf.Pos[i].XOffset += positions[i].XPlacement
		buf.Pos[i].YOffset += positions[i].YPlacement
	}
}

// applyGPOSFeature applies a single GPOS feature.
func (s *Shaper) applyGPOSFeature(feature Tag, glyphs []GlyphID, positions []GlyphPosition) {
	featureList, err := s.gpos.ParseFeatureList()
	if err != nil {
		return
	}

	lookups := featureList.FindFeature(feature)
	if lookups == nil {
		return
	}

	// Apply lookups in order
	for _, lookupIdx := range lookups {
		s.gpos.ApplyLookupWithGDEF(int(lookupIdx), glyphs, positions, DirectionLTR, s.gdef)
	}
}

// GuessDirection guesses text direction from the content.
func GuessDirection(s string) Direction {
	for _, r := range s {
		if unicode.Is(unicode.Arabic, r) ||
			unicode.Is(unicode.Hebrew, r) ||
			unicode.Is(unicode.Syriac, r) ||
			unicode.Is(unicode.Thaana, r) {
			return DirectionRTL
		}
		if unicode.IsLetter(r) {
			return DirectionLTR
		}
	}
	return DirectionLTR
}

// ShapeString is a convenience function that shapes a string and returns
// the glyph IDs and positions.
func (s *Shaper) ShapeString(text string) ([]GlyphID, []GlyphPos) {
	buf := NewBuffer()
	buf.AddString(text)
	buf.SetDirection(GuessDirection(text))
	s.Shape(buf, nil) // Use default features
	return buf.GlyphIDs(), buf.Pos
}

// HasGSUB returns true if the shaper has GSUB data.
func (s *Shaper) HasGSUB() bool {
	return s.gsub != nil
}

// HasGPOS returns true if the shaper has GPOS data.
func (s *Shaper) HasGPOS() bool {
	return s.gpos != nil
}

// HasGDEF returns true if the shaper has GDEF data.
func (s *Shaper) HasGDEF() bool {
	return s.gdef != nil
}

// HasHmtx returns true if the shaper has hmtx data.
func (s *Shaper) HasHmtx() bool {
	return s.hmtx != nil
}

// GDEF returns the GDEF table (may be nil).
func (s *Shaper) GDEF() *GDEF {
	return s.gdef
}

// SetDefaultFeatures sets the default features to apply when Shape is called with nil.
func (s *Shaper) SetDefaultFeatures(features []Feature) {
	s.defaultFeatures = features
}

// DefaultFeatures returns the current default features.
func (s *Shaper) GetDefaultFeatures() []Feature {
	return s.defaultFeatures
}

// Font returns the font associated with this shaper.
func (s *Shaper) Font() *Font {
	return s.font
}

// GetGlyphName returns a debug name for a glyph (just the ID as string).
func GetGlyphName(glyph GlyphID) string {
	return string(rune('A' + int(glyph)%26)) // Simple debug representation
}

// Shaper cache for convenience function
var shaperCache = make(map[*Font]*Shaper)
var shaperCacheMu sync.RWMutex

// Shape is a convenience function that shapes text in a buffer using a font.
// It caches shapers internally for efficiency.
// This is similar to HarfBuzz's hb_shape() function.
func Shape(font *Font, buf *Buffer, features []Feature) error {
	shaperCacheMu.RLock()
	shaper, ok := shaperCache[font]
	shaperCacheMu.RUnlock()

	if !ok {
		var err error
		shaper, err = NewShaper(font)
		if err != nil {
			return err
		}

		shaperCacheMu.Lock()
		shaperCache[font] = shaper
		shaperCacheMu.Unlock()
	}

	shaper.Shape(buf, features)
	return nil
}

// ClearShaperCache clears the internal shaper cache.
// Call this if fonts are being released to allow garbage collection.
func ClearShaperCache() {
	shaperCacheMu.Lock()
	shaperCache = make(map[*Font]*Shaper)
	shaperCacheMu.Unlock()
}
