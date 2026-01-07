package ot

import (
	"encoding/binary"
	"sort"
)

// NotCovered is returned when a glyph is not in a coverage table.
const NotCovered = ^uint32(0)

// GSUB lookup types
const (
	GSUBTypeSingle             = 1
	GSUBTypeMultiple           = 2
	GSUBTypeAlternate          = 3
	GSUBTypeLigature           = 4
	GSUBTypeContext            = 5
	GSUBTypeChainContext       = 6
	GSUBTypeExtension          = 7
	GSUBTypeReverseChainSingle = 8
)

// Coverage represents an OpenType Coverage table.
// It maps glyph IDs to coverage indices.
type Coverage struct {
	format uint16
	data   []byte
	offset int // offset to coverage table in data

	// Format 1: sorted array of glyphs
	glyphCount int
	glyphsOff  int

	// Format 2: range records
	rangeCount int
	rangesOff  int
}

// ParseCoverage parses a Coverage table from data at the given offset.
func ParseCoverage(data []byte, offset int) (*Coverage, error) {
	if offset+4 > len(data) {
		return nil, ErrInvalidOffset
	}

	format := binary.BigEndian.Uint16(data[offset:])

	c := &Coverage{
		format: format,
		data:   data,
		offset: offset,
	}

	switch format {
	case 1:
		// Format 1: Array of GlyphIDs
		glyphCount := int(binary.BigEndian.Uint16(data[offset+2:]))
		if offset+4+glyphCount*2 > len(data) {
			return nil, ErrInvalidOffset
		}
		c.glyphCount = glyphCount
		c.glyphsOff = offset + 4
		return c, nil

	case 2:
		// Format 2: Range records
		rangeCount := int(binary.BigEndian.Uint16(data[offset+2:]))
		if offset+4+rangeCount*6 > len(data) {
			return nil, ErrInvalidOffset
		}
		c.rangeCount = rangeCount
		c.rangesOff = offset + 4
		return c, nil

	default:
		return nil, ErrInvalidFormat
	}
}

// GetCoverage returns the coverage index for a glyph ID, or NotCovered if not found.
func (c *Coverage) GetCoverage(glyph GlyphID) uint32 {
	switch c.format {
	case 1:
		return c.getCoverageFormat1(glyph)
	case 2:
		return c.getCoverageFormat2(glyph)
	default:
		return NotCovered
	}
}

// getCoverageFormat1 performs binary search on sorted glyph array.
func (c *Coverage) getCoverageFormat1(glyph GlyphID) uint32 {
	lo, hi := 0, c.glyphCount
	for lo < hi {
		mid := (lo + hi) / 2
		g := binary.BigEndian.Uint16(c.data[c.glyphsOff+mid*2:])
		if glyph < GlyphID(g) {
			hi = mid
		} else if glyph > GlyphID(g) {
			lo = mid + 1
		} else {
			return uint32(mid)
		}
	}
	return NotCovered
}

// getCoverageFormat2 performs binary search on range records.
func (c *Coverage) getCoverageFormat2(glyph GlyphID) uint32 {
	lo, hi := 0, c.rangeCount
	for lo < hi {
		mid := (lo + hi) / 2
		off := c.rangesOff + mid*6
		startGlyph := binary.BigEndian.Uint16(c.data[off:])
		endGlyph := binary.BigEndian.Uint16(c.data[off+2:])

		if glyph < GlyphID(startGlyph) {
			hi = mid
		} else if glyph > GlyphID(endGlyph) {
			lo = mid + 1
		} else {
			// Found: coverage index = startCoverageIndex + (glyph - startGlyph)
			startCoverageIndex := binary.BigEndian.Uint16(c.data[off+4:])
			return uint32(startCoverageIndex) + uint32(glyph-GlyphID(startGlyph))
		}
	}
	return NotCovered
}

// Glyphs returns all glyphs covered by this coverage table.
func (c *Coverage) Glyphs() []GlyphID {
	var glyphs []GlyphID

	switch c.format {
	case 1:
		// Format 1: sorted array of glyphs
		glyphs = make([]GlyphID, c.glyphCount)
		for i := 0; i < c.glyphCount; i++ {
			glyphs[i] = GlyphID(binary.BigEndian.Uint16(c.data[c.glyphsOff+i*2:]))
		}
	case 2:
		// Format 2: range records
		for i := 0; i < c.rangeCount; i++ {
			off := c.rangesOff + i*6
			startGlyph := GlyphID(binary.BigEndian.Uint16(c.data[off:]))
			endGlyph := GlyphID(binary.BigEndian.Uint16(c.data[off+2:]))
			for g := startGlyph; g <= endGlyph; g++ {
				glyphs = append(glyphs, g)
			}
		}
	}

	return glyphs
}

// GSUB represents the Glyph Substitution table.
type GSUB struct {
	data        []byte
	version     uint32
	scriptList  uint16 // offset to script list
	featureList uint16 // offset to feature list
	lookupList  uint16 // offset to lookup list

	// Parsed lookup list
	lookups []*GSUBLookup
}

// ParseGSUB parses a GSUB table from data.
func ParseGSUB(data []byte) (*GSUB, error) {
	if len(data) < 10 {
		return nil, ErrInvalidTable
	}

	p := NewParser(data)

	major, _ := p.U16()
	minor, _ := p.U16()
	version := uint32(major)<<16 | uint32(minor)

	if major != 1 || (minor != 0 && minor != 1) {
		return nil, ErrInvalidFormat
	}

	scriptList, _ := p.U16()
	featureList, _ := p.U16()
	lookupList, _ := p.U16()

	gsub := &GSUB{
		data:        data,
		version:     version,
		scriptList:  scriptList,
		featureList: featureList,
		lookupList:  lookupList,
	}

	// Parse lookup list
	if err := gsub.parseLookupList(); err != nil {
		return nil, err
	}

	return gsub, nil
}

// parseLookupList parses the lookup list.
func (g *GSUB) parseLookupList() error {
	off := int(g.lookupList)
	if off+2 > len(g.data) {
		return ErrInvalidOffset
	}

	lookupCount := int(binary.BigEndian.Uint16(g.data[off:]))
	if off+2+lookupCount*2 > len(g.data) {
		return ErrInvalidOffset
	}

	g.lookups = make([]*GSUBLookup, lookupCount)

	for i := 0; i < lookupCount; i++ {
		lookupOff := int(binary.BigEndian.Uint16(g.data[off+2+i*2:]))
		lookup, err := parseGSUBLookup(g.data, off+lookupOff, g)
		if err != nil {
			// Continue with nil lookup (will be skipped during application)
			continue
		}
		g.lookups[i] = lookup
	}

	return nil
}

// NumLookups returns the number of lookups in the GSUB table.
func (g *GSUB) NumLookups() int {
	return len(g.lookups)
}

// GetLookup returns the lookup at the given index.
func (g *GSUB) GetLookup(index int) *GSUBLookup {
	if index < 0 || index >= len(g.lookups) {
		return nil
	}
	return g.lookups[index]
}

// GSUBLookup represents a GSUB lookup table.
type GSUBLookup struct {
	Type       uint16
	Flag       uint16
	subtables  []GSUBSubtable
	MarkFilter uint16 // For flag & 0x10
}

// Subtables returns the lookup subtables.
func (l *GSUBLookup) Subtables() []GSUBSubtable {
	return l.subtables
}

// GSUBSubtable is the interface for GSUB lookup subtables.
type GSUBSubtable interface {
	// Apply applies the substitution to the glyph at the current position.
	// Returns the number of glyphs consumed (0 if not applied).
	Apply(ctx *GSUBContext) int
}

// parseGSUBLookup parses a single GSUB lookup.
func parseGSUBLookup(data []byte, offset int, gsub *GSUB) (*GSUBLookup, error) {
	if offset+6 > len(data) {
		return nil, ErrInvalidOffset
	}

	lookupType := binary.BigEndian.Uint16(data[offset:])
	lookupFlag := binary.BigEndian.Uint16(data[offset+2:])
	subtableCount := int(binary.BigEndian.Uint16(data[offset+4:]))

	if offset+6+subtableCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	lookup := &GSUBLookup{
		Type:      lookupType,
		Flag:      lookupFlag,
		subtables: make([]GSUBSubtable, 0, subtableCount),
	}

	// Check for MarkFilteringSet
	markFilterOff := 6 + subtableCount*2
	if lookupFlag&0x0010 != 0 {
		if offset+markFilterOff+2 > len(data) {
			return nil, ErrInvalidOffset
		}
		lookup.MarkFilter = binary.BigEndian.Uint16(data[offset+markFilterOff:])
	}

	for i := 0; i < subtableCount; i++ {
		subtableOff := int(binary.BigEndian.Uint16(data[offset+6+i*2:]))
		actualType := lookupType

		// Handle extension lookups
		if lookupType == GSUBTypeExtension {
			extOff := offset + subtableOff
			if extOff+8 > len(data) {
				continue
			}
			extFormat := binary.BigEndian.Uint16(data[extOff:])
			if extFormat != 1 {
				continue
			}
			actualType = binary.BigEndian.Uint16(data[extOff+2:])
			extOffset := binary.BigEndian.Uint32(data[extOff+4:])
			subtableOff += int(extOffset)
		}

		subtable, err := parseGSUBSubtable(data, offset+subtableOff, actualType, gsub)
		if err != nil {
			continue
		}
		if subtable != nil {
			lookup.subtables = append(lookup.subtables, subtable)
		}
	}

	return lookup, nil
}

