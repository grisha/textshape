package buffer

// Buffer holds input text and output glyphs for shaping.
//
// The buffer uses a two-buffer system internally to handle GSUB substitutions
// that change the number of glyphs. During GSUB processing, glyphs are read
// from the input buffer (info) and written to the output buffer (outInfo).
// After processing, the buffers are swapped via Sync().
//
// For GPOS processing, modifications happen in-place since glyph count
// doesn't change.
type Buffer struct {
	// Public properties
	Props        SegmentProperties
	Flags        Flags
	ClusterLevel ClusterLevel

	// Replacement codepoint for invalid characters (default: U+FFFD)
	Replacement Codepoint

	// Glyph to use for invisible characters (0 = hide)
	Invisible Codepoint

	// Glyph to use when cmap lookup fails (0 = .notdef)
	NotFound Codepoint

	// Glyph for variation selector lookup failure
	NotFoundVariationSelector Codepoint

	// Content type
	contentType ContentType

	// Main glyph data
	info []GlyphInfo
	pos  []GlyphPosition

	// Output buffer (for GSUB)
	// When haveOutput is true, outInfo may point to a separate slice
	// or share storage with info (as optimization when output fits)
	outInfo []GlyphInfo

	// Lengths and cursor
	len    int // Length of info/pos
	outLen int // Length of outInfo
	idx    int // Current position in info during processing

	// State flags
	successful    bool // No allocation failures
	haveOutput    bool // Output buffer is active (GSUB mode)
	havePositions bool // Position data is valid

	// Context (characters before/after the buffer, for contextual shaping)
	context    [2][5]Codepoint // [0]=pre, [1]=post
	contextLen [2]int

	// Internal state for shaping
	serial       uint8
	scratchFlags ScratchFlags
	maxLen       int // Maximum allowed length
	maxOps       int // Maximum operations (prevents infinite loops)
	randomState  uint32

	// Digest for fast glyph filtering (Bloom filter)
	digest SetDigest
}

// New creates a new empty buffer with default settings.
func New() *Buffer {
	b := &Buffer{
		Props: SegmentProperties{
			Direction: DirectionInvalid,
			Script:    ScriptInvalid,
			Language:  LanguageInvalid,
		},
		Flags:                     FlagDefault,
		ClusterLevel:              ClusterLevelDefault,
		Replacement:               ReplacementCodepoint,
		NotFoundVariationSelector: CodepointInvalid,
		contentType:               ContentTypeInvalid,
		successful:                true,
		maxLen:                    maxLenDefault,
		maxOps:                    maxOpsDefault,
		randomState:               1,
	}
	return b
}

// Buffer size limits (from HarfBuzz)
const (
	maxLenFactor  = 64
	maxLenMin     = 16384
	maxLenDefault = 0x3FFFFFFF // ~1 billion

	maxOpsFactor  = 1024
	maxOpsMin     = 16384
	maxOpsDefault = 0x1FFFFFFF // ~500 million
)

// Reset clears the buffer and resets all settings to defaults.
func (b *Buffer) Reset() {
	b.Props = SegmentProperties{
		Direction: DirectionInvalid,
		Script:    ScriptInvalid,
		Language:  LanguageInvalid,
	}
	b.Flags = FlagDefault
	b.ClusterLevel = ClusterLevelDefault
	b.Replacement = ReplacementCodepoint
	b.Invisible = 0
	b.NotFound = 0
	b.NotFoundVariationSelector = CodepointInvalid
	b.Clear()
}

// Clear clears the buffer contents and resets segment properties.
func (b *Buffer) Clear() {
	b.contentType = ContentTypeInvalid
	b.Props = SegmentProperties{
		Direction: DirectionInvalid,
		Script:    ScriptInvalid,
		Language:  LanguageInvalid,
	}

	b.successful = true
	b.haveOutput = false
	b.havePositions = false

	b.idx = 0
	b.len = 0
	b.outLen = 0
	b.outInfo = b.info

	b.context[0] = [5]Codepoint{}
	b.context[1] = [5]Codepoint{}
	b.contextLen[0] = 0
	b.contextLen[1] = 0

	b.serial = 0
	b.scratchFlags = ScratchFlagDefault
	b.randomState = 1
}

