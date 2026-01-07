package subset

import (
	"sort"

	"github.com/boxesandglue/textshape/ot"
)

// Plan holds the computed glyph mapping and metadata for subsetting.
type Plan struct {
	source *ot.Font
	input  *Input

	// glyphMap maps old glyph IDs to new glyph IDs.
	glyphMap map[ot.GlyphID]ot.GlyphID

	// reverseMap maps new glyph IDs to old glyph IDs.
	reverseMap map[ot.GlyphID]ot.GlyphID

	// unicodeMap maps codepoints to new glyph IDs.
	unicodeMap map[rune]ot.GlyphID

	// glyphSet contains all old glyph IDs to retain.
	glyphSet map[ot.GlyphID]bool

	// numOutputGlyphs is the number of glyphs in the output font.
	numOutputGlyphs int

	// Parsed tables (cached for subsetting)
	cmap *ot.Cmap
	gdef *ot.GDEF
	gsub *ot.GSUB
	gpos *ot.GPOS
	hmtx *ot.Hmtx
	glyf *ot.Glyf
	cff  *ot.CFF
}

// CreatePlan creates a subset plan from a font and input configuration.
func CreatePlan(font *ot.Font, input *Input) (*Plan, error) {
	p := &Plan{
		source:     font,
		input:      input,
		glyphMap:   make(map[ot.GlyphID]ot.GlyphID),
		reverseMap: make(map[ot.GlyphID]ot.GlyphID),
		unicodeMap: make(map[rune]ot.GlyphID),
		glyphSet:   make(map[ot.GlyphID]bool),
	}

	// Parse required tables
	if err := p.parseTables(); err != nil {
		return nil, err
	}

	// Compute glyph closure
	p.computeGlyphClosure()

	// Create glyph mapping
	p.createGlyphMapping()

	return p, nil
}

// parseTables parses the font tables needed for subsetting.
func (p *Plan) parseTables() error {
	// Parse cmap (required)
	if p.source.HasTable(ot.TagCmap) {
		data, err := p.source.TableData(ot.TagCmap)
		if err != nil {
			return err
		}
		p.cmap, err = ot.ParseCmap(data)
		if err != nil {
			return err
		}
	}

	// Parse GDEF (optional)
	if p.source.HasTable(ot.TagGDEF) {
		data, _ := p.source.TableData(ot.TagGDEF)
		p.gdef, _ = ot.ParseGDEF(data)
	}

	// Parse GSUB (optional)
	if p.source.HasTable(ot.TagGSUB) {
		data, _ := p.source.TableData(ot.TagGSUB)
		p.gsub, _ = ot.ParseGSUB(data)
	}

	// Parse GPOS (optional)
	if p.source.HasTable(ot.TagGPOS) {
		data, _ := p.source.TableData(ot.TagGPOS)
		p.gpos, _ = ot.ParseGPOS(data)
	}

	// Parse hmtx (optional)
	if p.source.HasTable(ot.TagHmtx) && p.source.HasTable(ot.TagHhea) {
		p.hmtx, _ = ot.ParseHmtxFromFont(p.source)
	}

	// Parse glyf/loca (optional, for TrueType fonts)
	if p.source.HasTable(ot.TagGlyf) && p.source.HasTable(ot.TagLoca) {
		p.glyf, _ = ot.ParseGlyfFromFont(p.source)
	}

	// Parse CFF (optional, for OpenType/CFF fonts)
	if p.source.HasTable(ot.TagCFF) {
		data, _ := p.source.TableData(ot.TagCFF)
		p.cff, _ = ot.ParseCFF(data)
	}

	return nil
}

// computeGlyphClosure computes all glyphs that need to be retained.
func (p *Plan) computeGlyphClosure() {
	// Always keep .notdef (GID 0)
	p.glyphSet[0] = true

	// Add glyphs for requested Unicode codepoints
	if p.cmap != nil {
		for cp := range p.input.unicodes {
			if gid, ok := p.cmap.Lookup(ot.Codepoint(cp)); ok {
				p.glyphSet[gid] = true
			}
		}
	}

	// Add explicitly requested glyphs
	for gid := range p.input.glyphs {
		p.glyphSet[gid] = true
	}

	// Compute composite glyph closure (components)
	p.computeCompositeGlyphClosure()

	// Compute GSUB closure (unless disabled)
	if p.input.Flags&FlagNoLayoutClosure == 0 {
		p.computeGSUBClosure()
	}
}

// computeCompositeGlyphClosure adds component glyphs from composites.
func (p *Plan) computeCompositeGlyphClosure() {
	if p.glyf == nil {
		return
	}

	// Iterate until no new glyphs are added (for nested composites)
	for {
		added := false

		// Check each glyph in the set
		for gid := range p.glyphSet {
			components := p.glyf.GetComponents(gid)
			for _, comp := range components {
				if !p.glyphSet[comp] {
					p.glyphSet[comp] = true
					added = true
				}
			}
		}

		if !added {
			break
		}
	}
}

// computeGSUBClosure adds glyphs reachable through GSUB substitutions.
func (p *Plan) computeGSUBClosure() {
	if p.gsub == nil {
		return
	}

	// Iterate until no new glyphs are added
	for {
		added := false

		// Check each lookup
		for i := 0; i < p.gsub.NumLookups(); i++ {
			lookup := p.gsub.GetLookup(i)
			if lookup == nil {
				continue
			}

			// Get glyphs produced by this lookup for our current glyph set
			newGlyphs := p.getGSUBLookupOutputGlyphs(lookup)
			for gid := range newGlyphs {
				if !p.glyphSet[gid] {
					p.glyphSet[gid] = true
					added = true
				}
			}
		}

		if !added {
			break
		}
	}
}

