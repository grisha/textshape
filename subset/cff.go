package subset

import (
	"bytes"
	"encoding/binary"
	"sort"

	"github.com/boxesandglue/textshape/ot"
)

// subsetCFF creates a subsetted CFF table.
func (p *Plan) subsetCFF() ([]byte, error) {
	cff := p.cff
	if cff == nil {
		return nil, nil
	}

	// 1. Collect CharStrings for kept glyphs
	usedCharStrings := make([][]byte, p.numOutputGlyphs)
	for newGID := 0; newGID < p.numOutputGlyphs; newGID++ {
		oldGID := p.reverseMap[ot.GlyphID(newGID)]
		if int(oldGID) < len(cff.CharStrings) {
			usedCharStrings[newGID] = cff.CharStrings[oldGID]
		} else {
			// Empty CharString for missing glyphs
			usedCharStrings[newGID] = []byte{14} // endchar
		}
	}

	// 2. Find used subroutines (closure)
	interp := ot.NewCharStringInterpreter(cff.GlobalSubrs, cff.LocalSubrs)
	for _, cs := range usedCharStrings {
		interp.FindUsedSubroutines(cs)
	}

	// Also process subroutines themselves (recursive closure)
	changed := true
	for changed {
		changed = false
		for subrNum := range interp.UsedGlobalSubrs {
			if subrNum >= 0 && subrNum < len(cff.GlobalSubrs) {
				beforeLocal := len(interp.UsedLocalSubrs)
				beforeGlobal := len(interp.UsedGlobalSubrs)
				interp.FindUsedSubroutines(cff.GlobalSubrs[subrNum])
				if len(interp.UsedLocalSubrs) > beforeLocal || len(interp.UsedGlobalSubrs) > beforeGlobal {
					changed = true
				}
			}
		}
		for subrNum := range interp.UsedLocalSubrs {
			if subrNum >= 0 && subrNum < len(cff.LocalSubrs) {
				beforeLocal := len(interp.UsedLocalSubrs)
				beforeGlobal := len(interp.UsedGlobalSubrs)
				interp.FindUsedSubroutines(cff.LocalSubrs[subrNum])
				if len(interp.UsedLocalSubrs) > beforeLocal || len(interp.UsedGlobalSubrs) > beforeGlobal {
					changed = true
				}
			}
		}
	}

	// 3. Build subroutine remapping
	globalSubrMap := buildSubrMap(interp.UsedGlobalSubrs)
	localSubrMap := buildSubrMap(interp.UsedLocalSubrs)

	// 4. Calculate new biases
	oldGlobalBias := calcSubrBias(len(cff.GlobalSubrs))
	oldLocalBias := calcSubrBias(len(cff.LocalSubrs))
	newGlobalBias := calcSubrBias(len(globalSubrMap))
	newLocalBias := calcSubrBias(len(localSubrMap))

	// 5. Remap CharStrings with new subroutine numbers
	for i, cs := range usedCharStrings {
		usedCharStrings[i] = ot.RemapCharString(cs, globalSubrMap, localSubrMap,
			oldGlobalBias, oldLocalBias, newGlobalBias, newLocalBias)
	}

	// 6. Build subset subroutines
	newGlobalSubrs := subsetSubrs(cff.GlobalSubrs, globalSubrMap, localSubrMap,
		oldGlobalBias, oldLocalBias, newGlobalBias, newLocalBias)
	newLocalSubrs := subsetSubrs(cff.LocalSubrs, localSubrMap, globalSubrMap,
		oldLocalBias, oldGlobalBias, newLocalBias, newGlobalBias)

	// 7. Build new Charset (Format 0 - simple array)
	newCharset := buildCFFCharset(p.numOutputGlyphs)

	// 8. Serialize
	return serializeCFF(cff, usedCharStrings, newGlobalSubrs, newLocalSubrs, newCharset)
}

// buildSubrMap creates a mapping from old subroutine numbers to new (consecutive) numbers.
func buildSubrMap(used map[int]bool) map[int]int {
	if len(used) == 0 {
		return nil
	}

	// Sort used subroutine numbers
	nums := make([]int, 0, len(used))
	for n := range used {
		nums = append(nums, n)
	}
	sort.Ints(nums)

	// Create consecutive mapping
	m := make(map[int]int, len(nums))
	for i, n := range nums {
		m[n] = i
	}
	return m
}