// Len returns the number of glyphs in the buffer.
func (b *Buffer) Len() int {
	return b.len
}

// Info returns the glyph info slice.
func (b *Buffer) Info() []GlyphInfo {
	return b.info[:b.len]
}

// Pos returns the glyph position slice.
// Only valid after shaping (when HavePositions is true).
func (b *Buffer) Pos() []GlyphPosition {
	return b.pos[:b.len]
}

// HavePositions returns true if position data is available.
func (b *Buffer) HavePositions() bool {
	return b.havePositions
}

// ContentType returns the current content type.
func (b *Buffer) ContentType() ContentType {
	return b.contentType
}

// SetContentType sets the content type.
func (b *Buffer) SetContentType(ct ContentType) {
	b.contentType = ct
}

// InError returns true if an allocation or operation error occurred.
func (b *Buffer) InError() bool {
	return !b.successful
}

// --- Adding content ---

// Add adds a single codepoint with the given cluster value.
func (b *Buffer) Add(codepoint Codepoint, cluster uint32) {
	if !b.ensure(b.len + 1) {
		return
	}

	b.info[b.len] = GlyphInfo{
		Codepoint: codepoint,
		Cluster:   cluster,
	}
	b.len++
}

// AddRunes adds Unicode codepoints from a rune slice.
// Each rune gets its index as cluster value.
func (b *Buffer) AddRunes(runes []rune) {
	if !b.ensureUnicode() {
		return
	}
	for i, r := range runes {
		b.Add(Codepoint(r), uint32(i))
	}
}

// AddString adds Unicode codepoints from a string.
// Cluster values are byte offsets into the original string.
func (b *Buffer) AddString(s string) {
	if !b.ensureUnicode() {
		return
	}
	byteIdx := 0
	for _, r := range s {
		b.Add(Codepoint(r), uint32(byteIdx))
		byteIdx += len(string(r))
	}
}

// AddUTF8 adds text from a UTF-8 byte slice.
// Cluster values are byte offsets.
func (b *Buffer) AddUTF8(text []byte) {
	b.AddString(string(text))
}

// --- Internal buffer management ---

// ensure makes sure the buffer can hold at least size elements.
func (b *Buffer) ensure(size int) bool {
	if size <= cap(b.info) {
		return true
	}
	return b.enlarge(size)
}

// enlarge grows the buffer to accommodate size elements.
func (b *Buffer) enlarge(size int) bool {
	if size > b.maxLen {
		b.successful = false
		return false
	}

	if !b.successful {
		return false
	}

	// Grow by 1.5x + 32
	newAlloc := cap(b.info)
	for size >= newAlloc {
		newAlloc = newAlloc + newAlloc/2 + 32
	}

	separateOut := len(b.outInfo) > 0 && &b.outInfo[0] != &b.info[0]

	newInfo := make([]GlyphInfo, newAlloc)
	newPos := make([]GlyphPosition, newAlloc)

	copy(newInfo, b.info[:b.len])
	copy(newPos, b.pos[:b.len])

	b.info = newInfo
	b.pos = newPos

	if separateOut {
		// outInfo was separate, keep it that way
		// (it uses pos storage in HarfBuzz, we allocate separately)
		newOut := make([]GlyphInfo, newAlloc)
		copy(newOut, b.outInfo[:b.outLen])
		b.outInfo = newOut
	} else {
		b.outInfo = b.info
	}

	return true
}

// ensureUnicode ensures the buffer is in Unicode mode.
func (b *Buffer) ensureUnicode() bool {
	if b.contentType == ContentTypeUnicode {
		return true
	}
	if b.contentType != ContentTypeInvalid {
		return false
	}
	if b.len != 0 {
		return false
	}
	b.contentType = ContentTypeUnicode
	return true
}

// ensureGlyphs ensures the buffer is in Glyph mode.
func (b *Buffer) ensureGlyphs() bool {
	if b.contentType == ContentTypeGlyphs {
		return true
	}
	if b.contentType != ContentTypeInvalid {
		return false
	}
	if b.len != 0 {
		return false
	}
	b.contentType = ContentTypeGlyphs
	return true
}

// --- Direction and properties ---

