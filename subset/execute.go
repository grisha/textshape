package subset

import (
	"encoding/binary"
	"sort"

	"github.com/boxesandglue/textshape/ot"
)

// cmapMapping is used for cmap construction
type cmapMapping struct {
	cp  rune
	gid ot.GlyphID
}

// Execute performs the subsetting operation and returns the new font data.
func (p *Plan) Execute() ([]byte, error) {
	builder := NewFontBuilder()

	// Subset required tables
	if err := p.subsetHead(builder); err != nil {
		return nil, err
	}
	if err := p.subsetMaxp(builder); err != nil {
		return nil, err
	}
	if err := p.subsetHhea(builder); err != nil {
		return nil, err
	}
	if err := p.subsetHmtx(builder); err != nil {
		return nil, err
	}

	// Subset glyf/loca if present (TrueType)
	if p.source.HasTable(ot.TagGlyf) {
		if err := p.subsetGlyf(builder); err != nil {
			return nil, err
		}
	}

	// Subset CFF if present (OpenType/CFF)
	if p.source.HasTable(ot.TagCFF) && p.cff != nil {
		if cffData, err := p.subsetCFF(); err == nil && cffData != nil {
			builder.AddTable(ot.TagCFF, cffData)
		}
	}

	// Subset cmap
	if err := p.subsetCmap(builder); err != nil {
		return nil, err
	}

	// Subset GSUB/GPOS/GDEF unless FlagDropLayoutTables is set
	// (For PDF embedding, these tables are not needed since shaping is already done)
	if p.input.Flags&FlagDropLayoutTables == 0 {
		// Subset GSUB (with glyph ID remapping)
		if p.gsub != nil {
			if gsubData, err := p.subsetGSUB(); err == nil && gsubData != nil {
				builder.AddTable(ot.TagGSUB, gsubData)
			}
		}

		// Subset GPOS (with glyph ID remapping)
		if p.gpos != nil {
			if gposData, err := p.subsetGPOS(); err == nil && gposData != nil {
				builder.AddTable(ot.TagGPOS, gposData)
			}
		}

		// Subset GDEF (with glyph ID remapping)
		if p.gdef != nil {
			if gdefData, err := p.subsetGDEF(); err == nil && gdefData != nil {
				builder.AddTable(ot.TagGDEF, gdefData)
			}
		}
	}

	// Copy or subset optional tables
	p.handleOptionalTables(builder)

	return builder.Build()
}

// subsetHead subsets the head table.
func (p *Plan) subsetHead(builder *FontBuilder) error {
	data, err := p.source.TableData(ot.TagHead)
	if err != nil {
		return ErrMissingTable
	}

	// head is 54 bytes, copy it unchanged (checksumAdjustment will be fixed later)
	newData := make([]byte, len(data))
	copy(newData, data)

	// Update indexToLocFormat based on output loca format
	// We'll use long format (1) by default for simplicity
	binary.BigEndian.PutUint16(newData[50:], 1)

	builder.AddTable(ot.TagHead, newData)
	return nil
}

// subsetMaxp subsets the maxp table.
func (p *Plan) subsetMaxp(builder *FontBuilder) error {
	data, err := p.source.TableData(ot.TagMaxp)
	if err != nil {
		return ErrMissingTable
	}

	newData := make([]byte, len(data))
	copy(newData, data)

	// Update numGlyphs
	binary.BigEndian.PutUint16(newData[4:], uint16(p.numOutputGlyphs))

	builder.AddTable(ot.TagMaxp, newData)
	return nil
}

// subsetHhea subsets the hhea table.
func (p *Plan) subsetHhea(builder *FontBuilder) error {
	data, err := p.source.TableData(ot.TagHhea)
	if err != nil {
		return ErrMissingTable
	}

	newData := make([]byte, len(data))
	copy(newData, data)

	// Update numberOfHMetrics (we'll have one per glyph for simplicity)
	binary.BigEndian.PutUint16(newData[34:], uint16(p.numOutputGlyphs))

	builder.AddTable(ot.TagHhea, newData)
	return nil
}