// parseGSUBSubtable parses a GSUB subtable based on its type.
func parseGSUBSubtable(data []byte, offset int, lookupType uint16, gsub *GSUB) (GSUBSubtable, error) {
	if offset+2 > len(data) {
		return nil, ErrInvalidOffset
	}

	switch lookupType {
	case GSUBTypeSingle:
		return parseSingleSubst(data, offset)
	case GSUBTypeLigature:
		return parseLigatureSubst(data, offset)
	case GSUBTypeMultiple:
		return parseMultipleSubst(data, offset)
	case GSUBTypeAlternate:
		return parseAlternateSubst(data, offset)
	case GSUBTypeContext:
		return parseContextSubst(data, offset, gsub)
	case GSUBTypeChainContext:
		return parseChainContextSubst(data, offset, gsub)
	case GSUBTypeReverseChainSingle:
		return parseReverseChainSingleSubst(data, offset)
	default:
		// Unsupported lookup type
		return nil, nil
	}
}

// --- Single Substitution ---

// SingleSubst represents a Single Substitution subtable.
type SingleSubst struct {
	format   uint16
	coverage *Coverage

	// Format 1: delta
	delta int16

	// Format 2: substitute array
	substitutes []GlyphID
}

func parseSingleSubst(data []byte, offset int) (*SingleSubst, error) {
	if offset+6 > len(data) {
		return nil, ErrInvalidOffset
	}

	format := binary.BigEndian.Uint16(data[offset:])
	coverageOff := int(binary.BigEndian.Uint16(data[offset+2:]))

	coverage, err := ParseCoverage(data, offset+coverageOff)
	if err != nil {
		return nil, err
	}

	s := &SingleSubst{
		format:   format,
		coverage: coverage,
	}

	switch format {
	case 1:
		// Format 1: deltaGlyphID
		s.delta = int16(binary.BigEndian.Uint16(data[offset+4:]))
		return s, nil

	case 2:
		// Format 2: substitute array
		glyphCount := int(binary.BigEndian.Uint16(data[offset+4:]))
		if offset+6+glyphCount*2 > len(data) {
			return nil, ErrInvalidOffset
		}
		s.substitutes = make([]GlyphID, glyphCount)
		for i := 0; i < glyphCount; i++ {
			s.substitutes[i] = GlyphID(binary.BigEndian.Uint16(data[offset+6+i*2:]))
		}
		return s, nil

	default:
		return nil, ErrInvalidFormat
	}
}

// Apply applies the single substitution.
func (s *SingleSubst) Apply(ctx *GSUBContext) int {
	glyph := ctx.Glyphs[ctx.Index]
	coverageIndex := s.coverage.GetCoverage(glyph)
	if coverageIndex == NotCovered {
		return 0
	}

	var newGlyph GlyphID
	switch s.format {
	case 1:
		newGlyph = GlyphID(int(glyph) + int(s.delta))
	case 2:
		if int(coverageIndex) >= len(s.substitutes) {
			return 0
		}
		newGlyph = s.substitutes[coverageIndex]
	default:
		return 0
	}

	ctx.ReplaceGlyph(newGlyph)
	return 1
}

// Mapping returns all input->output glyph mappings for this substitution.
func (s *SingleSubst) Mapping() map[GlyphID]GlyphID {
	result := make(map[GlyphID]GlyphID)
	glyphs := s.coverage.Glyphs()

	switch s.format {
	case 1:
		// Format 1: apply delta to each covered glyph
		for _, glyph := range glyphs {
			result[glyph] = GlyphID(int(glyph) + int(s.delta))
		}
	case 2:
		// Format 2: direct mapping via coverage index
		for i, glyph := range glyphs {
			if i < len(s.substitutes) {
				result[glyph] = s.substitutes[i]
			}
		}
	}
	return result
}

// --- Multiple Substitution ---

// MultipleSubst represents a Multiple Substitution subtable (1 -> n).
type MultipleSubst struct {
	coverage  *Coverage
	sequences [][]GlyphID // Array of replacement sequences
}

func parseMultipleSubst(data []byte, offset int) (*MultipleSubst, error) {
	if offset+6 > len(data) {
		return nil, ErrInvalidOffset
	}

	format := binary.BigEndian.Uint16(data[offset:])
	if format != 1 {
		return nil, ErrInvalidFormat
	}

	coverageOff := int(binary.BigEndian.Uint16(data[offset+2:]))
	coverage, err := ParseCoverage(data, offset+coverageOff)
	if err != nil {
		return nil, err
	}

	seqCount := int(binary.BigEndian.Uint16(data[offset+4:]))
	if offset+6+seqCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	m := &MultipleSubst{
		coverage:  coverage,
		sequences: make([][]GlyphID, seqCount),
	}

	for i := 0; i < seqCount; i++ {
		seqOff := int(binary.BigEndian.Uint16(data[offset+6+i*2:]))
		absOff := offset + seqOff
		if absOff+2 > len(data) {
			continue
		}
		glyphCount := int(binary.BigEndian.Uint16(data[absOff:]))
		if absOff+2+glyphCount*2 > len(data) {
			continue
		}
		seq := make([]GlyphID, glyphCount)
		for j := 0; j < glyphCount; j++ {
			seq[j] = GlyphID(binary.BigEndian.Uint16(data[absOff+2+j*2:]))
		}
		m.sequences[i] = seq
	}

	return m, nil
}

// Apply applies the multiple substitution.
func (m *MultipleSubst) Apply(ctx *GSUBContext) int {
	glyph := ctx.Glyphs[ctx.Index]
	coverageIndex := m.coverage.GetCoverage(glyph)
	if coverageIndex == NotCovered {
		return 0
	}

	if int(coverageIndex) >= len(m.sequences) {
		return 0
	}

	seq := m.sequences[coverageIndex]
	if len(seq) == 0 {
		// Deletion
		ctx.DeleteGlyph()
		return 1
	}

	ctx.ReplaceGlyphs(seq)
	return 1
}

// Mapping returns the input->output mapping for glyph closure computation.
func (m *MultipleSubst) Mapping() map[GlyphID][]GlyphID {
	result := make(map[GlyphID][]GlyphID)
	glyphs := m.coverage.Glyphs()
	for i, glyph := range glyphs {
		if i < len(m.sequences) {
			result[glyph] = m.sequences[i]
		}
	}
	return result
}

// --- Alternate Substitution ---

// AlternateSubst represents an Alternate Substitution subtable (1 -> 1 from set).
// It allows choosing one glyph from a set of alternatives.
type AlternateSubst struct {
	coverage      *Coverage
	alternateSets [][]GlyphID // Array of alternate glyph sets
}

func parseAlternateSubst(data []byte, offset int) (*AlternateSubst, error) {
	if offset+6 > len(data) {
		return nil, ErrInvalidOffset
	}

	format := binary.BigEndian.Uint16(data[offset:])
	if format != 1 {
		return nil, ErrInvalidFormat
	}

	coverageOff := int(binary.BigEndian.Uint16(data[offset+2:]))
	coverage, err := ParseCoverage(data, offset+coverageOff)
	if err != nil {
		return nil, err
	}

	altSetCount := int(binary.BigEndian.Uint16(data[offset+4:]))
	if offset+6+altSetCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	a := &AlternateSubst{
		coverage:      coverage,
		alternateSets: make([][]GlyphID, altSetCount),
	}

	for i := 0; i < altSetCount; i++ {
		altSetOff := int(binary.BigEndian.Uint16(data[offset+6+i*2:]))
		absOff := offset + altSetOff
		if absOff+2 > len(data) {
			continue
		}
		glyphCount := int(binary.BigEndian.Uint16(data[absOff:]))
		if absOff+2+glyphCount*2 > len(data) {
			continue
		}
		alts := make([]GlyphID, glyphCount)
		for j := 0; j < glyphCount; j++ {
			alts[j] = GlyphID(binary.BigEndian.Uint16(data[absOff+2+j*2:]))
		}
		a.alternateSets[i] = alts
	}

	return a, nil
}

// Apply applies the alternate substitution.
// By default, it selects the first alternative (index 0).
// Use ApplyWithIndex to select a specific alternative.
func (a *AlternateSubst) Apply(ctx *GSUBContext) int {
	return a.ApplyWithIndex(ctx, 0)
}

