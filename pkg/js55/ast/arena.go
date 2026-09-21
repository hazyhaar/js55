// SPDX-License-Identifier: Apache-2.0 OR MIT

package ast

// ASTArena stores AST nodes in a Struct-of-Arrays (SoA) layout.
// It completely eliminates Go pointer tracing (CSG-025) and enables O(1) reset.
type ASTArena struct {
	DirtyMasks    []uint64
	NodeTypes     []uint8
	LeftChildren  []uint32
	RightChildren []uint32
	LexemeOffsets []uint32
	LexemeLengths []uint32
	Length        uint32
}

// NewASTArena pre-allocates an AST arena with initial capacity.
func NewASTArena(capacity int) *ASTArena {
	return &ASTArena{
		DirtyMasks:    make([]uint64, capacity),
		NodeTypes:     make([]uint8, capacity),
		LeftChildren:  make([]uint32, capacity),
		RightChildren: make([]uint32, capacity),
		LexemeOffsets: make([]uint32, capacity),
		LexemeLengths: make([]uint32, capacity),
		Length:        0,
	}
}

// AllocNode allocates a new AST node index in O(1).
func (a *ASTArena) AllocNode(nodeType uint8, left, right, offset, length uint32) uint32 {
	idx := a.Length
	if int(idx) >= len(a.NodeTypes) {
		a.grow()
	}
	a.NodeTypes[idx] = nodeType
	a.LeftChildren[idx] = left
	a.RightChildren[idx] = right
	a.LexemeOffsets[idx] = offset
	a.LexemeLengths[idx] = length
	a.DirtyMasks[idx] = 1 << (nodeType & 0x3F)
	a.Length++
	return idx
}

// Reset resets the arena in O(1) without GC overhead.
func (a *ASTArena) Reset() {
	a.Length = 0
}

func (a *ASTArena) grow() {
	newCap := len(a.NodeTypes) * 2
	if newCap == 0 {
		newCap = 64
	}
	a.DirtyMasks = append(a.DirtyMasks, make([]uint64, newCap-len(a.DirtyMasks))...)
	a.NodeTypes = append(a.NodeTypes, make([]uint8, newCap-len(a.NodeTypes))...)
	a.LeftChildren = append(a.LeftChildren, make([]uint32, newCap-len(a.LeftChildren))...)
	a.RightChildren = append(a.RightChildren, make([]uint32, newCap-len(a.RightChildren))...)
	a.LexemeOffsets = append(a.LexemeOffsets, make([]uint32, newCap-len(a.LexemeOffsets))...)
	a.LexemeLengths = append(a.LexemeLengths, make([]uint32, newCap-len(a.LexemeLengths))...)
}
