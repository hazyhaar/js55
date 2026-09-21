// SPDX-License-Identifier: Apache-2.0 OR MIT

package str

import (
	"errors"
)

// Irregexp Opcode constants mirroring V8 regexp-bytecodes.h
const (
	OpBreak = iota
	OpPushCP
	OpPopCP
	OpPushBT
	OpPopBT
	OpCheckAtStart
	OpCheckNotAtStart
	OpCheckCharacter
	OpCheckCharacterAfterAnd
	OpJump
	OpCheckNotCharacter
	OpCheckCharacterInRange
	OpCheckCharacterNotInRange
	OpCheckBitInTable
	OpSucceed
	OpFail
	OpAdvanceCP
)

// RegExpEngine represents the Irregexp bytecode interpreter.
type RegExpEngine struct {
	bytecodes []byte
	captures  []int
	stack     []int
}

// NewRegExpEngine creates a new Irregexp engine instance.
func NewRegExpEngine(bc []byte, numCaptures int) *RegExpEngine {
	return &RegExpEngine{
		bytecodes: bc,
		captures:  make([]int, numCaptures*2),
		stack:     make([]int, 0, 64),
	}
}

// Exec executes the Irregexp bytecode against a String input.
func (re *RegExpEngine) Exec(s *String, startPos int) (bool, []int, error) {
	if s == nil {
		return false, nil, errors.New("js55/regexp: input string is nil")
	}

	length := s.Len()
	if startPos < 0 || startPos > length {
		return false, nil, nil
	}

	cp := startPos
	pc := 0
	bc := re.bytecodes
	re.stack = re.stack[:0]

	for pc < len(bc) {
		op := bc[pc]
		pc++

		switch op {
		case OpSucceed:
			return true, re.captures, nil

		case OpFail:
			if len(re.stack) == 0 {
				return false, nil, nil
			}
			// Backtrack
			pc = re.stack[len(re.stack)-1]
			cp = re.stack[len(re.stack)-2]
			re.stack = re.stack[:len(re.stack)-2]

		case OpAdvanceCP:
			cp++
			if cp > length {
				// Implicit fail
				if len(re.stack) == 0 {
					return false, nil, nil
				}
				pc = re.stack[len(re.stack)-1]
				cp = re.stack[len(re.stack)-2]
				re.stack = re.stack[:len(re.stack)-2]
			}

		case OpCheckCharacter:
			if pc >= len(bc) || cp >= length {
				// Match failed
				if len(re.stack) == 0 {
					return false, nil, nil
				}
				pc = re.stack[len(re.stack)-1]
				cp = re.stack[len(re.stack)-2]
				re.stack = re.stack[:len(re.stack)-2]
				continue
			}
			expected := bc[pc]
			pc++
			if s.At(cp) != uint16(expected) {
				if len(re.stack) == 0 {
					return false, nil, nil
				}
				pc = re.stack[len(re.stack)-1]
				cp = re.stack[len(re.stack)-2]
				re.stack = re.stack[:len(re.stack)-2]
			} else {
				cp++
			}

		case OpPushBT:
			if pc+2 > len(bc) {
				return false, nil, errors.New("js55/regexp: malformed bytecode in OpPushBT")
			}
			offset := int(int16(uint16(bc[pc]) | uint16(bc[pc+1])<<8))
			pc += 2
			// Push backtrack target and current char pointer
			re.stack = append(re.stack, cp, pc+offset)

		case OpBreak:
			return false, nil, nil

		default:
			return false, nil, errors.New("js55/regexp: unsupported bytecode instruction")
		}
	}

	return false, nil, nil
}