// GuessSegmentProperties guesses script and direction if not set.
func (b *Buffer) GuessSegmentProperties() {
	// Guess script from first strong-scripted character
	if b.Props.Script == ScriptInvalid && b.len > 0 {
		for i := 0; i < b.len; i++ {
			script := lookupScript(b.info[i].Codepoint)
			if script != ScriptCommon && script != ScriptInvalid {
				b.Props.Script = script
				break
			}
		}
		if b.Props.Script == ScriptInvalid {
			b.Props.Script = ScriptCommon
		}
	}

	// Guess direction from script
	if b.Props.Direction == DirectionInvalid {
		b.Props.Direction = scriptGetHorizontalDirection(b.Props.Script)
		if b.Props.Direction == DirectionInvalid {
			b.Props.Direction = DirectionLTR
		}
	}
}

// lookupScript returns the script for a codepoint.
// This is a simplified version; full implementation would use Unicode data.
func lookupScript(cp Codepoint) Script {
	// Basic Latin
	if cp < 0x0080 {
		return ScriptLatin
	}
	// Arabic
	if cp >= 0x0600 && cp <= 0x06FF {
		return ScriptArabic
	}
	// Hebrew
	if cp >= 0x0590 && cp <= 0x05FF {
		return ScriptHebrew
	}
	// Greek
	if cp >= 0x0370 && cp <= 0x03FF {
		return ScriptGreek
	}
	// CJK
	if cp >= 0x4E00 && cp <= 0x9FFF {
		return ScriptHan
	}
	return ScriptCommon
}

// scriptGetHorizontalDirection returns the default direction for a script.
func scriptGetHorizontalDirection(script Script) Direction {
	// RTL scripts
	switch script {
	case ScriptArabic, ScriptHebrew:
		return DirectionRTL
	}
	return DirectionLTR
}

// Reverse reverses the buffer contents.
func (b *Buffer) Reverse() {
	b.ReverseRange(0, b.len)
}

// ReverseRange reverses a range of the buffer.
func (b *Buffer) ReverseRange(start, end int) {
	if start >= end {
		return
	}
	for i, j := start, end-1; i < j; i, j = i+1, j-1 {
		b.info[i], b.info[j] = b.info[j], b.info[i]
	}
	if b.havePositions {
		for i, j := start, end-1; i < j; i, j = i+1, j-1 {
			b.pos[i], b.pos[j] = b.pos[j], b.pos[i]
		}
	}
}

// ReverseClusters reverses cluster groups while keeping glyphs within clusters
// in their original order.
func (b *Buffer) ReverseClusters() {
	b.reverseGroups(func(a, b *GlyphInfo) bool {
		return a.Cluster == b.Cluster
	}, false)
}

func (b *Buffer) reverseGroups(sameGroup func(a, b *GlyphInfo) bool, mergeClusters bool) {
	if b.len == 0 {
		return
	}

	start := 0
	for i := 1; i <= b.len; i++ {
		if i == b.len || !sameGroup(&b.info[i-1], &b.info[i]) {
			if mergeClusters {
				b.MergeClusters(start, i)
			}
			b.ReverseRange(start, i)
			start = i
		}
	}
	b.Reverse()
}

// --- Cluster management ---

// MergeClusters merges clusters in the given range to the minimum cluster value.
func (b *Buffer) MergeClusters(start, end int) {
	if end-start < 2 {
		return
	}
	b.mergeClustersImpl(start, end)
}

func (b *Buffer) mergeClustersImpl(start, end int) {
	if !b.ClusterLevel.IsMonotone() {
		b.unsafeToBreak(start, end)
		return
	}

	b.maxOps -= end - start
	if b.maxOps < 0 {
		b.successful = false
	}

	cluster := b.info[start].Cluster
	for i := start + 1; i < end; i++ {
		if b.info[i].Cluster < cluster {
			cluster = b.info[i].Cluster
		}
	}

	// Extend end
	for end < b.len && b.info[end-1].Cluster == b.info[end].Cluster {
		end++
	}

	// Extend start
	for b.idx < start && b.info[start-1].Cluster == b.info[start].Cluster {
		start--
	}

	// Set cluster values
	for i := start; i < end; i++ {
		b.setCluster(&b.info[i], cluster, 0)
	}
}

