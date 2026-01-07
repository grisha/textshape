package ot

// CFF DICT Operators (Top DICT and Private DICT)
// See Adobe Technical Note #5176: The Compact Font Format Specification
const (
	// Top DICT operators (single byte)
	dictVersion         = 0
	dictNotice          = 1
	dictFullName        = 2
	dictFamilyName      = 3
	dictWeight          = 4
	dictFontBBox        = 5
	dictUniqueID        = 13
	dictXUID            = 14
	dictCharset         = 15
	dictEncoding        = 16
	dictCharStrings     = 17
	dictPrivate         = 18

	// Top DICT operators (two byte, prefix 12)
	dictCopyright       = 12<<8 | 0
	dictIsFixedPitch    = 12<<8 | 1
	dictItalicAngle     = 12<<8 | 2
	dictUnderlinePos    = 12<<8 | 3
	dictUnderlineThick  = 12<<8 | 4
	dictPaintType       = 12<<8 | 5
	dictCharstringType  = 12<<8 | 6
	dictFontMatrix      = 12<<8 | 7
	dictStrokeWidth     = 12<<8 | 8
	dictSyntheticBase   = 12<<8 | 20
	dictPostScript      = 12<<8 | 21
	dictBaseFontName    = 12<<8 | 22
	dictBaseFontBlend   = 12<<8 | 23

	// CIDFont operators (two byte, prefix 12)
	dictROS             = 12<<8 | 30
	dictCIDFontVersion  = 12<<8 | 31
	dictCIDFontRevision = 12<<8 | 32
	dictCIDFontType     = 12<<8 | 33
	dictCIDCount        = 12<<8 | 34
	dictUIDBase         = 12<<8 | 35
	dictFDArray         = 12<<8 | 36
	dictFDSelect        = 12<<8 | 37
	dictFontName        = 12<<8 | 38

	// Private DICT operators (single byte)
	dictBlueValues      = 6
	dictOtherBlues      = 7
	dictFamilyBlues     = 8
	dictFamilyOtherBlues = 9
	dictStdHW           = 10
	dictStdVW           = 11
	dictSubrs           = 19 // Local Subrs offset

	// Private DICT operators (two byte, prefix 12)
	dictBlueScale       = 12<<8 | 9
	dictBlueShift       = 12<<8 | 10
	dictBlueFuzz        = 12<<8 | 11
	dictStemSnapH       = 12<<8 | 12
	dictStemSnapV       = 12<<8 | 13
	dictForceBold       = 12<<8 | 14
	dictLanguageGroup   = 12<<8 | 17
	dictExpansionFactor = 12<<8 | 18
	dictInitialRandomSeed = 12<<8 | 19
	dictDefaultWidthX   = 20
	dictNominalWidthX   = 21
)

// CharString Type 2 Operators
// See Adobe Technical Note #5177: Type 2 Charstring Format
const (
	csHstem       = 1
	csVstem       = 3
	csVmoveto     = 4
	csRlineto     = 5
	csHlineto     = 6
	csVlineto     = 7
	csRrcurveto   = 8
	csCallsubr    = 10  // Call local subroutine
	csReturn      = 11
	csEscape      = 12  // Two-byte operator prefix
	csEndchar     = 14
	csHstemhm     = 18
	csHintmask    = 19
	csCntrmask    = 20
	csRmoveto     = 21
	csHmoveto     = 22
	csVstemhm     = 23
	csRcurveline  = 24
	csRlinecurve  = 25
	csVvcurveto   = 26
	csHhcurveto   = 27
	csShortint    = 28  // 16-bit integer follows
	csCallgsubr   = 29  // Call global subroutine
	csVhcurveto   = 30
	csHvcurveto   = 31

	// Two-byte operators (prefix 12)
	csAnd         = 12<<8 | 3
	csOr          = 12<<8 | 4
	csNot         = 12<<8 | 5
	csAbs         = 12<<8 | 9
	csAdd         = 12<<8 | 10
	csSub         = 12<<8 | 11
	csDiv         = 12<<8 | 12
	csNeg         = 12<<8 | 14
	csEq          = 12<<8 | 15
	csDrop        = 12<<8 | 18
	csPut         = 12<<8 | 20
	csGet         = 12<<8 | 21
	csIfelse      = 12<<8 | 22
	csRandom      = 12<<8 | 23
	csMul         = 12<<8 | 24
	csSqrt        = 12<<8 | 26
	csDup         = 12<<8 | 27
	csExch        = 12<<8 | 28
	csIndex       = 12<<8 | 29
	csRoll        = 12<<8 | 30
	csHflex       = 12<<8 | 34
	csFlex        = 12<<8 | 35
	csHflex1      = 12<<8 | 36
	csFlex1       = 12<<8 | 37
)

// Standard CFF Strings (SID 0-390)
// Custom strings start at SID 391
const cffStdStringCount = 391

// calcSubrBias returns the subroutine bias based on the number of subroutines.
// CFF uses biased subroutine numbers to allow efficient encoding.
func calcSubrBias(count int) int {
	if count < 1240 {
		return 107
	}
	if count < 33900 {
		return 1131
	}
	return 32768
}

// encodeCFFInt encodes an integer in CFF DICT format.
// Returns the encoded bytes.
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

// encodeCSInt encodes an integer in CharString format.
// Similar to DICT format but uses different ranges.
func encodeCSInt(v int) []byte {
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
	// Use 16-bit encoding (operator 28)
	return []byte{28, byte(v >> 8), byte(v)}
}
