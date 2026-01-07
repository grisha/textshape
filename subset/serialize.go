package subset

import (
	"encoding/binary"
	"sort"

	"github.com/boxesandglue/textshape/ot"
)

// FontBuilder builds a new font from subset tables.
type FontBuilder struct {
	tables map[ot.Tag][]byte
}

// NewFontBuilder creates a new FontBuilder.
func NewFontBuilder() *FontBuilder {
	return &FontBuilder{
		tables: make(map[ot.Tag][]byte),
	}
}

// AddTable adds or replaces a table in the font.
func (b *FontBuilder) AddTable(tag ot.Tag, data []byte) {
	b.tables[tag] = data
}

// HasTable returns true if the table exists.
func (b *FontBuilder) HasTable(tag ot.Tag) bool {
	_, ok := b.tables[tag]
	return ok
}

// Build produces the final font binary.
func (b *FontBuilder) Build() ([]byte, error) {
	if len(b.tables) == 0 {
		return nil, ErrNoTables
	}

	// Sort table tags for deterministic output
	tags := make([]ot.Tag, 0, len(b.tables))
	for tag := range b.tables {
		tags = append(tags, tag)
	}
	sort.Slice(tags, func(i, j int) bool { return tags[i] < tags[j] })

	numTables := len(tags)

	// Calculate search range parameters
	searchRange, entrySelector, rangeShift := calcSearchParams(numTables)

	// Calculate total size
	// Offset table: 12 bytes
	// Table records: 16 bytes each
	headerSize := 12 + numTables*16

	// Align header to 4 bytes (should already be aligned)
	if headerSize%4 != 0 {
		headerSize += 4 - (headerSize % 4)
	}

	// Calculate table data size (each table padded to 4 bytes)
	dataSize := 0
	for _, tag := range tags {
		tableLen := len(b.tables[tag])
		dataSize += tableLen
		// Pad to 4-byte boundary
		if tableLen%4 != 0 {
			dataSize += 4 - (tableLen % 4)
		}
	}

	// Allocate output buffer
	totalSize := headerSize + dataSize
	out := make([]byte, totalSize)

	// Write offset table
	binary.BigEndian.PutUint32(out[0:], 0x00010000) // sfntVersion for TrueType
	binary.BigEndian.PutUint16(out[4:], uint16(numTables))
	binary.BigEndian.PutUint16(out[6:], searchRange)
	binary.BigEndian.PutUint16(out[8:], entrySelector)
	binary.BigEndian.PutUint16(out[10:], rangeShift)

	// Write table records and copy table data
	offset := headerSize
	recordOff := 12

	for _, tag := range tags {
		data := b.tables[tag]

		// Calculate checksum
		checksum := calcChecksum(data)

		// Write table record
		binary.BigEndian.PutUint32(out[recordOff:], uint32(tag))
		binary.BigEndian.PutUint32(out[recordOff+4:], checksum)
		binary.BigEndian.PutUint32(out[recordOff+8:], uint32(offset))
		binary.BigEndian.PutUint32(out[recordOff+12:], uint32(len(data)))
		recordOff += 16

		// Copy table data
		copy(out[offset:], data)
		offset += len(data)

		// Pad to 4-byte boundary
		for offset%4 != 0 {
			out[offset] = 0
			offset++
		}
	}

	// Calculate and set checksumAdjustment in head table
	if headData, ok := b.tables[ot.TagHead]; ok && len(headData) >= 12 {
		// Find head table offset in output
		headOffset := -1
		recOff := 12
		for _, tag := range tags {
			if tag == ot.TagHead {
				headOffset = int(binary.BigEndian.Uint32(out[recOff+8:]))
				break
			}
			recOff += 16
		}

		if headOffset >= 0 {
			// Zero out checksumAdjustment before calculating
			binary.BigEndian.PutUint32(out[headOffset+8:], 0)

			// Calculate full font checksum
			fontChecksum := calcChecksum(out)

			// checksumAdjustment = 0xB1B0AFBA - fontChecksum
			adjustment := uint32(0xB1B0AFBA) - fontChecksum
			binary.BigEndian.PutUint32(out[headOffset+8:], adjustment)
		}
	}

	return out, nil
}

// calcSearchParams calculates the search range parameters for the offset table.
func calcSearchParams(numTables int) (searchRange, entrySelector, rangeShift uint16) {
	// Find largest power of 2 <= numTables
	entrySelector = 0
	power := 1
	for power*2 <= numTables {
		power *= 2
		entrySelector++
	}
	searchRange = uint16(power * 16)
	rangeShift = uint16(numTables*16) - searchRange
	return
}

// calcChecksum calculates the OpenType table checksum.
func calcChecksum(data []byte) uint32 {
	var sum uint32
	// Process in 4-byte chunks
	length := len(data)
	for i := 0; i+4 <= length; i += 4 {
		sum += binary.BigEndian.Uint32(data[i:])
	}
	// Handle remaining bytes (if any)
	remaining := length % 4
	if remaining > 0 {
		var last uint32
		offset := length - remaining
		for i := 0; i < remaining; i++ {
			last |= uint32(data[offset+i]) << (24 - i*8)
		}
		sum += last
	}
	return sum
}
