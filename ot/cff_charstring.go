package ot

import (
	"encoding/binary"
)

// CharStringInterpreter interprets CFF Type 2 CharStrings to find subroutine usage.
type CharStringInterpreter struct {
	globalSubrs [][]byte
	localSubrs  [][]byte
	globalBias  int
	localBias   int

	// Closure tracking
	UsedGlobalSubrs map[int]bool
	UsedLocalSubrs  map[int]bool

	// Execution state
	stack     []int
	callStack []csCallFrame
	hintCount int
}

type csCallFrame struct {
	data []byte
	pos  int
}

// NewCharStringInterpreter creates a new interpreter for finding subroutine usage.
func NewCharStringInterpreter(globalSubrs, localSubrs [][]byte) *CharStringInterpreter {
	return &CharStringInterpreter{
		globalSubrs:     globalSubrs,
		localSubrs:      localSubrs,
		globalBias:      calcSubrBias(len(globalSubrs)),
		localBias:       calcSubrBias(len(localSubrs)),
		UsedGlobalSubrs: make(map[int]bool),
		UsedLocalSubrs:  make(map[int]bool),
		stack:           make([]int, 0, 48),
		callStack:       make([]csCallFrame, 0, 10),
	}
}

// FindUsedSubroutines executes a CharString to find all subroutine calls.
// This is a simplified interpreter that only tracks subr calls, not actual drawing.
func (i *CharStringInterpreter) FindUsedSubroutines(charstring []byte) error {
	i.stack = i.stack[:0]
	i.callStack = i.callStack[:0]
	i.hintCount = 0

	return i.execute(charstring)
}

func (i *CharStringInterpreter) execute(data []byte) error {
	pos := 0

	for pos < len(data) {
		b := data[pos]

		// Number operand
		if b >= 32 || b == 28 {
			val, consumed := decodeCSOperand(data[pos:])
			i.stack = append(i.stack, val)
			pos += consumed
			continue
		}

		// Operator
		op := int(b)
		pos++

		// Two-byte operator
		if b == 12 && pos < len(data) {
			op = 12<<8 | int(data[pos])
			pos++
		}

		switch op {
		case csCallsubr:
			// Call local subroutine
			if len(i.stack) > 0 {
				subrNum := i.stack[len(i.stack)-1] + i.localBias
				i.stack = i.stack[:len(i.stack)-1]

				if subrNum >= 0 && subrNum < len(i.localSubrs) {
					if !i.UsedLocalSubrs[subrNum] {
						i.UsedLocalSubrs[subrNum] = true
						// Recursively process subroutine
						i.callStack = append(i.callStack, csCallFrame{data: data, pos: pos})
						if err := i.execute(i.localSubrs[subrNum]); err != nil {
							return err
						}
						if len(i.callStack) > 0 {
							frame := i.callStack[len(i.callStack)-1]
							i.callStack = i.callStack[:len(i.callStack)-1]
							data = frame.data
							pos = frame.pos
						}
					}
				}
			}

		case csCallgsubr:
			// Call global subroutine
			if len(i.stack) > 0 {
				subrNum := i.stack[len(i.stack)-1] + i.globalBias
				i.stack = i.stack[:len(i.stack)-1]

				if subrNum >= 0 && subrNum < len(i.globalSubrs) {
					if !i.UsedGlobalSubrs[subrNum] {
						i.UsedGlobalSubrs[subrNum] = true
						// Recursively process subroutine
						i.callStack = append(i.callStack, csCallFrame{data: data, pos: pos})
						if err := i.execute(i.globalSubrs[subrNum]); err != nil {
							return err
						}
						if len(i.callStack) > 0 {
							frame := i.callStack[len(i.callStack)-1]
							i.callStack = i.callStack[:len(i.callStack)-1]
							data = frame.data
							pos = frame.pos
						}
					}
				}
			}

		case csReturn:
			// Return from subroutine
			return nil

		case csEndchar:
			// End of CharString
			return nil

		case csHstem, csVstem, csHstemhm, csVstemhm:
			// Hint operators - count stems and clear stack
			i.hintCount += len(i.stack) / 2
			i.stack = i.stack[:0]

		case csHintmask, csCntrmask:
			// Hint mask - implicit vstem if stack has data
			if len(i.stack) > 0 {
				i.hintCount += len(i.stack) / 2
				i.stack = i.stack[:0]
			}
			// Skip mask bytes
			maskBytes := (i.hintCount + 7) / 8
			pos += maskBytes

		case csRmoveto, csHmoveto, csVmoveto:
			// Movement operators clear stack
			i.stack = i.stack[:0]

		case csRlineto, csHlineto, csVlineto, csRrcurveto,
			csRcurveline, csRlinecurve, csVvcurveto, csHhcurveto,
			csVhcurveto, csHvcurveto:
			// Drawing operators clear stack
			i.stack = i.stack[:0]

		default:
			// Other operators - clear stack for safety
			if op >= 12<<8 {
				// Two-byte flex operators
				i.stack = i.stack[:0]
			}
		}
	}

	return nil
}