// ApplyWithIndex applies the alternate substitution with a specific alternate index.
// altIndex is 0-based (0 = first alternate, 1 = second, etc.)
func (a *AlternateSubst) ApplyWithIndex(ctx *GSUBContext, altIndex int) int {
	glyph := ctx.Glyphs[ctx.Index]
	coverageIndex := a.coverage.GetCoverage(glyph)
	if coverageIndex == NotCovered {
		return 0
	}

	if int(coverageIndex) >= len(a.alternateSets) {
		return 0
	}

	alts := a.alternateSets[coverageIndex]
	if len(alts) == 0 {
		return 0
	}

	// Clamp altIndex to valid range
	if altIndex < 0 {
		altIndex = 0
	}
	if altIndex >= len(alts) {
		altIndex = len(alts) - 1
	}

	ctx.ReplaceGlyph(alts[altIndex])
	return 1
}

// GetAlternates returns the available alternates for a glyph.
// Returns nil if the glyph is not covered.
func (a *AlternateSubst) GetAlternates(glyph GlyphID) []GlyphID {
	coverageIndex := a.coverage.GetCoverage(glyph)
	if coverageIndex == NotCovered {
		return nil
	}
	if int(coverageIndex) >= len(a.alternateSets) {
		return nil
	}
	return a.alternateSets[coverageIndex]
}

// Mapping returns the input->alternates mapping for glyph closure computation.
func (a *AlternateSubst) Mapping() map[GlyphID][]GlyphID {
	result := make(map[GlyphID][]GlyphID)
	glyphs := a.coverage.Glyphs()
	for i, glyph := range glyphs {
		if i < len(a.alternateSets) {
			result[glyph] = a.alternateSets[i]
		}
	}
	return result
}

// --- Context Substitution ---

// ContextSubst represents a Context Substitution subtable (GSUB Type 5).
// It matches input sequences and applies nested lookups.
type ContextSubst struct {
	format uint16
	gsub   *GSUB

	// Format 1: Simple glyph contexts
	coverage *Coverage
	ruleSets [][]ContextRule // Indexed by coverage index

	// Format 2: Class-based contexts
	classDef *ClassDef
	// ruleSets also used for format 2 (indexed by class)

	// Format 3: Coverage-based contexts
	inputCoverages []*Coverage
	lookupRecords  []LookupRecord
}

// ContextRule represents a single context rule.
type ContextRule struct {
	Input         []GlyphID      // Input sequence (starting from second glyph)
	LookupRecords []LookupRecord // Lookups to apply
}

func parseContextSubst(data []byte, offset int, gsub *GSUB) (*ContextSubst, error) {
	if offset+2 > len(data) {
		return nil, ErrInvalidOffset
	}

	format := binary.BigEndian.Uint16(data[offset:])

	switch format {
	case 1:
		return parseContextFormat1(data, offset, gsub)
	case 2:
		return parseContextFormat2(data, offset, gsub)
	case 3:
		return parseContextFormat3(data, offset, gsub)
	default:
		return nil, ErrInvalidFormat
	}
}

// parseContextFormat1 parses ContextSubstFormat1 (simple glyph context).
func parseContextFormat1(data []byte, offset int, gsub *GSUB) (*ContextSubst, error) {
	if offset+6 > len(data) {
		return nil, ErrInvalidOffset
	}

	coverageOff := int(binary.BigEndian.Uint16(data[offset+2:]))
	ruleSetCount := int(binary.BigEndian.Uint16(data[offset+4:]))

	if offset+6+ruleSetCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	coverage, err := ParseCoverage(data, offset+coverageOff)
	if err != nil {
		return nil, err
	}

	cs := &ContextSubst{
		format:   1,
		gsub:     gsub,
		coverage: coverage,
		ruleSets: make([][]ContextRule, ruleSetCount),
	}

	for i := 0; i < ruleSetCount; i++ {
		ruleSetOff := int(binary.BigEndian.Uint16(data[offset+6+i*2:]))
		if ruleSetOff == 0 {
			continue
		}
		rules, err := parseContextRuleSet(data, offset+ruleSetOff)
		if err != nil {
			continue
		}
		cs.ruleSets[i] = rules
	}

	return cs, nil
}