func (b *Buffer) setCluster(info *GlyphInfo, cluster uint32, mask Mask) {
	if info.Cluster != cluster {
		info.Mask = (info.Mask &^ Mask(GlyphFlagDefined)) | (mask & Mask(GlyphFlagDefined))
	}
	info.Cluster = cluster
}

func (b *Buffer) unsafeToBreak(start, end int) {
	b.setGlyphFlags(Mask(GlyphFlagUnsafeToBreak|GlyphFlagUnsafeToConcat), start, end, true, false)
}

func (b *Buffer) setGlyphFlags(mask Mask, start, end int, interior, fromOutBuffer bool) {
	if end > b.len {
		end = b.len
	}
	if interior && !fromOutBuffer && end-start < 2 {
		return
	}

	if !fromOutBuffer || !b.haveOutput {
		if !interior {
			for i := start; i < end; i++ {
				b.info[i].Mask |= mask
			}
		} else {
			cluster := b.findMinCluster(b.info, start, end)
			b.setGlyphFlagsWithCluster(b.info, start, end, cluster, mask)
		}
	}
}

func (b *Buffer) findMinCluster(info []GlyphInfo, start, end int) uint32 {
	if start >= end {
		return 0xFFFFFFFF
	}
	if b.ClusterLevel == ClusterLevelCharacters {
		cluster := info[start].Cluster
		for i := start + 1; i < end; i++ {
			if info[i].Cluster < cluster {
				cluster = info[i].Cluster
			}
		}
		return cluster
	}
	// For monotone levels, min is at start or end
	if info[start].Cluster < info[end-1].Cluster {
		return info[start].Cluster
	}
	return info[end-1].Cluster
}

func (b *Buffer) setGlyphFlagsWithCluster(info []GlyphInfo, start, end int, cluster uint32, mask Mask) {
	for i := start; i < end; i++ {
		if info[i].Cluster != cluster {
			info[i].Mask |= mask
		}
	}
}

// --- Mask management ---

// ResetMasks sets all glyph masks to the given value.
func (b *Buffer) ResetMasks(mask Mask) {
	for i := 0; i < b.len; i++ {
		b.info[i].Mask = mask
	}
}

// AddMasks ORs the given mask into all glyph masks.
func (b *Buffer) AddMasks(mask Mask) {
	for i := 0; i < b.len; i++ {
		b.info[i].Mask |= mask
	}
}

// SetMasks sets mask bits for glyphs in a cluster range.
func (b *Buffer) SetMasks(value, mask Mask, clusterStart, clusterEnd uint32) {
	if mask == 0 {
		return
	}

	notMask := ^mask
	value &= mask

	for i := 0; i < b.len; i++ {
		if b.info[i].Cluster >= clusterStart && b.info[i].Cluster < clusterEnd {
			b.info[i].Mask = (b.info[i].Mask & notMask) | value
		}
	}
}

// --- Digest (Bloom filter for fast glyph lookup) ---

// UpdateDigest updates the Bloom filter digest with current glyphs.
func (b *Buffer) UpdateDigest() {
	b.digest = SetDigest{}
	for i := 0; i < b.len; i++ {
		b.digest.Add(b.info[i].Codepoint)
	}
}

// Digest returns the current digest.
func (b *Buffer) Digest() SetDigest {
	return b.digest
}

// --- Enter/Leave shaping ---

// Enter prepares the buffer for shaping.
func (b *Buffer) Enter() {
	b.serial = 0
	b.scratchFlags = ScratchFlagDefault

	// Set max_len based on input length
	mul := b.len * maxLenFactor
	if mul/maxLenFactor == b.len { // no overflow
		b.maxLen = max(mul, maxLenMin)
	} else {
		b.maxLen = maxLenDefault
	}

	// Set max_ops based on input length
	mul = b.len * maxOpsFactor
	if mul/maxOpsFactor == b.len { // no overflow
		b.maxOps = max(mul, maxOpsMin)
	} else {
		b.maxOps = maxOpsDefault
	}
}

// Leave cleans up after shaping.
func (b *Buffer) Leave() {
	b.maxLen = maxLenDefault
	b.maxOps = maxOpsDefault
	b.serial = 0
}

// NextSerial returns the next serial number for lookup tracking.
func (b *Buffer) NextSerial() uint8 {
	b.serial++
	if b.serial == 0 {
		b.serial = 1
	}
	return b.serial
}