// decodeCSOperand decodes a CharString operand.
func decodeCSOperand(data []byte) (int, int) {
	if len(data) == 0 {
		return 0, 0
	}

	b0 := data[0]

	// 1-byte integer (32-246)
	if b0 >= 32 && b0 <= 246 {
		return int(b0) - 139, 1
	}

	// 2-byte positive integer (247-250)
	if b0 >= 247 && b0 <= 250 {
		if len(data) < 2 {
			return 0, 1
		}
		return (int(b0)-247)*256 + int(data[1]) + 108, 2
	}

	// 2-byte negative integer (251-254)
	if b0 >= 251 && b0 <= 254 {
		if len(data) < 2 {
			return 0, 1
		}
		return -(int(b0)-251)*256 - int(data[1]) - 108, 2
	}

	// 3-byte integer (operator 28)
	if b0 == 28 {
		if len(data) < 3 {
			return 0, 1
		}
		v := int(int16(binary.BigEndian.Uint16(data[1:3])))
		return v, 3
	}

	// Fixed-point number (255) - used in CharStrings only
	if b0 == 255 {
		if len(data) < 5 {
			return 0, 1
		}
		// 16.16 fixed point, return integer part
		v := int(int32(binary.BigEndian.Uint32(data[1:5])))
		return v >> 16, 5
	}

	return 0, 1
}

// RemapCharString rewrites a CharString with remapped subroutine numbers.
func RemapCharString(cs []byte, globalMap, localMap map[int]int, oldGlobalBias, oldLocalBias, newGlobalBias, newLocalBias int) []byte {
	result := make([]byte, 0, len(cs))
	stack := make([]struct {
		val   int
		start int
		end   int
	}, 0, 48)
	pos := 0
	hintCount := 0

	for pos < len(cs) {
		b := cs[pos]

		// Number operand - track position for potential rewriting
		if b >= 32 || b == 28 || b == 255 {
			start := pos
			val, consumed := decodeCSOperand(cs[pos:])
			stack = append(stack, struct {
				val   int
				start int
				end   int
			}{val, start, pos + consumed})
			pos += consumed
			continue
		}

		// Operator
		op := int(b)
		opStart := pos
		pos++

		// Two-byte operator
		if b == 12 && pos < len(cs) {
			op = 12<<8 | int(cs[pos])
			pos++
		}

		switch op {
		case csCallsubr:
			// Remap local subroutine call
			if len(stack) > 0 {
				entry := stack[len(stack)-1]
				stack = stack[:len(stack)-1]

				oldSubrNum := entry.val + oldLocalBias
				if newNum, ok := localMap[oldSubrNum]; ok {
					// Write everything before this operand
					result = append(result, cs[:entry.start]...)
					// Write remapped operand
					newBiasedNum := newNum - newLocalBias
					result = append(result, encodeCSInt(newBiasedNum)...)
					// Write operator
					result = append(result, cs[opStart:pos]...)
					// Continue from here
					cs = cs[pos:]
					pos = 0
					continue
				}
			}

		case csCallgsubr:
			// Remap global subroutine call
			if len(stack) > 0 {
				entry := stack[len(stack)-1]
				stack = stack[:len(stack)-1]

				oldSubrNum := entry.val + oldGlobalBias
				if newNum, ok := globalMap[oldSubrNum]; ok {
					// Write everything before this operand
					result = append(result, cs[:entry.start]...)
					// Write remapped operand
					newBiasedNum := newNum - newGlobalBias
					result = append(result, encodeCSInt(newBiasedNum)...)
					// Write operator
					result = append(result, cs[opStart:pos]...)
					// Continue from here
					cs = cs[pos:]
					pos = 0
					continue
				}
			}

		case csHstem, csVstem, csHstemhm, csVstemhm:
			hintCount += len(stack) / 2
			stack = stack[:0]

		case csHintmask, csCntrmask:
			if len(stack) > 0 {
				hintCount += len(stack) / 2
				stack = stack[:0]
			}
			// Skip mask bytes
			maskBytes := (hintCount + 7) / 8
			pos += maskBytes

		default:
			stack = stack[:0]
		}
	}

	// If no remapping happened, return original
	if len(result) == 0 {
		return cs
	}

	// Append remaining data
	result = append(result, cs...)
	return result
}