// subsetHmtx subsets the hmtx table.
// When axes are pinned (instancing), HVAR deltas are applied to the advances.
func (p *Plan) subsetHmtx(builder *FontBuilder) error {
	if p.hmtx == nil {
		return ErrMissingTable
	}

	// Build new hmtx with one longHorMetric per glyph
	newData := make([]byte, p.numOutputGlyphs*4)

	for newGID := 0; newGID < p.numOutputGlyphs; newGID++ {
		oldGID, ok := p.reverseMap[ot.GlyphID(newGID)]
		if !ok {
			// Empty glyph slot (only happens with FlagRetainGIDs)
			continue
		}

		// Use instanced advance if available (includes HVAR deltas)
		var advance uint16
		if p.IsInstanced() {
			advance = p.GetInstancedAdvance(oldGID)
		} else {
			advance = p.hmtx.GetAdvanceWidth(oldGID)
		}
		_, lsb := p.hmtx.GetMetrics(oldGID)

		off := newGID * 4
		binary.BigEndian.PutUint16(newData[off:], advance)
		binary.BigEndian.PutUint16(newData[off+2:], uint16(lsb))
	}

	builder.AddTable(ot.TagHmtx, newData)
	return nil
}

// subsetGlyf subsets the glyf and loca tables.
// When instancing (axes are pinned), gvar deltas are applied to glyph outlines.
func (p *Plan) subsetGlyf(builder *FontBuilder) error {
	if p.glyf == nil {
		return ErrMissingTable
	}

	// Build new glyf and loca
	var glyfData []byte
	offsets := make([]uint32, p.numOutputGlyphs+1)

	for newGID := 0; newGID < p.numOutputGlyphs; newGID++ {
		offsets[newGID] = uint32(len(glyfData))

		oldGID, ok := p.reverseMap[ot.GlyphID(newGID)]
		if !ok {
			// Empty glyph slot
			continue
		}

		glyphBytes := p.glyf.GetGlyphBytes(oldGID)
		if glyphBytes == nil {
			continue
		}

		// Apply gvar deltas when instancing
		if p.IsInstanced() && len(glyphBytes) >= 10 {
			numberOfContours := int16(glyphBytes[0])<<8 | int16(glyphBytes[1])
			if numberOfContours > 0 {
				// Simple glyph - parse points and apply deltas
				points, _, err := ot.ParseSimpleGlyph(glyphBytes)
				if err == nil && len(points) > 0 {
					// Use GetGlyphDeltasWithCoords for proper IUP interpolation
					xDeltas, yDeltas := p.GetGlyphDeltasWithCoords(oldGID, len(points), points)
					if xDeltas != nil && yDeltas != nil {
						glyphBytes = ot.InstanceSimpleGlyph(glyphBytes, xDeltas, yDeltas)
					}
				}
			}
			// Composite glyphs: component transforms may need adjustment
			// but for now we just pass them through unchanged
		}

		// Remap composite glyph component IDs
		glyphBytes = ot.RemapComposite(glyphBytes, p.glyphMap)

		glyfData = append(glyfData, glyphBytes...)

		// Pad to 2-byte boundary (required for loca)
		for len(glyfData)%2 != 0 {
			glyfData = append(glyfData, 0)
		}
	}
	offsets[p.numOutputGlyphs] = uint32(len(glyfData))

	// Build loca table (long format)
	locaData := ot.BuildLoca(offsets, false)

	builder.AddTable(ot.TagGlyf, glyfData)
	builder.AddTable(ot.TagLoca, locaData)

	return nil
}

// subsetCmap subsets the cmap table.
func (p *Plan) subsetCmap(builder *FontBuilder) error {
	if p.cmap == nil || len(p.unicodeMap) == 0 {
		return nil
	}

	// Build a simple format 4 cmap for BMP characters
	// and format 12 for characters outside BMP

	// Collect mappings
	var mappings []cmapMapping
	for cp, gid := range p.unicodeMap {
		mappings = append(mappings, cmapMapping{cp, gid})
	}
	sort.Slice(mappings, func(i, j int) bool { return mappings[i].cp < mappings[j].cp })

	// Check if we need format 12 (characters > 0xFFFF)
	needsFormat12 := false
	for _, m := range mappings {
		if m.cp > 0xFFFF {
			needsFormat12 = true
			break
		}
	}

	var cmapData []byte

	if needsFormat12 {
		// Build format 12 cmap
		cmapData = p.buildCmapFormat12(mappings)
	} else {
		// Build format 4 cmap (simpler, sufficient for BMP)
		cmapData = p.buildCmapFormat4(mappings)
	}

	builder.AddTable(ot.TagCmap, cmapData)
	return nil
}