// getGSUBLookupOutputGlyphs returns output glyphs for a lookup given current glyph set.
func (p *Plan) getGSUBLookupOutputGlyphs(lookup *ot.GSUBLookup) map[ot.GlyphID]bool {
	result := make(map[ot.GlyphID]bool)

	for _, subtable := range lookup.Subtables() {
		switch st := subtable.(type) {
		case *ot.SingleSubst:
			// Single substitution: if input glyph is in set, add output
			for inGlyph, outGlyph := range st.Mapping() {
				if p.glyphSet[inGlyph] {
					result[outGlyph] = true
				}
			}

		case *ot.MultipleSubst:
			// Multiple substitution: if input glyph is in set, add all outputs
			for inGlyph, outGlyphs := range st.Mapping() {
				if p.glyphSet[inGlyph] {
					for _, g := range outGlyphs {
						result[g] = true
					}
				}
			}

		case *ot.LigatureSubst:
			// Ligature: if ALL input glyphs are in set, add ligature glyph
			for _, ligSet := range st.LigatureSets() {
				for _, lig := range ligSet {
					// Check if all components are in glyph set
					allPresent := true
					for _, comp := range lig.Components {
						if !p.glyphSet[comp] {
							allPresent = false
							break
						}
					}
					if allPresent {
						result[lig.LigGlyph] = true
					}
				}
			}

		case *ot.AlternateSubst:
			// Alternate: if input glyph is in set, add all alternates
			for inGlyph, alternates := range st.Mapping() {
				if p.glyphSet[inGlyph] {
					for _, alt := range alternates {
						result[alt] = true
					}
				}
			}
		}
	}

	return result
}

// createGlyphMapping creates the old->new glyph ID mapping.
func (p *Plan) createGlyphMapping() {
	if p.input.Flags&FlagRetainGIDs != 0 {
		// Retain original GIDs
		p.createRetainGIDsMapping()
	} else {
		// Compact mapping (remove gaps)
		p.createCompactMapping()
	}

	// Build unicode->new GID mapping
	if p.cmap != nil {
		for cp := range p.input.unicodes {
			if oldGID, ok := p.cmap.Lookup(ot.Codepoint(cp)); ok {
				if newGID, ok := p.glyphMap[oldGID]; ok {
					p.unicodeMap[cp] = newGID
				}
			}
		}
	}
}

// createRetainGIDsMapping keeps original glyph IDs.
func (p *Plan) createRetainGIDsMapping() {
	maxGID := ot.GlyphID(0)
	for gid := range p.glyphSet {
		p.glyphMap[gid] = gid
		p.reverseMap[gid] = gid
		if gid > maxGID {
			maxGID = gid
		}
	}
	p.numOutputGlyphs = int(maxGID) + 1
}

// createCompactMapping creates a compact glyph ID mapping.
func (p *Plan) createCompactMapping() {
	// Sort glyph IDs to ensure consistent ordering
	gids := make([]ot.GlyphID, 0, len(p.glyphSet))
	for gid := range p.glyphSet {
		gids = append(gids, gid)
	}
	sort.Slice(gids, func(i, j int) bool { return gids[i] < gids[j] })

	// Create compact mapping
	for newGID, oldGID := range gids {
		p.glyphMap[oldGID] = ot.GlyphID(newGID)
		p.reverseMap[ot.GlyphID(newGID)] = oldGID
	}
	p.numOutputGlyphs = len(gids)
}

// NumOutputGlyphs returns the number of glyphs in the output font.
func (p *Plan) NumOutputGlyphs() int {
	return p.numOutputGlyphs
}

// MapGlyph maps an old glyph ID to a new glyph ID.
// Returns (0, false) if the glyph is not in the subset.
func (p *Plan) MapGlyph(oldGID ot.GlyphID) (ot.GlyphID, bool) {
	newGID, ok := p.glyphMap[oldGID]
	return newGID, ok
}

// OldGlyph returns the old glyph ID for a new glyph ID.
func (p *Plan) OldGlyph(newGID ot.GlyphID) (ot.GlyphID, bool) {
	oldGID, ok := p.reverseMap[newGID]
	return oldGID, ok
}

// GlyphSet returns the set of old glyph IDs to retain.
func (p *Plan) GlyphSet() map[ot.GlyphID]bool {
	return p.glyphSet
}

// Source returns the source font.
func (p *Plan) Source() *ot.Font {
	return p.source
}

// Input returns the input configuration.
func (p *Plan) Input() *Input {
	return p.input
}

// Cmap returns the parsed cmap table.
func (p *Plan) Cmap() *ot.Cmap {
	return p.cmap
}

// Hmtx returns the parsed hmtx table.
func (p *Plan) Hmtx() *ot.Hmtx {
	return p.hmtx
}

// Glyf returns the parsed glyf table.
func (p *Plan) Glyf() *ot.Glyf {
	return p.glyf
}

// GlyphMap returns the old->new glyph ID mapping.
func (p *Plan) GlyphMap() map[ot.GlyphID]ot.GlyphID {
	return p.glyphMap
}

// CFF returns the parsed CFF table.
func (p *Plan) CFF() *ot.CFF {
	return p.cff
}
