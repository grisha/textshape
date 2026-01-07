package buffer

// SetDigest is a Bloom filter for fast glyph existence checks.
//
// During shaping, lookups use the digest to quickly skip glyphs
// that can't possibly match. This avoids expensive lookup processing
// for most glyphs.
//
// The digest is not perfectly accurate (false positives possible),
// but false negatives never occur: if MayHave returns false,
// the glyph is definitely not in the set.
type SetDigest struct {
	mask uint64
}

// Add adds a glyph ID to the digest.
func (d *SetDigest) Add(g Codepoint) {
	d.mask |= 1 << (g & 63)
}

// AddRange adds all glyph IDs in the range [first, last] to the digest.
func (d *SetDigest) AddRange(first, last Codepoint) {
	if last-first >= 63 {
		// Range covers all bits
		d.mask = ^uint64(0)
		return
	}
	for g := first; g <= last; g++ {
		d.Add(g)
	}
}

// AddArray adds multiple glyph IDs to the digest.
func (d *SetDigest) AddArray(glyphs []Codepoint) {
	for _, g := range glyphs {
		d.Add(g)
	}
}

// MayHave returns true if the glyph might be in the set.
// Returns false only if the glyph is definitely not in the set.
func (d *SetDigest) MayHave(g Codepoint) bool {
	return d.mask&(1<<(g&63)) != 0
}

// MayIntersect returns true if the two digests might have common elements.
func (d *SetDigest) MayIntersect(other SetDigest) bool {
	return d.mask&other.mask != 0
}

// Union combines this digest with another.
func (d *SetDigest) Union(other SetDigest) {
	d.mask |= other.mask
}

// Clear resets the digest to empty.
func (d *SetDigest) Clear() {
	d.mask = 0
}

// IsEmpty returns true if the digest is empty.
func (d *SetDigest) IsEmpty() bool {
	return d.mask == 0
}

// IsFull returns true if the digest covers all possible values.
func (d *SetDigest) IsFull() bool {
	return d.mask == ^uint64(0)
}