// buildCmapFormat4 builds a format 4 cmap subtable for BMP characters.
func (p *Plan) buildCmapFormat4(mappings []cmapMapping) []byte {
	// Group consecutive mappings with same delta
	type segment struct {
		startCode uint16
		endCode   uint16
		delta     int16
		idOffset  uint16
	}

	var segments []segment
	var glyphIDs []uint16

	i := 0
	for i < len(mappings) {
		start := mappings[i]
		if start.cp > 0xFFFF {
			i++
			continue
		}

		// Try to build a segment with constant delta
		delta := int(start.gid) - int(start.cp)
		end := start

		j := i + 1
		for j < len(mappings) {
			next := mappings[j]
			if next.cp > 0xFFFF {
				break
			}
			if int(next.gid)-int(next.cp) != delta || int(next.cp) != int(end.cp)+1 {
				break
			}
			end = next
			j++
		}

		segments = append(segments, segment{
			startCode: uint16(start.cp),
			endCode:   uint16(end.cp),
			delta:     int16(delta),
			idOffset:  0,
		})

		i = j
	}

	// Add terminating segment
	segments = append(segments, segment{
		startCode: 0xFFFF,
		endCode:   0xFFFF,
		delta:     1,
		idOffset:  0,
	})

	segCount := len(segments)
	segCountX2 := segCount * 2

	// Calculate search params
	searchRange := 2
	entrySelector := 0
	for searchRange*2 <= segCountX2 {
		searchRange *= 2
		entrySelector++
	}
	rangeShift := segCountX2 - searchRange

	// Build format 4 subtable
	headerLen := 16 + segCount*8 + len(glyphIDs)*2
	subtableLen := headerLen

	subtable := make([]byte, subtableLen)
	binary.BigEndian.PutUint16(subtable[0:], 4)                      // format
	binary.BigEndian.PutUint16(subtable[2:], uint16(subtableLen))    // length
	binary.BigEndian.PutUint16(subtable[4:], 0)                      // language
	binary.BigEndian.PutUint16(subtable[6:], uint16(segCountX2))     // segCountX2
	binary.BigEndian.PutUint16(subtable[8:], uint16(searchRange))    // searchRange
	binary.BigEndian.PutUint16(subtable[10:], uint16(entrySelector)) // entrySelector
	binary.BigEndian.PutUint16(subtable[12:], uint16(rangeShift))    // rangeShift

	// endCode array
	off := 14
	for _, seg := range segments {
		binary.BigEndian.PutUint16(subtable[off:], seg.endCode)
		off += 2
	}

	// reservedPad
	binary.BigEndian.PutUint16(subtable[off:], 0)
	off += 2

	// startCode array
	for _, seg := range segments {
		binary.BigEndian.PutUint16(subtable[off:], seg.startCode)
		off += 2
	}

	// idDelta array
	for _, seg := range segments {
		binary.BigEndian.PutUint16(subtable[off:], uint16(seg.delta))
		off += 2
	}

	// idRangeOffset array
	for _, seg := range segments {
		binary.BigEndian.PutUint16(subtable[off:], seg.idOffset)
		off += 2
	}

	// Build cmap header
	cmapLen := 4 + 8 + len(subtable) // header + 1 encoding record + subtable
	cmap := make([]byte, cmapLen)

	binary.BigEndian.PutUint16(cmap[0:], 0) // version
	binary.BigEndian.PutUint16(cmap[2:], 1) // numTables

	// Encoding record (platform 3, encoding 1 = Windows Unicode BMP)
	binary.BigEndian.PutUint16(cmap[4:], 3)  // platformID
	binary.BigEndian.PutUint16(cmap[6:], 1)  // encodingID
	binary.BigEndian.PutUint32(cmap[8:], 12) // offset to subtable

	// Copy subtable
	copy(cmap[12:], subtable)

	return cmap
}

