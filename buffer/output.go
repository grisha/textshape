package buffer

// This file implements the two-buffer system used during GSUB processing.
//
// During GSUB, the buffer operates with separate input and output:
// - Input: info[idx:len] contains glyphs to be processed
// - Output: outInfo[0:outLen] contains processed glyphs
//
// After processing, Sync() transfers output back to input.
//
// This design allows variable-length substitutions:
// - 1→1: replace_glyph
// - 1→N: output_glyph called N times, then skip_glyph
// - N→1: merge clusters, then replace_glyph
// - N→M: handled by replace_glyphs

// ClearOutput initializes the output buffer for GSUB processing.
// After calling this, use NextGlyph, ReplaceGlyph, OutputGlyph to process.
func (b *Buffer) ClearOutput() {
	b.haveOutput = true
	b.havePositions = false

	b.idx = 0
	b.outLen = 0
	b.outInfo = b.info // Start sharing (optimization)
}

// ClearPositions initializes position data for GPOS processing.
func (b *Buffer) ClearPositions() {
	b.haveOutput = false
	b.havePositions = true

	b.outLen = 0
	b.outInfo = b.info

	// Zero all positions
	for i := 0; i < b.len; i++ {
		b.pos[i] = GlyphPosition{}
	}
}

// Sync transfers output buffer to input buffer after GSUB processing.
// Returns true if successful.
func (b *Buffer) Sync() bool {
	if !b.haveOutput {
		return false
	}

	// Copy remaining input to output
	if !b.successful || !b.NextGlyphs(b.len-b.idx) {
		b.haveOutput = false
		b.outLen = 0
		b.outInfo = b.info
		b.idx = 0
		return false
	}

	// If outInfo is separate from info, swap them
	if len(b.outInfo) > 0 && len(b.info) > 0 && &b.outInfo[0] != &b.info[0] {
		// outInfo was using separate storage
		b.info, b.outInfo = b.outInfo, b.info
	}

	b.len = b.outLen

	b.haveOutput = false
	b.outLen = 0
	b.outInfo = b.info
	b.idx = 0

	return true
}

// Cur returns a pointer to the current input glyph.
func (b *Buffer) Cur(offset int) *GlyphInfo {
	return &b.info[b.idx+offset]
}

// CurPos returns a pointer to the current input glyph position.
func (b *Buffer) CurPos(offset int) *GlyphPosition {
	return &b.pos[b.idx+offset]
}

// Prev returns a pointer to the previous output glyph.
func (b *Buffer) Prev() *GlyphInfo {
	if b.outLen > 0 {
		return &b.outInfo[b.outLen-1]
	}
	return &b.outInfo[0]
}

// Idx returns the current input position.
func (b *Buffer) Idx() int {
	return b.idx
}

// OutLen returns the current output length.
func (b *Buffer) OutLen() int {
	return b.outLen
}

// BacktrackLen returns the number of glyphs available for backtracking.
func (b *Buffer) BacktrackLen() int {
	if b.haveOutput {
		return b.outLen
	}
	return b.idx
}

// LookaheadLen returns the number of glyphs available for lookahead.
func (b *Buffer) LookaheadLen() int {
	return b.len - b.idx
}

// --- Output operations ---

// NextGlyph copies the current glyph to output and advances.
// If there's no output buffer, just advances idx.
func (b *Buffer) NextGlyph() bool {
	if b.haveOutput {
		if &b.outInfo[0] != &b.info[0] || b.outLen != b.idx {
			if !b.ensure(b.outLen + 1) {
				return false
			}
			b.outInfo[b.outLen] = b.info[b.idx]
		}
		b.outLen++
	}
	b.idx++
	return true
}

// NextGlyphs copies n glyphs to output and advances.
func (b *Buffer) NextGlyphs(n int) bool {
	if b.haveOutput {
		if &b.outInfo[0] != &b.info[0] || b.outLen != b.idx {
			if !b.ensure(b.outLen + n) {
				return false
			}
			copy(b.outInfo[b.outLen:b.outLen+n], b.info[b.idx:b.idx+n])
		}
		b.outLen += n
	}
	b.idx += n
	return true
}

// SkipGlyph advances the input cursor without copying to output.
func (b *Buffer) SkipGlyph() {
	b.idx++
}

// ReplaceGlyph replaces the current input glyph with a new glyph ID.
// Equivalent to ReplaceGlyphs(1, 1, ...)
func (b *Buffer) ReplaceGlyph(glyphID Codepoint) bool {
	return b.ReplaceGlyphs(1, []Codepoint{glyphID})
}

// ReplaceGlyphs replaces numIn input glyphs with the given output glyphs.
// Handles cluster merging for the replaced glyphs.
func (b *Buffer) ReplaceGlyphs(numIn int, glyphData []Codepoint) bool {
	numOut := len(glyphData)

	if !b.makeRoomFor(numIn, numOut) {
		return false
	}

	if b.idx+numIn > b.len {
		return false
	}

	// Merge clusters of input glyphs
	b.MergeClusters(b.idx, b.idx+numIn)

	// Get original info to copy properties from
	var origInfo *GlyphInfo
	if b.idx < b.len {
		origInfo = b.Cur(0)
	} else {
		origInfo = b.Prev()
	}

	// Write output glyphs
	for i := 0; i < numOut; i++ {
		b.outInfo[b.outLen+i] = *origInfo
		b.outInfo[b.outLen+i].Codepoint = glyphData[i]
	}

	b.idx += numIn
	b.outLen += numOut
	return true
}