// subsetSubrs extracts and remaps used subroutines.
func subsetSubrs(subrs [][]byte, primaryMap, secondaryMap map[int]int,
	oldPrimaryBias, oldSecondaryBias, newPrimaryBias, newSecondaryBias int) [][]byte {
	if len(primaryMap) == 0 {
		return nil
	}

	// Sort by new index
	type entry struct {
		oldNum int
		newNum int
	}
	entries := make([]entry, 0, len(primaryMap))
	for old, new := range primaryMap {
		entries = append(entries, entry{old, new})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].newNum < entries[j].newNum
	})

	result := make([][]byte, len(entries))
	for _, e := range entries {
		if e.oldNum >= 0 && e.oldNum < len(subrs) {
			// Remap subroutine calls within the subroutine
			remapped := ot.RemapCharString(subrs[e.oldNum], secondaryMap, primaryMap,
				oldSecondaryBias, oldPrimaryBias, newSecondaryBias, newPrimaryBias)
			result[e.newNum] = remapped
		} else {
			result[e.newNum] = []byte{11} // return
		}
	}
	return result
}

// buildCFFCharset builds a Format 0 charset (simple SID array).
func buildCFFCharset(numGlyphs int) []byte {
	if numGlyphs <= 1 {
		return []byte{0} // Format 0, only .notdef
	}

	// Format 0: format byte + (numGlyphs-1) * 2 bytes for SIDs
	buf := make([]byte, 1+(numGlyphs-1)*2)
	buf[0] = 0 // Format 0

	// Assign sequential SIDs starting from 1 (0 is .notdef)
	for i := 1; i < numGlyphs; i++ {
		binary.BigEndian.PutUint16(buf[1+(i-1)*2:], uint16(i))
	}
	return buf
}