// buildCmapFormat12 builds a format 12 cmap subtable.
func (p *Plan) buildCmapFormat12(mappings []cmapMapping) []byte {
	// Group consecutive mappings
	type group struct {
		startChar  uint32
		endChar    uint32
		startGlyph uint32
	}

	var groups []group

	i := 0
	for i < len(mappings) {
		start := mappings[i]
		startGlyph := uint32(start.gid)
		endChar := start.cp

		j := i + 1
		for j < len(mappings) {
			next := mappings[j]
			if int(next.cp) != int(endChar)+1 || uint32(next.gid) != startGlyph+uint32(j-i) {
				break
			}
			endChar = next.cp
			j++
		}

		groups = append(groups, group{
			startChar:  uint32(start.cp),
			endChar:    uint32(endChar),
			startGlyph: startGlyph,
		})

		i = j
	}

	// Build format 12 subtable
	subtableLen := 16 + len(groups)*12

	subtable := make([]byte, subtableLen)
	binary.BigEndian.PutUint16(subtable[0:], 12)                   // format
	binary.BigEndian.PutUint16(subtable[2:], 0)                    // reserved
	binary.BigEndian.PutUint32(subtable[4:], uint32(subtableLen))  // length
	binary.BigEndian.PutUint32(subtable[8:], 0)                    // language
	binary.BigEndian.PutUint32(subtable[12:], uint32(len(groups))) // numGroups

	off := 16
	for _, g := range groups {
		binary.BigEndian.PutUint32(subtable[off:], g.startChar)
		binary.BigEndian.PutUint32(subtable[off+4:], g.endChar)
		binary.BigEndian.PutUint32(subtable[off+8:], g.startGlyph)
		off += 12
	}

	// Build cmap header
	cmapLen := 4 + 8 + len(subtable)
	cmap := make([]byte, cmapLen)

	binary.BigEndian.PutUint16(cmap[0:], 0) // version
	binary.BigEndian.PutUint16(cmap[2:], 1) // numTables

	// Encoding record (platform 3, encoding 10 = Windows Unicode full)
	binary.BigEndian.PutUint16(cmap[4:], 3)  // platformID
	binary.BigEndian.PutUint16(cmap[6:], 10) // encodingID
	binary.BigEndian.PutUint32(cmap[8:], 12) // offset to subtable

	copy(cmap[12:], subtable)

	return cmap
}

// handleOptionalTables copies or drops optional tables.
func (p *Plan) handleOptionalTables(builder *FontBuilder) {
	// Hinting tables - required by PDF spec for TrueType fonts.
	// Copy by default unless FlagNoHinting is set.
	if p.input.Flags&FlagNoHinting == 0 {
		hintingTables := []ot.Tag{
			ot.TagCvt,
			ot.TagFpgm,
			ot.TagPrep,
		}
		for _, tag := range hintingTables {
			if p.input.ShouldDropTable(tag) {
				continue
			}
			if p.source.HasTable(tag) {
				if data, err := p.source.TableData(tag); err == nil {
					builder.AddTable(tag, data)
				}
			}
		}
	}

	// Copy OS/2 if present (important for metrics)
	if !p.input.ShouldDropTable(ot.TagOS2) && p.source.HasTable(ot.TagOS2) {
		if data, err := p.source.TableData(ot.TagOS2); err == nil {
			builder.AddTable(ot.TagOS2, data)
		}
	}

	// Variation tables - drop when instanced (axes pinned), otherwise pass through
	variationTables := []ot.Tag{
		ot.TagFvar,
		ot.TagAvar,
		ot.TagHvar,
		ot.TagVvar,
		ot.TagGvar,
		ot.TagSTAT,
		ot.TagMvar,
		ot.TagCvar,
	}
	if !p.IsInstanced() {
		// Keep variation tables if not instancing
		for _, tag := range variationTables {
			if p.input.ShouldDropTable(tag) {
				continue
			}
			if p.input.ShouldPassThrough(tag) || p.input.Flags&FlagPassUnrecognized != 0 {
				if p.source.HasTable(tag) {
					if data, err := p.source.TableData(tag); err == nil {
						builder.AddTable(tag, data)
					}
				}
			}
		}
	}
	// When instanced, variation tables are dropped (not copied)

	// Optional tables - only copy if explicitly requested or FlagPassUnrecognized is set
	optionalTables := []ot.Tag{
		ot.TagName,
		ot.TagPost,
		ot.TagGasp,
	}
	for _, tag := range optionalTables {
		if p.input.ShouldDropTable(tag) {
			continue
		}
		if p.input.ShouldPassThrough(tag) || p.input.Flags&FlagPassUnrecognized != 0 {
			if data, err := p.source.TableData(tag); err == nil {
				builder.AddTable(tag, data)
			}
		}
	}

	// GPOS and GDEF are now subsetted with glyph remapping in Execute()
}

// Subset is a convenience function that subsets a font for given codepoints.
func Subset(font *ot.Font, codepoints []rune) ([]byte, error) {
	input := NewInput()
	for _, cp := range codepoints {
		input.AddUnicode(cp)
	}

	plan, err := CreatePlan(font, input)
	if err != nil {
		return nil, err
	}

	return plan.Execute()
}

// SubsetString is a convenience function that subsets a font for a string.
func SubsetString(font *ot.Font, text string) ([]byte, error) {
	input := NewInput()
	input.AddString(text)

	plan, err := CreatePlan(font, input)
	if err != nil {
		return nil, err
	}

	return plan.Execute()
}