// parseContextRuleSet parses a RuleSet (array of Rules).
func parseContextRuleSet(data []byte, offset int) ([]ContextRule, error) {
	if offset+2 > len(data) {
		return nil, ErrInvalidOffset
	}

	ruleCount := int(binary.BigEndian.Uint16(data[offset:]))
	if offset+2+ruleCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	rules := make([]ContextRule, 0, ruleCount)

	for i := 0; i < ruleCount; i++ {
		ruleOff := int(binary.BigEndian.Uint16(data[offset+2+i*2:]))
		rule, err := parseContextRule(data, offset+ruleOff)
		if err != nil {
			continue
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

// parseContextRule parses a single Rule.
func parseContextRule(data []byte, offset int) (ContextRule, error) {
	var rule ContextRule

	if offset+4 > len(data) {
		return rule, ErrInvalidOffset
	}

	// inputCount includes first glyph
	inputCount := int(binary.BigEndian.Uint16(data[offset:]))
	lookupCount := int(binary.BigEndian.Uint16(data[offset+2:]))

	inputLen := inputCount - 1
	if inputLen < 0 {
		inputLen = 0
	}

	off := offset + 4
	if off+inputLen*2 > len(data) {
		return rule, ErrInvalidOffset
	}

	rule.Input = make([]GlyphID, inputLen)
	for i := 0; i < inputLen; i++ {
		rule.Input[i] = GlyphID(binary.BigEndian.Uint16(data[off+i*2:]))
	}
	off += inputLen * 2

	if off+lookupCount*4 > len(data) {
		return rule, ErrInvalidOffset
	}

	rule.LookupRecords = make([]LookupRecord, lookupCount)
	for i := 0; i < lookupCount; i++ {
		rule.LookupRecords[i].SequenceIndex = binary.BigEndian.Uint16(data[off+i*4:])
		rule.LookupRecords[i].LookupIndex = binary.BigEndian.Uint16(data[off+i*4+2:])
	}

	return rule, nil
}

// parseContextFormat2 parses ContextSubstFormat2 (class-based context).
func parseContextFormat2(data []byte, offset int, gsub *GSUB) (*ContextSubst, error) {
	if offset+8 > len(data) {
		return nil, ErrInvalidOffset
	}

	coverageOff := int(binary.BigEndian.Uint16(data[offset+2:]))
	classDefOff := int(binary.BigEndian.Uint16(data[offset+4:]))
	ruleSetCount := int(binary.BigEndian.Uint16(data[offset+6:]))

	if offset+8+ruleSetCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	coverage, err := ParseCoverage(data, offset+coverageOff)
	if err != nil {
		return nil, err
	}

	classDef, err := ParseClassDef(data, offset+classDefOff)
	if err != nil {
		return nil, err
	}

	cs := &ContextSubst{
		format:   2,
		gsub:     gsub,
		coverage: coverage,
		classDef: classDef,
		ruleSets: make([][]ContextRule, ruleSetCount),
	}

	for i := 0; i < ruleSetCount; i++ {
		ruleSetOff := int(binary.BigEndian.Uint16(data[offset+8+i*2:]))
		if ruleSetOff == 0 {
			continue
		}
		rules, err := parseContextRuleSet(data, offset+ruleSetOff)
		if err != nil {
			continue
		}
		cs.ruleSets[i] = rules
	}

	return cs, nil
}

// parseContextFormat3 parses ContextSubstFormat3 (coverage-based context).
func parseContextFormat3(data []byte, offset int, gsub *GSUB) (*ContextSubst, error) {
	if offset+6 > len(data) {
		return nil, ErrInvalidOffset
	}

	glyphCount := int(binary.BigEndian.Uint16(data[offset+2:]))
	lookupCount := int(binary.BigEndian.Uint16(data[offset+4:]))

	if offset+6+glyphCount*2+lookupCount*4 > len(data) {
		return nil, ErrInvalidOffset
	}

	inputCoverages := make([]*Coverage, glyphCount)
	off := offset + 6
	for i := 0; i < glyphCount; i++ {
		covOff := int(binary.BigEndian.Uint16(data[off+i*2:]))
		cov, err := ParseCoverage(data, offset+covOff)
		if err != nil {
			return nil, err
		}
		inputCoverages[i] = cov
	}
	off += glyphCount * 2

	lookupRecords := make([]LookupRecord, lookupCount)
	for i := 0; i < lookupCount; i++ {
		lookupRecords[i].SequenceIndex = binary.BigEndian.Uint16(data[off+i*4:])
		lookupRecords[i].LookupIndex = binary.BigEndian.Uint16(data[off+i*4+2:])
	}

	return &ContextSubst{
		format:         3,
		gsub:           gsub,
		inputCoverages: inputCoverages,
		lookupRecords:  lookupRecords,
	}, nil
}

// Apply applies the context substitution.
func (cs *ContextSubst) Apply(ctx *GSUBContext) int {
	switch cs.format {
	case 1:
		return cs.applyFormat1(ctx)
	case 2:
		return cs.applyFormat2(ctx)
	case 3:
		return cs.applyFormat3(ctx)
	default:
		return 0
	}
}

// applyFormat1 applies ContextSubstFormat1 (simple glyph context).
func (cs *ContextSubst) applyFormat1(ctx *GSUBContext) int {
	glyph := ctx.Glyphs[ctx.Index]
	coverageIndex := cs.coverage.GetCoverage(glyph)
	if coverageIndex == NotCovered {
		return 0
	}

	if int(coverageIndex) >= len(cs.ruleSets) {
		return 0
	}

	rules := cs.ruleSets[coverageIndex]
	for _, rule := range rules {
		if cs.matchRuleFormat1(ctx, &rule) {
			cs.applyLookups(ctx, rule.LookupRecords, len(rule.Input)+1)
			return 1
		}
	}

	return 0
}

// matchRuleFormat1 checks if a ContextRule matches at the current position (Format 1).
func (cs *ContextSubst) matchRuleFormat1(ctx *GSUBContext, rule *ContextRule) bool {
	inputLen := len(rule.Input) + 1
	if ctx.Index+inputLen > len(ctx.Glyphs) {
		return false
	}

	for i, g := range rule.Input {
		if ctx.Glyphs[ctx.Index+1+i] != g {
			return false
		}
	}

	return true
}

// applyFormat2 applies ContextSubstFormat2 (class-based context).
func (cs *ContextSubst) applyFormat2(ctx *GSUBContext) int {
	glyph := ctx.Glyphs[ctx.Index]
	if cs.coverage.GetCoverage(glyph) == NotCovered {
		return 0
	}

	inputClass := cs.classDef.GetClass(glyph)
	if inputClass < 0 || inputClass >= len(cs.ruleSets) {
		return 0
	}

	rules := cs.ruleSets[inputClass]
	for _, rule := range rules {
		if cs.matchRuleFormat2(ctx, &rule) {
			cs.applyLookups(ctx, rule.LookupRecords, len(rule.Input)+1)
			return 1
		}
	}

	return 0
}

// matchRuleFormat2 checks if a ContextRule matches at the current position (Format 2).
func (cs *ContextSubst) matchRuleFormat2(ctx *GSUBContext, rule *ContextRule) bool {
	inputLen := len(rule.Input) + 1
	if ctx.Index+inputLen > len(ctx.Glyphs) {
		return false
	}

	for i, classID := range rule.Input {
		glyphClass := cs.classDef.GetClass(ctx.Glyphs[ctx.Index+1+i])
		if glyphClass != int(classID) {
			return false
		}
	}

	return true
}

// applyFormat3 applies ContextSubstFormat3 (coverage-based context).
func (cs *ContextSubst) applyFormat3(ctx *GSUBContext) int {
	inputLen := len(cs.inputCoverages)
	if inputLen == 0 {
		return 0
	}

	if ctx.Index+inputLen > len(ctx.Glyphs) {
		return 0
	}

	for i, cov := range cs.inputCoverages {
		if cov.GetCoverage(ctx.Glyphs[ctx.Index+i]) == NotCovered {
			return 0
		}
	}

	cs.applyLookups(ctx, cs.lookupRecords, inputLen)
	return 1
}

// applyLookups applies the nested lookups.
func (cs *ContextSubst) applyLookups(ctx *GSUBContext, lookupRecords []LookupRecord, inputLen int) {
	if cs.gsub == nil {
		return
	}

	for _, record := range lookupRecords {
		seqIdx := int(record.SequenceIndex)
		if seqIdx >= inputLen {
			continue
		}

		lookup := cs.gsub.GetLookup(int(record.LookupIndex))
		if lookup == nil {
			continue
		}

		// Determine mark filtering set for nested lookup
		nestedMarkFilteringSet := -1
		if lookup.Flag&LookupFlagUseMarkFilteringSet != 0 {
			nestedMarkFilteringSet = int(lookup.MarkFilter)
		}

		// Create context for nested lookup with its own flags
		nestedCtx := &GSUBContext{
			Glyphs:           ctx.Glyphs,
			Index:            ctx.Index + seqIdx,
			LookupFlag:       lookup.Flag,
			GDEF:             ctx.GDEF,
			MarkFilteringSet: nestedMarkFilteringSet,
			OnReplace:        ctx.OnReplace,
			OnReplaces:       ctx.OnReplaces,
			OnDelete:         ctx.OnDelete,
			OnLigate:         ctx.OnLigate,
		}

		if nestedCtx.Index < len(nestedCtx.Glyphs) {
			for _, subtable := range lookup.subtables {
				if subtable.Apply(nestedCtx) > 0 {
					// Update the main context's Glyphs if they changed
					ctx.Glyphs = nestedCtx.Glyphs
					break
				}
			}
		}
	}

	ctx.Index += inputLen
}

// --- Ligature Substitution ---

// LigatureSubst represents a Ligature Substitution subtable.
type LigatureSubst struct {
	coverage     *Coverage
	ligatureSets [][]Ligature
}

// Coverage returns the coverage table.
func (l *LigatureSubst) Coverage() *Coverage {
	return l.coverage
}

// LigatureSets returns the ligature sets.
func (l *LigatureSubst) LigatureSets() [][]Ligature {
	return l.ligatureSets
}

// Ligature represents a single ligature rule.
type Ligature struct {
	LigGlyph   GlyphID   // The resulting ligature glyph
	Components []GlyphID // Component glyphs (starting from second)
}

func parseLigatureSubst(data []byte, offset int) (*LigatureSubst, error) {
	if offset+6 > len(data) {
		return nil, ErrInvalidOffset
	}

	format := binary.BigEndian.Uint16(data[offset:])
	if format != 1 {
		return nil, ErrInvalidFormat
	}

	coverageOff := int(binary.BigEndian.Uint16(data[offset+2:]))
	coverage, err := ParseCoverage(data, offset+coverageOff)
	if err != nil {
		return nil, err
	}

	ligSetCount := int(binary.BigEndian.Uint16(data[offset+4:]))
	if offset+6+ligSetCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	l := &LigatureSubst{
		coverage:     coverage,
		ligatureSets: make([][]Ligature, ligSetCount),
	}

	for i := 0; i < ligSetCount; i++ {
		ligSetOff := int(binary.BigEndian.Uint16(data[offset+6+i*2:]))
		ligatures, err := parseLigatureSet(data, offset+ligSetOff)
		if err != nil {
			continue
		}
		l.ligatureSets[i] = ligatures
	}

	return l, nil
}

func parseLigatureSet(data []byte, offset int) ([]Ligature, error) {
	if offset+2 > len(data) {
		return nil, ErrInvalidOffset
	}

	ligCount := int(binary.BigEndian.Uint16(data[offset:]))
	if offset+2+ligCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	ligatures := make([]Ligature, 0, ligCount)

	for i := 0; i < ligCount; i++ {
		ligOff := int(binary.BigEndian.Uint16(data[offset+2+i*2:]))
		lig, err := parseLigature(data, offset+ligOff)
		if err != nil {
			continue
		}
		ligatures = append(ligatures, lig)
	}

	return ligatures, nil
}

func parseLigature(data []byte, offset int) (Ligature, error) {
	if offset+4 > len(data) {
		return Ligature{}, ErrInvalidOffset
	}

	ligGlyph := GlyphID(binary.BigEndian.Uint16(data[offset:]))
	compCount := int(binary.BigEndian.Uint16(data[offset+2:]))

	// compCount includes first glyph (which is in coverage), so components are compCount-1
	numComponents := compCount - 1
	if numComponents < 0 {
		numComponents = 0
	}

	if offset+4+numComponents*2 > len(data) {
		return Ligature{}, ErrInvalidOffset
	}

	lig := Ligature{
		LigGlyph:   ligGlyph,
		Components: make([]GlyphID, numComponents),
	}

	for i := 0; i < numComponents; i++ {
		lig.Components[i] = GlyphID(binary.BigEndian.Uint16(data[offset+4+i*2:]))
	}

	return lig, nil
}

// Apply applies the ligature substitution.
func (l *LigatureSubst) Apply(ctx *GSUBContext) int {
	glyph := ctx.Glyphs[ctx.Index]
	coverageIndex := l.coverage.GetCoverage(glyph)
	if coverageIndex == NotCovered {
		return 0
	}

	if int(coverageIndex) >= len(l.ligatureSets) {
		return 0
	}

	ligSet := l.ligatureSets[coverageIndex]

	// Try each ligature in order of preference
	for _, lig := range ligSet {
		if l.matchLigature(ctx, &lig) {
			// Apply ligature
			ctx.Ligate(lig.LigGlyph, len(lig.Components)+1)
			return 1
		}
	}

	return 0
}

// matchLigature checks if the ligature matches at the current position.
func (l *LigatureSubst) matchLigature(ctx *GSUBContext, lig *Ligature) bool {
	if ctx.Index+len(lig.Components)+1 > len(ctx.Glyphs) {
		return false
	}

	for i, comp := range lig.Components {
		// TODO: Handle ignoreMarks flag properly
		if ctx.Glyphs[ctx.Index+1+i] != comp {
			return false
		}
	}

	return true
}

// --- GSUBContext ---

// GSUBContext provides context for GSUB application.
type GSUBContext struct {
	Glyphs []GlyphID // Current glyph sequence
	Index  int       // Current position

	// Lookup filtering
	LookupFlag       uint16 // Current lookup flag
	GDEF             *GDEF  // GDEF table for glyph classification (may be nil)
	MarkFilteringSet int    // Mark filtering set index (-1 if not used)

	// Output callbacks
	OnReplace  func(index int, newGlyph GlyphID)
	OnReplaces func(index int, newGlyphs []GlyphID)
	OnDelete   func(index int)
	OnLigate   func(index int, ligGlyph GlyphID, numGlyphs int)
}

// ShouldSkipGlyph returns true if the glyph at the given index should be skipped
// based on the current LookupFlag and GDEF glyph classification.
func (ctx *GSUBContext) ShouldSkipGlyph(index int) bool {
	if index < 0 || index >= len(ctx.Glyphs) {
		return true
	}
	return shouldSkipGlyph(ctx.Glyphs[index], ctx.LookupFlag, ctx.GDEF, ctx.MarkFilteringSet)
}

// NextGlyph returns the index of the next glyph that should not be skipped,
// starting from startIndex. Returns -1 if no such glyph exists.
func (ctx *GSUBContext) NextGlyph(startIndex int) int {
	for i := startIndex; i < len(ctx.Glyphs); i++ {
		if !ctx.ShouldSkipGlyph(i) {
			return i
		}
	}
	return -1
}

// PrevGlyph returns the index of the previous glyph that should not be skipped,
// starting from startIndex (exclusive). Returns -1 if no such glyph exists.
func (ctx *GSUBContext) PrevGlyph(startIndex int) int {
	for i := startIndex - 1; i >= 0; i-- {
		if !ctx.ShouldSkipGlyph(i) {
			return i
		}
	}
	return -1
}

// ReplaceGlyph replaces the current glyph.
func (ctx *GSUBContext) ReplaceGlyph(newGlyph GlyphID) {
	if ctx.OnReplace != nil {
		ctx.OnReplace(ctx.Index, newGlyph)
	}
	ctx.Glyphs[ctx.Index] = newGlyph
	ctx.Index++
}

// ReplaceGlyphs replaces the current glyph with multiple glyphs.
func (ctx *GSUBContext) ReplaceGlyphs(newGlyphs []GlyphID) {
	if ctx.OnReplaces != nil {
		ctx.OnReplaces(ctx.Index, newGlyphs)
	}

	if len(newGlyphs) == 0 {
		ctx.DeleteGlyph()
		return
	}

	if len(newGlyphs) == 1 {
		ctx.Glyphs[ctx.Index] = newGlyphs[0]
		ctx.Index++
		return
	}

	// Replace 1 glyph with multiple
	oldGlyphs := ctx.Glyphs
	newLen := len(oldGlyphs) - 1 + len(newGlyphs)
	result := make([]GlyphID, newLen)

	copy(result, oldGlyphs[:ctx.Index])
	copy(result[ctx.Index:], newGlyphs)
	copy(result[ctx.Index+len(newGlyphs):], oldGlyphs[ctx.Index+1:])

	ctx.Glyphs = result
	ctx.Index += len(newGlyphs)
}

// DeleteGlyph deletes the current glyph.
func (ctx *GSUBContext) DeleteGlyph() {
	if ctx.OnDelete != nil {
		ctx.OnDelete(ctx.Index)
	}
	ctx.Glyphs = append(ctx.Glyphs[:ctx.Index], ctx.Glyphs[ctx.Index+1:]...)
}

// Ligate replaces numGlyphs at current position with a ligature.
func (ctx *GSUBContext) Ligate(ligGlyph GlyphID, numGlyphs int) {
	if ctx.OnLigate != nil {
		ctx.OnLigate(ctx.Index, ligGlyph, numGlyphs)
	}

	if numGlyphs <= 1 {
		ctx.ReplaceGlyph(ligGlyph)
		return
	}

	// Remove numGlyphs-1 glyphs and replace first with ligature
	oldGlyphs := ctx.Glyphs
	newLen := len(oldGlyphs) - numGlyphs + 1
	result := make([]GlyphID, newLen)

	copy(result, oldGlyphs[:ctx.Index])
	result[ctx.Index] = ligGlyph
	copy(result[ctx.Index+1:], oldGlyphs[ctx.Index+numGlyphs:])

	ctx.Glyphs = result
	ctx.Index++
}

// --- Feature/Script lookup ---

// FeatureList represents a GSUB/GPOS FeatureList.
type FeatureList struct {
	data   []byte
	offset int
	count  int
}

// ParseFeatureList parses a FeatureList from a GSUB/GPOS table.
func (g *GSUB) ParseFeatureList() (*FeatureList, error) {
	off := int(g.featureList)
	if off+2 > len(g.data) {
		return nil, ErrInvalidOffset
	}

	count := int(binary.BigEndian.Uint16(g.data[off:]))
	if off+2+count*6 > len(g.data) {
		return nil, ErrInvalidOffset
	}

	return &FeatureList{
		data:   g.data,
		offset: off,
		count:  count,
	}, nil
}

// FeatureRecord represents a parsed feature record with its lookup indices.
// This is the internal representation from the font's FeatureList table.
type FeatureRecord struct {
	Tag     Tag
	Lookups []uint16
}

// GetFeature returns the feature record at the given index.
func (f *FeatureList) GetFeature(index int) (*FeatureRecord, error) {
	if index < 0 || index >= f.count {
		return nil, ErrInvalidOffset
	}

	recordOff := f.offset + 2 + index*6
	tag := Tag(binary.BigEndian.Uint32(f.data[recordOff:]))
	featureOff := int(binary.BigEndian.Uint16(f.data[recordOff+4:]))

	absOff := f.offset + featureOff
	if absOff+4 > len(f.data) {
		return nil, ErrInvalidOffset
	}

	// Skip featureParams offset
	lookupCount := int(binary.BigEndian.Uint16(f.data[absOff+2:]))
	if absOff+4+lookupCount*2 > len(f.data) {
		return nil, ErrInvalidOffset
	}

	feat := &FeatureRecord{
		Tag:     tag,
		Lookups: make([]uint16, lookupCount),
	}

	for i := 0; i < lookupCount; i++ {
		feat.Lookups[i] = binary.BigEndian.Uint16(f.data[absOff+4+i*2:])
	}

	return feat, nil
}

// FindFeature finds a feature by tag and returns its lookup indices.
func (f *FeatureList) FindFeature(tag Tag) []uint16 {
	// Collect unique lookup indices from all features with matching tag
	lookupSet := make(map[uint16]bool)
	for i := 0; i < f.count; i++ {
		feat, err := f.GetFeature(i)
		if err != nil {
			continue
		}
		if feat.Tag == tag {
			for _, idx := range feat.Lookups {
				lookupSet[idx] = true
			}
		}
	}

	if len(lookupSet) == 0 {
		return nil
	}

	// Convert to sorted slice
	lookups := make([]uint16, 0, len(lookupSet))
	for idx := range lookupSet {
		lookups = append(lookups, idx)
	}
	// Sort to ensure consistent application order
	for i := 0; i < len(lookups)-1; i++ {
		for j := i + 1; j < len(lookups); j++ {
			if lookups[j] < lookups[i] {
				lookups[i], lookups[j] = lookups[j], lookups[i]
			}
		}
	}
	return lookups
}

// Count returns the number of features.
func (f *FeatureList) Count() int {
	return f.count
}

// --- Apply lookup ---

// ApplyLookup applies a single lookup to the glyph sequence.
// This version does not use GDEF for glyph filtering.
func (g *GSUB) ApplyLookup(lookupIndex int, glyphs []GlyphID) []GlyphID {
	return g.ApplyLookupWithGDEF(lookupIndex, glyphs, nil)
}

// ApplyLookupWithGDEF applies a single lookup with GDEF-based glyph filtering.
func (g *GSUB) ApplyLookupWithGDEF(lookupIndex int, glyphs []GlyphID, gdef *GDEF) []GlyphID {
	lookup := g.GetLookup(lookupIndex)
	if lookup == nil {
		return glyphs
	}

	// Determine mark filtering set index
	markFilteringSet := -1
	if lookup.Flag&LookupFlagUseMarkFilteringSet != 0 {
		markFilteringSet = int(lookup.MarkFilter)
	}

	ctx := &GSUBContext{
		Glyphs:           glyphs,
		Index:            0,
		LookupFlag:       lookup.Flag,
		GDEF:             gdef,
		MarkFilteringSet: markFilteringSet,
	}

	for ctx.Index < len(ctx.Glyphs) {
		// Skip glyphs that should be ignored based on LookupFlag and GDEF
		if ctx.ShouldSkipGlyph(ctx.Index) {
			ctx.Index++
			continue
		}

		applied := false
		for _, subtable := range lookup.subtables {
			if subtable.Apply(ctx) > 0 {
				applied = true
				break
			}
		}
		if !applied {
			ctx.Index++
		}
	}

	return ctx.Glyphs
}

// ApplyFeature applies all lookups for a feature to the glyph sequence.
// This version does not use GDEF for glyph filtering.
func (g *GSUB) ApplyFeature(tag Tag, glyphs []GlyphID) []GlyphID {
	return g.ApplyFeatureWithGDEF(tag, glyphs, nil)
}

// ApplyFeatureWithGDEF applies all lookups for a feature with GDEF-based glyph filtering.
func (g *GSUB) ApplyFeatureWithGDEF(tag Tag, glyphs []GlyphID, gdef *GDEF) []GlyphID {
	featureList, err := g.ParseFeatureList()
	if err != nil {
		return glyphs
	}

	lookups := featureList.FindFeature(tag)
	if lookups == nil {
		return glyphs
	}

	// Sort lookups by index (they should be applied in order)
	sorted := make([]int, len(lookups))
	for i, l := range lookups {
		sorted[i] = int(l)
	}
	sort.Ints(sorted)

	for _, lookupIdx := range sorted {
		glyphs = g.ApplyLookupWithGDEF(lookupIdx, glyphs, gdef)
	}

	return glyphs
}

// Common feature tags
var (
	TagLiga = MakeTag('l', 'i', 'g', 'a') // Standard Ligatures
	TagClig = MakeTag('c', 'l', 'i', 'g') // Contextual Ligatures
	TagDlig = MakeTag('d', 'l', 'i', 'g') // Discretionary Ligatures
	TagHlig = MakeTag('h', 'l', 'i', 'g') // Historical Ligatures
	TagCcmp = MakeTag('c', 'c', 'm', 'p') // Glyph Composition/Decomposition
	TagLocl = MakeTag('l', 'o', 'c', 'l') // Localized Forms
	TagRlig = MakeTag('r', 'l', 'i', 'g') // Required Ligatures
	TagSmcp = MakeTag('s', 'm', 'c', 'p') // Small Capitals
	TagCalt = MakeTag('c', 'a', 'l', 't') // Contextual Alternates
)

// --- LookupRecord ---

// LookupRecord specifies a lookup to apply at a specific position.
type LookupRecord struct {
	SequenceIndex uint16 // Index into current glyph sequence (0-based)
	LookupIndex   uint16 // Lookup to apply
}

// --- ChainContextSubst ---

// ChainContextSubst represents a Chaining Context Substitution subtable (GSUB Type 6).
// It enables substitution based on surrounding context (backtrack, input, lookahead).
type ChainContextSubst struct {
	format uint16
	gsub   *GSUB // Reference to parent GSUB for nested lookup application

	// Format 1: Simple glyph contexts
	coverage      *Coverage
	chainRuleSets [][]ChainRule // Indexed by coverage index

	// Format 2: Class-based contexts
	backtrackClassDef *ClassDef
	inputClassDef     *ClassDef
	lookaheadClassDef *ClassDef
	// chainRuleSets also used for format 2 (indexed by input class)

	// Format 3: Coverage-based contexts
	backtrackCoverages []*Coverage
	inputCoverages     []*Coverage
	lookaheadCoverages []*Coverage
	lookupRecords      []LookupRecord
}

// ChainRule represents a single chaining context rule.
type ChainRule struct {
	Backtrack     []GlyphID      // Backtrack sequence (in reverse order)
	Input         []GlyphID      // Input sequence (starting from second glyph)
	Lookahead     []GlyphID      // Lookahead sequence
	LookupRecords []LookupRecord // Lookups to apply
}

func parseChainContextSubst(data []byte, offset int, gsub *GSUB) (*ChainContextSubst, error) {
	if offset+2 > len(data) {
		return nil, ErrInvalidOffset
	}

	format := binary.BigEndian.Uint16(data[offset:])

	switch format {
	case 1:
		return parseChainContextFormat1(data, offset, gsub)
	case 2:
		return parseChainContextFormat2(data, offset, gsub)
	case 3:
		return parseChainContextFormat3(data, offset, gsub)
	default:
		return nil, ErrInvalidFormat
	}
}

// parseChainContextFormat1 parses ChainContextSubstFormat1 (simple glyph context).
func parseChainContextFormat1(data []byte, offset int, gsub *GSUB) (*ChainContextSubst, error) {
	if offset+6 > len(data) {
		return nil, ErrInvalidOffset
	}

	coverageOff := int(binary.BigEndian.Uint16(data[offset+2:]))
	chainRuleSetCount := int(binary.BigEndian.Uint16(data[offset+4:]))

	if offset+6+chainRuleSetCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	coverage, err := ParseCoverage(data, offset+coverageOff)
	if err != nil {
		return nil, err
	}

	ccs := &ChainContextSubst{
		format:        1,
		gsub:          gsub,
		coverage:      coverage,
		chainRuleSets: make([][]ChainRule, chainRuleSetCount),
	}

	for i := 0; i < chainRuleSetCount; i++ {
		ruleSetOff := int(binary.BigEndian.Uint16(data[offset+6+i*2:]))
		if ruleSetOff == 0 {
			continue // NULL offset
		}
		rules, err := parseChainRuleSet(data, offset+ruleSetOff)
		if err != nil {
			continue
		}
		ccs.chainRuleSets[i] = rules
	}

	return ccs, nil
}

// parseChainRuleSet parses a ChainRuleSet (array of ChainRules).
func parseChainRuleSet(data []byte, offset int) ([]ChainRule, error) {
	if offset+2 > len(data) {
		return nil, ErrInvalidOffset
	}

	ruleCount := int(binary.BigEndian.Uint16(data[offset:]))
	if offset+2+ruleCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	rules := make([]ChainRule, 0, ruleCount)

	for i := 0; i < ruleCount; i++ {
		ruleOff := int(binary.BigEndian.Uint16(data[offset+2+i*2:]))
		rule, err := parseChainRule(data, offset+ruleOff)
		if err != nil {
			continue
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

// parseChainRule parses a single ChainRule.
func parseChainRule(data []byte, offset int) (ChainRule, error) {
	var rule ChainRule
	off := offset

	if off+2 > len(data) {
		return rule, ErrInvalidOffset
	}

	// Backtrack count and array
	backtrackCount := int(binary.BigEndian.Uint16(data[off:]))
	off += 2
	if off+backtrackCount*2 > len(data) {
		return rule, ErrInvalidOffset
	}
	rule.Backtrack = make([]GlyphID, backtrackCount)
	for i := 0; i < backtrackCount; i++ {
		rule.Backtrack[i] = GlyphID(binary.BigEndian.Uint16(data[off+i*2:]))
	}
	off += backtrackCount * 2

	// Input count and array (count includes first glyph)
	if off+2 > len(data) {
		return rule, ErrInvalidOffset
	}
	inputCount := int(binary.BigEndian.Uint16(data[off:]))
	off += 2
	inputLen := inputCount - 1 // First glyph is covered by coverage table
	if inputLen < 0 {
		inputLen = 0
	}
	if off+inputLen*2 > len(data) {
		return rule, ErrInvalidOffset
	}
	rule.Input = make([]GlyphID, inputLen)
	for i := 0; i < inputLen; i++ {
		rule.Input[i] = GlyphID(binary.BigEndian.Uint16(data[off+i*2:]))
	}
	off += inputLen * 2

	// Lookahead count and array
	if off+2 > len(data) {
		return rule, ErrInvalidOffset
	}
	lookaheadCount := int(binary.BigEndian.Uint16(data[off:]))
	off += 2
	if off+lookaheadCount*2 > len(data) {
		return rule, ErrInvalidOffset
	}
	rule.Lookahead = make([]GlyphID, lookaheadCount)
	for i := 0; i < lookaheadCount; i++ {
		rule.Lookahead[i] = GlyphID(binary.BigEndian.Uint16(data[off+i*2:]))
	}
	off += lookaheadCount * 2

	// Lookup records
	if off+2 > len(data) {
		return rule, ErrInvalidOffset
	}
	lookupCount := int(binary.BigEndian.Uint16(data[off:]))
	off += 2
	if off+lookupCount*4 > len(data) {
		return rule, ErrInvalidOffset
	}
	rule.LookupRecords = make([]LookupRecord, lookupCount)
	for i := 0; i < lookupCount; i++ {
		rule.LookupRecords[i].SequenceIndex = binary.BigEndian.Uint16(data[off+i*4:])
		rule.LookupRecords[i].LookupIndex = binary.BigEndian.Uint16(data[off+i*4+2:])
	}

	return rule, nil
}

// parseChainContextFormat2 parses ChainContextSubstFormat2 (class-based context).
func parseChainContextFormat2(data []byte, offset int, gsub *GSUB) (*ChainContextSubst, error) {
	if offset+12 > len(data) {
		return nil, ErrInvalidOffset
	}

	coverageOff := int(binary.BigEndian.Uint16(data[offset+2:]))
	backtrackClassDefOff := int(binary.BigEndian.Uint16(data[offset+4:]))
	inputClassDefOff := int(binary.BigEndian.Uint16(data[offset+6:]))
	lookaheadClassDefOff := int(binary.BigEndian.Uint16(data[offset+8:]))
	chainRuleSetCount := int(binary.BigEndian.Uint16(data[offset+10:]))

	if offset+12+chainRuleSetCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	coverage, err := ParseCoverage(data, offset+coverageOff)
	if err != nil {
		return nil, err
	}

	backtrackClassDef, err := ParseClassDef(data, offset+backtrackClassDefOff)
	if err != nil {
		return nil, err
	}

	inputClassDef, err := ParseClassDef(data, offset+inputClassDefOff)
	if err != nil {
		return nil, err
	}

	lookaheadClassDef, err := ParseClassDef(data, offset+lookaheadClassDefOff)
	if err != nil {
		return nil, err
	}

	ccs := &ChainContextSubst{
		format:            2,
		gsub:              gsub,
		coverage:          coverage,
		backtrackClassDef: backtrackClassDef,
		inputClassDef:     inputClassDef,
		lookaheadClassDef: lookaheadClassDef,
		chainRuleSets:     make([][]ChainRule, chainRuleSetCount),
	}

	for i := 0; i < chainRuleSetCount; i++ {
		ruleSetOff := int(binary.BigEndian.Uint16(data[offset+12+i*2:]))
		if ruleSetOff == 0 {
			continue // NULL offset
		}
		rules, err := parseChainRuleSet(data, offset+ruleSetOff)
		if err != nil {
			continue
		}
		ccs.chainRuleSets[i] = rules
	}

	return ccs, nil
}

// parseChainContextFormat3 parses ChainContextSubstFormat3 (coverage-based context).
func parseChainContextFormat3(data []byte, offset int, gsub *GSUB) (*ChainContextSubst, error) {
	off := offset + 2 // Skip format

	if off+2 > len(data) {
		return nil, ErrInvalidOffset
	}

	// Backtrack coverages
	backtrackCount := int(binary.BigEndian.Uint16(data[off:]))
	off += 2
	if off+backtrackCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	backtrackCoverages := make([]*Coverage, backtrackCount)
	for i := 0; i < backtrackCount; i++ {
		covOff := int(binary.BigEndian.Uint16(data[off+i*2:]))
		cov, err := ParseCoverage(data, offset+covOff)
		if err != nil {
			return nil, err
		}
		backtrackCoverages[i] = cov
	}
	off += backtrackCount * 2

	// Input coverages
	if off+2 > len(data) {
		return nil, ErrInvalidOffset
	}
	inputCount := int(binary.BigEndian.Uint16(data[off:]))
	off += 2
	if off+inputCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	inputCoverages := make([]*Coverage, inputCount)
	for i := 0; i < inputCount; i++ {
		covOff := int(binary.BigEndian.Uint16(data[off+i*2:]))
		cov, err := ParseCoverage(data, offset+covOff)
		if err != nil {
			return nil, err
		}
		inputCoverages[i] = cov
	}
	off += inputCount * 2

	// Lookahead coverages
	if off+2 > len(data) {
		return nil, ErrInvalidOffset
	}
	lookaheadCount := int(binary.BigEndian.Uint16(data[off:]))
	off += 2
	if off+lookaheadCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	lookaheadCoverages := make([]*Coverage, lookaheadCount)
	for i := 0; i < lookaheadCount; i++ {
		covOff := int(binary.BigEndian.Uint16(data[off+i*2:]))
		cov, err := ParseCoverage(data, offset+covOff)
		if err != nil {
			return nil, err
		}
		lookaheadCoverages[i] = cov
	}
	off += lookaheadCount * 2

	// Lookup records
	if off+2 > len(data) {
		return nil, ErrInvalidOffset
	}
	lookupCount := int(binary.BigEndian.Uint16(data[off:]))
	off += 2
	if off+lookupCount*4 > len(data) {
		return nil, ErrInvalidOffset
	}

	lookupRecords := make([]LookupRecord, lookupCount)
	for i := 0; i < lookupCount; i++ {
		lookupRecords[i].SequenceIndex = binary.BigEndian.Uint16(data[off+i*4:])
		lookupRecords[i].LookupIndex = binary.BigEndian.Uint16(data[off+i*4+2:])
	}

	return &ChainContextSubst{
		format:             3,
		gsub:               gsub,
		backtrackCoverages: backtrackCoverages,
		inputCoverages:     inputCoverages,
		lookaheadCoverages: lookaheadCoverages,
		lookupRecords:      lookupRecords,
	}, nil
}

// Apply applies the chaining context substitution.
func (ccs *ChainContextSubst) Apply(ctx *GSUBContext) int {
	switch ccs.format {
	case 1:
		return ccs.applyFormat1(ctx)
	case 2:
		return ccs.applyFormat2(ctx)
	case 3:
		return ccs.applyFormat3(ctx)
	default:
		return 0
	}
}

// applyFormat1 applies ChainContextSubstFormat1 (simple glyph context).
func (ccs *ChainContextSubst) applyFormat1(ctx *GSUBContext) int {
	glyph := ctx.Glyphs[ctx.Index]
	coverageIndex := ccs.coverage.GetCoverage(glyph)
	if coverageIndex == NotCovered {
		return 0
	}

	if int(coverageIndex) >= len(ccs.chainRuleSets) {
		return 0
	}

	rules := ccs.chainRuleSets[coverageIndex]
	for _, rule := range rules {
		if ccs.matchRuleFormat1(ctx, &rule) {
			ccs.applyLookups(ctx, rule.LookupRecords, len(rule.Input)+1)
			return 1
		}
	}

	return 0
}

// matchRuleFormat1 checks if a ChainRule matches at the current position (Format 1).
func (ccs *ChainContextSubst) matchRuleFormat1(ctx *GSUBContext, rule *ChainRule) bool {
	// Check if enough glyphs for input sequence
	inputLen := len(rule.Input) + 1 // +1 for first glyph (covered by coverage)
	if ctx.Index+inputLen > len(ctx.Glyphs) {
		return false
	}

	// Match input sequence (starting from second glyph)
	for i, g := range rule.Input {
		if ctx.Glyphs[ctx.Index+1+i] != g {
			return false
		}
	}

	// Check lookahead
	lookaheadStart := ctx.Index + inputLen
	if lookaheadStart+len(rule.Lookahead) > len(ctx.Glyphs) {
		return false
	}
	for i, g := range rule.Lookahead {
		if ctx.Glyphs[lookaheadStart+i] != g {
			return false
		}
	}

	// Check backtrack (in reverse order)
	if ctx.Index < len(rule.Backtrack) {
		return false
	}
	for i, g := range rule.Backtrack {
		// Backtrack[0] is immediately before current position
		if ctx.Glyphs[ctx.Index-1-i] != g {
			return false
		}
	}

	return true
}

// applyFormat2 applies ChainContextSubstFormat2 (class-based context).
func (ccs *ChainContextSubst) applyFormat2(ctx *GSUBContext) int {
	glyph := ctx.Glyphs[ctx.Index]
	if ccs.coverage.GetCoverage(glyph) == NotCovered {
		return 0
	}

	// Get class of current glyph
	inputClass := ccs.inputClassDef.GetClass(glyph)
	if inputClass < 0 || inputClass >= len(ccs.chainRuleSets) {
		return 0
	}

	rules := ccs.chainRuleSets[inputClass]
	for _, rule := range rules {
		if ccs.matchRuleFormat2(ctx, &rule) {
			ccs.applyLookups(ctx, rule.LookupRecords, len(rule.Input)+1)
			return 1
		}
	}

	return 0
}

// matchRuleFormat2 checks if a ChainRule matches at the current position (Format 2).
// In Format 2, rule values are class IDs, not glyph IDs.
func (ccs *ChainContextSubst) matchRuleFormat2(ctx *GSUBContext, rule *ChainRule) bool {
	// Check if enough glyphs for input sequence
	inputLen := len(rule.Input) + 1
	if ctx.Index+inputLen > len(ctx.Glyphs) {
		return false
	}

	// Match input sequence by class (starting from second glyph)
	for i, classID := range rule.Input {
		glyphClass := ccs.inputClassDef.GetClass(ctx.Glyphs[ctx.Index+1+i])
		if glyphClass != int(classID) {
			return false
		}
	}

	// Check lookahead by class
	lookaheadStart := ctx.Index + inputLen
	if lookaheadStart+len(rule.Lookahead) > len(ctx.Glyphs) {
		return false
	}
	for i, classID := range rule.Lookahead {
		glyphClass := ccs.lookaheadClassDef.GetClass(ctx.Glyphs[lookaheadStart+i])
		if glyphClass != int(classID) {
			return false
		}
	}

	// Check backtrack by class (in reverse order)
	if ctx.Index < len(rule.Backtrack) {
		return false
	}
	for i, classID := range rule.Backtrack {
		glyphClass := ccs.backtrackClassDef.GetClass(ctx.Glyphs[ctx.Index-1-i])
		if glyphClass != int(classID) {
			return false
		}
	}

	return true
}

// applyFormat3 applies ChainContextSubstFormat3 (coverage-based context).
func (ccs *ChainContextSubst) applyFormat3(ctx *GSUBContext) int {
	inputLen := len(ccs.inputCoverages)
	if inputLen == 0 {
		return 0
	}

	// Check if enough glyphs for input sequence
	if ctx.Index+inputLen > len(ctx.Glyphs) {
		return 0
	}

	// Match input sequence by coverage
	for i, cov := range ccs.inputCoverages {
		if cov.GetCoverage(ctx.Glyphs[ctx.Index+i]) == NotCovered {
			return 0
		}
	}

	// Check lookahead
	lookaheadStart := ctx.Index + inputLen
	if lookaheadStart+len(ccs.lookaheadCoverages) > len(ctx.Glyphs) {
		return 0
	}
	for i, cov := range ccs.lookaheadCoverages {
		if cov.GetCoverage(ctx.Glyphs[lookaheadStart+i]) == NotCovered {
			return 0
		}
	}

	// Check backtrack (in reverse order)
	if ctx.Index < len(ccs.backtrackCoverages) {
		return 0
	}
	for i, cov := range ccs.backtrackCoverages {
		if cov.GetCoverage(ctx.Glyphs[ctx.Index-1-i]) == NotCovered {
			return 0
		}
	}

	// Apply lookups
	ccs.applyLookups(ctx, ccs.lookupRecords, inputLen)
	return 1
}

// applyLookups applies the nested lookups specified in the lookup records.
func (ccs *ChainContextSubst) applyLookups(ctx *GSUBContext, lookupRecords []LookupRecord, inputLen int) {
	if ccs.gsub == nil {
		return
	}

	// Apply lookups in order
	// Note: We need to track position shifts as glyphs may be added/removed
	for _, record := range lookupRecords {
		seqIdx := int(record.SequenceIndex)
		if seqIdx >= inputLen {
			continue
		}

		lookup := ccs.gsub.GetLookup(int(record.LookupIndex))
		if lookup == nil {
			continue
		}

		// Determine mark filtering set for nested lookup
		nestedMarkFilteringSet := -1
		if lookup.Flag&LookupFlagUseMarkFilteringSet != 0 {
			nestedMarkFilteringSet = int(lookup.MarkFilter)
		}

		// Create context for nested lookup with its own flags
		nestedCtx := &GSUBContext{
			Glyphs:           ctx.Glyphs,
			Index:            ctx.Index + seqIdx,
			LookupFlag:       lookup.Flag,
			GDEF:             ctx.GDEF,
			MarkFilteringSet: nestedMarkFilteringSet,
			OnReplace:        ctx.OnReplace,
			OnReplaces:       ctx.OnReplaces,
			OnDelete:         ctx.OnDelete,
			OnLigate:         ctx.OnLigate,
		}

		if nestedCtx.Index < len(nestedCtx.Glyphs) {
			for _, subtable := range lookup.subtables {
				if subtable.Apply(nestedCtx) > 0 {
					// Update the main context's Glyphs if they changed
					ctx.Glyphs = nestedCtx.Glyphs
					break
				}
			}
		}
	}

	// Advance past the input sequence
	ctx.Index += inputLen
}

// --- Reverse Chain Single Substitution ---

// ReverseChainSingleSubst represents a Reverse Chaining Context Single Substitution subtable (GSUB Type 8).
// It is designed to be applied in reverse (from end to beginning of buffer).
// Unlike ChainContextSubst, it only performs single glyph substitution (no nested lookups).
type ReverseChainSingleSubst struct {
	coverage           *Coverage
	backtrackCoverages []*Coverage
	lookaheadCoverages []*Coverage
	substitutes        []GlyphID
}

func parseReverseChainSingleSubst(data []byte, offset int) (*ReverseChainSingleSubst, error) {
	if offset+6 > len(data) {
		return nil, ErrInvalidOffset
	}

	format := binary.BigEndian.Uint16(data[offset:])
	if format != 1 {
		return nil, ErrInvalidFormat
	}

	coverageOff := int(binary.BigEndian.Uint16(data[offset+2:]))
	coverage, err := ParseCoverage(data, offset+coverageOff)
	if err != nil {
		return nil, err
	}

	off := offset + 4

	// Backtrack coverages
	if off+2 > len(data) {
		return nil, ErrInvalidOffset
	}
	backtrackCount := int(binary.BigEndian.Uint16(data[off:]))
	off += 2

	if off+backtrackCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	backtrackCoverages := make([]*Coverage, backtrackCount)
	for i := 0; i < backtrackCount; i++ {
		covOff := int(binary.BigEndian.Uint16(data[off+i*2:]))
		cov, err := ParseCoverage(data, offset+covOff)
		if err != nil {
			return nil, err
		}
		backtrackCoverages[i] = cov
	}
	off += backtrackCount * 2

	// Lookahead coverages
	if off+2 > len(data) {
		return nil, ErrInvalidOffset
	}
	lookaheadCount := int(binary.BigEndian.Uint16(data[off:]))
	off += 2

	if off+lookaheadCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	lookaheadCoverages := make([]*Coverage, lookaheadCount)
	for i := 0; i < lookaheadCount; i++ {
		covOff := int(binary.BigEndian.Uint16(data[off+i*2:]))
		cov, err := ParseCoverage(data, offset+covOff)
		if err != nil {
			return nil, err
		}
		lookaheadCoverages[i] = cov
	}
	off += lookaheadCount * 2

	// Substitute glyphs
	if off+2 > len(data) {
		return nil, ErrInvalidOffset
	}
	substituteCount := int(binary.BigEndian.Uint16(data[off:]))
	off += 2

	if off+substituteCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	substitutes := make([]GlyphID, substituteCount)
	for i := 0; i < substituteCount; i++ {
		substitutes[i] = GlyphID(binary.BigEndian.Uint16(data[off+i*2:]))
	}

	return &ReverseChainSingleSubst{
		coverage:           coverage,
		backtrackCoverages: backtrackCoverages,
		lookaheadCoverages: lookaheadCoverages,
		substitutes:        substitutes,
	}, nil
}

// Apply applies the reverse chaining context single substitution.
// This lookup is intended to be applied in reverse (from end to beginning of buffer).
// It replaces the current glyph if it matches the coverage and context.
func (r *ReverseChainSingleSubst) Apply(ctx *GSUBContext) int {
	glyph := ctx.Glyphs[ctx.Index]
	coverageIndex := r.coverage.GetCoverage(glyph)
	if coverageIndex == NotCovered {
		return 0
	}

	if int(coverageIndex) >= len(r.substitutes) {
		return 0
	}

	// Match backtrack (in reverse order, looking backwards from current position)
	if ctx.Index < len(r.backtrackCoverages) {
		return 0
	}
	for i, cov := range r.backtrackCoverages {
		if cov.GetCoverage(ctx.Glyphs[ctx.Index-1-i]) == NotCovered {
			return 0
		}
	}

	// Match lookahead (looking forward from current position)
	lookaheadStart := ctx.Index + 1
	if lookaheadStart+len(r.lookaheadCoverages) > len(ctx.Glyphs) {
		return 0
	}
	for i, cov := range r.lookaheadCoverages {
		if cov.GetCoverage(ctx.Glyphs[lookaheadStart+i]) == NotCovered {
			return 0
		}
	}

	// Replace glyph in place (don't advance index - reverse lookup handles this)
	ctx.Glyphs[ctx.Index] = r.substitutes[coverageIndex]
	return 1
}

// ApplyLookupReverse applies this lookup in reverse order through the glyph buffer.
// This is the intended way to use ReverseChainSingleSubst.
// This version does not use GDEF for glyph filtering.
func (g *GSUB) ApplyLookupReverse(lookupIndex int, glyphs []GlyphID) []GlyphID {
	return g.ApplyLookupReverseWithGDEF(lookupIndex, glyphs, nil)
}

// ApplyLookupReverseWithGDEF applies this lookup in reverse order with GDEF-based glyph filtering.
func (g *GSUB) ApplyLookupReverseWithGDEF(lookupIndex int, glyphs []GlyphID, gdef *GDEF) []GlyphID {
	lookup := g.GetLookup(lookupIndex)
	if lookup == nil {
		return glyphs
	}

	// Only Type 8 should be applied in reverse
	if lookup.Type != GSUBTypeReverseChainSingle {
		return g.ApplyLookupWithGDEF(lookupIndex, glyphs, gdef)
	}

	// Determine mark filtering set index
	markFilteringSet := -1
	if lookup.Flag&LookupFlagUseMarkFilteringSet != 0 {
		markFilteringSet = int(lookup.MarkFilter)
	}

	ctx := &GSUBContext{
		Glyphs:           glyphs,
		LookupFlag:       lookup.Flag,
		GDEF:             gdef,
		MarkFilteringSet: markFilteringSet,
	}

	// Apply in reverse order
	for ctx.Index = len(ctx.Glyphs) - 1; ctx.Index >= 0; ctx.Index-- {
		// Skip glyphs that should be ignored based on LookupFlag and GDEF
		if ctx.ShouldSkipGlyph(ctx.Index) {
			continue
		}

		for _, subtable := range lookup.subtables {
			if subtable.Apply(ctx) > 0 {
				break
			}
		}
	}

	return ctx.Glyphs
}