// serializeCFF writes the complete CFF table.
func serializeCFF(original *ot.CFF, charStrings [][]byte,
	globalSubrs, localSubrs [][]byte, charset []byte) ([]byte, error) {

	var buf bytes.Buffer

	// Phase 1: Calculate sizes and offsets
	// We need to know offsets before writing Top DICT

	// Header: 4 bytes
	headerSize := 4

	// Name INDEX
	nameData := [][]byte{[]byte(original.Name)}
	nameINDEX := buildINDEX(nameData)

	// String INDEX (empty for now - we use predefined SIDs)
	stringINDEX := buildINDEX(nil)

	// Global Subrs INDEX
	globalSubrsINDEX := buildINDEX(globalSubrs)

	// CharStrings INDEX
	charStringsINDEX := buildINDEX(charStrings)

	// Private DICT (we need to build this to know its size)
	var privateDict bytes.Buffer
	localSubrsOffset := 0

	// Copy relevant Private DICT values
	if len(original.PrivateDict.BlueValues) > 0 {
		writeIntArray(&privateDict, original.PrivateDict.BlueValues, 6) // BlueValues
	}
	if len(original.PrivateDict.OtherBlues) > 0 {
		writeIntArray(&privateDict, original.PrivateDict.OtherBlues, 7) // OtherBlues
	}
	if original.PrivateDict.StdHW != 0 {
		writeDictInt(&privateDict, original.PrivateDict.StdHW, 10) // StdHW
	}
	if original.PrivateDict.StdVW != 0 {
		writeDictInt(&privateDict, original.PrivateDict.StdVW, 11) // StdVW
	}
	if original.PrivateDict.DefaultWidthX != 0 {
		writeDictInt(&privateDict, original.PrivateDict.DefaultWidthX, 20) // defaultWidthX
	}
	if original.PrivateDict.NominalWidthX != 0 {
		writeDictInt(&privateDict, original.PrivateDict.NominalWidthX, 21) // nominalWidthX
	}

	// Local Subrs offset (relative to Private DICT start)
	localSubrsINDEX := buildINDEX(localSubrs)
	if len(localSubrs) > 0 {
		localSubrsOffset = privateDict.Len() + 5 // Account for the Subrs operator itself
		// We'll add the Subrs operator at the end of Private DICT
	}

	privateDictSize := privateDict.Len()
	if localSubrsOffset > 0 {
		privateDictSize += 5 // Space for Subrs offset operator
	}

	// Calculate offsets
	// Header
	// Name INDEX
	// Top DICT INDEX (size TBD)
	// String INDEX
	// Global Subrs INDEX
	// Charset
	// CharStrings INDEX
	// Private DICT
	// Local Subrs INDEX

	// Build a minimal Top DICT to calculate its size first
	topDictEntries := map[int][]int{
		17: {0}, // CharStrings (placeholder)
		15: {0}, // charset (placeholder)
		18: {privateDictSize, 0}, // Private (size, offset placeholder)
	}

	// Estimate Top DICT size
	estimatedTopDictSize := 50 // Rough estimate

	// Calculate actual offsets
	offset := headerSize
	offset += len(nameINDEX)

	_ = offset // topDictOffset not directly used
	offset += 2 + 1 + 2 + estimatedTopDictSize // INDEX overhead + data

	offset += len(stringINDEX)
	offset += len(globalSubrsINDEX)

	charsetOffset := offset
	offset += len(charset)

	charStringsOffset := offset
	offset += len(charStringsINDEX)

	privateDictOffset := offset
	offset += privateDictSize

	// Now build the actual Top DICT with correct offsets
	topDictEntries[17] = []int{charStringsOffset}
	topDictEntries[15] = []int{charsetOffset}
	topDictEntries[18] = []int{privateDictSize, privateDictOffset}

	topDictData := buildTopDict(topDictEntries)
	topDictINDEX := buildINDEX([][]byte{topDictData})

	// Recalculate if Top DICT size changed significantly
	actualTopDictIndexSize := len(topDictINDEX)
	expectedTopDictIndexSize := 2 + 1 + 2 + estimatedTopDictSize

	if actualTopDictIndexSize != expectedTopDictIndexSize {
		// Recalculate offsets with actual size
		sizeDiff := actualTopDictIndexSize - expectedTopDictIndexSize

		charsetOffset += sizeDiff
		charStringsOffset += sizeDiff
		privateDictOffset += sizeDiff

		topDictEntries[17] = []int{charStringsOffset}
		topDictEntries[15] = []int{charsetOffset}
		topDictEntries[18] = []int{privateDictSize, privateDictOffset}

		topDictData = buildTopDict(topDictEntries)
		topDictINDEX = buildINDEX([][]byte{topDictData})
	}

	// Phase 2: Write the CFF data

	// Header
	buf.WriteByte(1) // major version
	buf.WriteByte(0) // minor version
	buf.WriteByte(4) // header size
	buf.WriteByte(1) // offSize (minimum)

	// Name INDEX
	buf.Write(nameINDEX)

	// Top DICT INDEX
	buf.Write(topDictINDEX)

	// String INDEX
	buf.Write(stringINDEX)

	// Global Subrs INDEX
	buf.Write(globalSubrsINDEX)

	// Charset
	buf.Write(charset)

	// CharStrings INDEX
	buf.Write(charStringsINDEX)

	// Private DICT
	privateDict.Reset()
	if len(original.PrivateDict.BlueValues) > 0 {
		writeIntArray(&privateDict, original.PrivateDict.BlueValues, 6)
	}
	if len(original.PrivateDict.OtherBlues) > 0 {
		writeIntArray(&privateDict, original.PrivateDict.OtherBlues, 7)
	}
	if original.PrivateDict.StdHW != 0 {
		writeDictInt(&privateDict, original.PrivateDict.StdHW, 10)
	}
	if original.PrivateDict.StdVW != 0 {
		writeDictInt(&privateDict, original.PrivateDict.StdVW, 11)
	}
	if original.PrivateDict.DefaultWidthX != 0 {
		writeDictInt(&privateDict, original.PrivateDict.DefaultWidthX, 20)
	}
	if original.PrivateDict.NominalWidthX != 0 {
		writeDictInt(&privateDict, original.PrivateDict.NominalWidthX, 21)
	}
	if len(localSubrs) > 0 {
		// Local Subrs offset (relative to start of Private DICT)
		subrsOffset := privateDict.Len() + 5 // +5 for this operator itself
		writeDictInt(&privateDict, subrsOffset, 19) // Subrs
	}
	buf.Write(privateDict.Bytes())

	// Local Subrs INDEX
	if len(localSubrs) > 0 {
		buf.Write(localSubrsINDEX)
	}

	return buf.Bytes(), nil
}