// OutputGlyph outputs a glyph without consuming input.
// Used for 1→N substitutions.
func (b *Buffer) OutputGlyph(glyphID Codepoint) bool {
	return b.ReplaceGlyphs(0, []Codepoint{glyphID})
}

// OutputInfo outputs a complete GlyphInfo to the output buffer.
func (b *Buffer) OutputInfo(info GlyphInfo) bool {
	if !b.makeRoomFor(0, 1) {
		return false
	}
	b.outInfo[b.outLen] = info
	b.outLen++
	return true
}

// CopyGlyph copies the current glyph to output without advancing.
func (b *Buffer) CopyGlyph() bool {
	return b.OutputInfo(b.info[b.idx])
}

// DeleteGlyph merges clusters and skips the current glyph.
// Used for removing glyphs during shaping.
func (b *Buffer) DeleteGlyph() {
	// Merge cluster of deleted glyph with neighbors
	cluster := b.info[b.idx].Cluster

	if b.outLen > 0 && cluster < b.outInfo[b.outLen-1].Cluster {
		// Merge with previous output
		for i := b.outLen; i > 0 && b.outInfo[i-1].Cluster == b.outInfo[b.outLen-1].Cluster; i-- {
			b.setCluster(&b.outInfo[i-1], cluster, 0)
		}
	}

	if b.idx+1 < b.len && cluster < b.info[b.idx+1].Cluster {
		// Merge with next input
		for i := b.idx + 1; i < b.len && b.info[i].Cluster == b.info[b.idx+1].Cluster; i++ {
			b.setCluster(&b.info[i], cluster, 0)
		}
	}

	b.SkipGlyph()
}

// makeRoomFor ensures output buffer can hold numOut more glyphs.
// If output would overtake input, splits to separate buffer.
func (b *Buffer) makeRoomFor(numIn, numOut int) bool {
	if !b.ensure(b.outLen + numOut) {
		return false
	}

	// If output is sharing with input and would overtake, split
	if len(b.outInfo) > 0 && len(b.info) > 0 && &b.outInfo[0] == &b.info[0] {
		if b.outLen+numOut > b.idx+numIn {
			if !b.haveOutput {
				return false
			}

			// Split: outInfo gets its own storage
			newOut := make([]GlyphInfo, cap(b.info))
			copy(newOut, b.outInfo[:b.outLen])
			b.outInfo = newOut
		}
	}

	return true
}

// MoveTo moves the output cursor to position i.
// Used for rewinding during contextual lookups.
func (b *Buffer) MoveTo(i int) bool {
	if !b.haveOutput {
		if i > b.len {
			return false
		}
		b.idx = i
		return true
	}

	if !b.successful {
		return false
	}

	totalLen := b.outLen + (b.len - b.idx)
	if i > totalLen {
		return false
	}

	if b.outLen < i {
		// Move forward: copy from input to output
		count := i - b.outLen
		if !b.makeRoomFor(count, count) {
			return false
		}
		copy(b.outInfo[b.outLen:], b.info[b.idx:b.idx+count])
		b.idx += count
		b.outLen += count
	} else if b.outLen > i {
		// Move backward: move from output back to input
		count := b.outLen - i
		if b.idx < count {
			// Need to shift input forward
			if !b.shiftForward(count - b.idx) {
				return false
			}
		}
		b.idx -= count
		b.outLen -= count
		copy(b.info[b.idx:], b.outInfo[b.outLen:b.outLen+count])
	}

	return true
}

// shiftForward shifts the input buffer forward by count positions.
func (b *Buffer) shiftForward(count int) bool {
	if !b.haveOutput {
		return false
	}
	if !b.ensure(b.len + count) {
		return false
	}

	b.maxOps -= b.len - b.idx
	if b.maxOps < 0 {
		b.successful = false
		return false
	}

	copy(b.info[b.idx+count:], b.info[b.idx:b.len])

	// Zero new positions
	for i := b.len; i < b.idx+count; i++ {
		b.info[i] = GlyphInfo{}
	}

	b.len += count
	b.idx += count

	return true
}

// --- Sorting ---

// Sort sorts glyphs in the given range using the provided comparison function.
func (b *Buffer) Sort(start, end int, less func(a, b *GlyphInfo) bool) {
	if start >= end {
		return
	}

	// Simple insertion sort (stable, good for small ranges)
	for i := start + 1; i < end; i++ {
		j := i
		for j > start && less(&b.info[j], &b.info[j-1]) {
			b.info[j], b.info[j-1] = b.info[j-1], b.info[j]
			if b.havePositions {
				b.pos[j], b.pos[j-1] = b.pos[j-1], b.pos[j]
			}
			j--
		}
	}
}

// --- Group iteration helpers ---

// GroupEnd returns the end of the group starting at start,
// where consecutive glyphs satisfy the sameGroup predicate.
func (b *Buffer) GroupEnd(start int, sameGroup func(a, b *GlyphInfo) bool) int {
	for start++; start < b.len && sameGroup(&b.info[start-1], &b.info[start]); start++ {
	}
	return start
}

// ClusterGroupFunc returns true if two glyphs are in the same cluster.
func ClusterGroupFunc(a, b *GlyphInfo) bool {
	return a.Cluster == b.Cluster
}