// buildINDEX creates a CFF INDEX structure.
func buildINDEX(data [][]byte) []byte {
	count := len(data)

	if count == 0 {
		// Empty INDEX: just count = 0
		return []byte{0, 0}
	}

	// Calculate total data size
	totalSize := 0
	for _, d := range data {
		totalSize += len(d)
	}

	// Determine offset size
	offSize := 1
	if totalSize+1 > 255 {
		offSize = 2
	}
	if totalSize+1 > 65535 {
		offSize = 3
	}
	if totalSize+1 > 16777215 {
		offSize = 4
	}

	// Build INDEX
	// count(2) + offSize(1) + offsets((count+1)*offSize) + data
	indexSize := 2 + 1 + (count+1)*offSize + totalSize
	buf := make([]byte, indexSize)

	// Count
	binary.BigEndian.PutUint16(buf[0:], uint16(count))
	// OffSize
	buf[2] = byte(offSize)

	// Offsets (1-based)
	offset := 1
	for i := 0; i <= count; i++ {
		writeOffset(buf[3+i*offSize:], offset, offSize)
		if i < count {
			offset += len(data[i])
		}
	}

	// Data
	dataStart := 3 + (count+1)*offSize
	pos := dataStart
	for _, d := range data {
		copy(buf[pos:], d)
		pos += len(d)
	}

	return buf
}

func writeOffset(buf []byte, offset, size int) {
	switch size {
	case 1:
		buf[0] = byte(offset)
	case 2:
		binary.BigEndian.PutUint16(buf, uint16(offset))
	case 3:
		buf[0] = byte(offset >> 16)
		buf[1] = byte(offset >> 8)
		buf[2] = byte(offset)
	case 4:
		binary.BigEndian.PutUint32(buf, uint32(offset))
	}
}

// buildTopDict creates a Top DICT with the given entries.
func buildTopDict(entries map[int][]int) []byte {
	var buf bytes.Buffer

	// Write entries in a consistent order
	keys := make([]int, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Ints(keys)

	for _, op := range keys {
		vals := entries[op]
		for _, v := range vals {
			buf.Write(encodeCFFInt(v))
		}
		if op >= 256 {
			buf.WriteByte(12)
			buf.WriteByte(byte(op & 0xff))
		} else {
			buf.WriteByte(byte(op))
		}
	}

	return buf.Bytes()
}

// writeDictInt writes an integer operand followed by an operator.
func writeDictInt(buf *bytes.Buffer, val int, op int) {
	buf.Write(encodeCFFInt(val))
	if op >= 256 {
		buf.WriteByte(12)
		buf.WriteByte(byte(op & 0xff))
	} else {
		buf.WriteByte(byte(op))
	}
}

// writeIntArray writes an array of integers followed by an operator.
func writeIntArray(buf *bytes.Buffer, vals []int, op int) {
	for _, v := range vals {
		buf.Write(encodeCFFInt(v))
	}
	if op >= 256 {
		buf.WriteByte(12)
		buf.WriteByte(byte(op & 0xff))
	} else {
		buf.WriteByte(byte(op))
	}
}

// encodeCFFInt encodes an integer in CFF DICT format.
func encodeCFFInt(v int) []byte {
	if v >= -107 && v <= 107 {
		return []byte{byte(v + 139)}
	}
	if v >= 108 && v <= 1131 {
		v -= 108
		return []byte{byte(v/256 + 247), byte(v % 256)}
	}
	if v >= -1131 && v <= -108 {
		v = -v - 108
		return []byte{byte(v/256 + 251), byte(v % 256)}
	}
	if v >= -32768 && v <= 32767 {
		return []byte{28, byte(v >> 8), byte(v)}
	}
	// 4-byte integer
	return []byte{29, byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}
}

// calcSubrBias calculates subroutine bias based on count.
func calcSubrBias(count int) int {
	if count < 1240 {
		return 107
	}
	if count < 33900 {
		return 1131
	}
	return 32768
}
