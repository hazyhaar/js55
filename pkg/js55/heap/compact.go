// SPDX-License-Identifier: Apache-2.0 OR MIT

package heap

// MemoryBlock represents an allocated region in heap space.
type MemoryBlock struct {
	Address uintptr
	Size    int
	Live    bool
}

// TwoFingerCompactor compacts live memory blocks to eliminate fragmentation.
type TwoFingerCompactor struct {
	Blocks []MemoryBlock
}

// Compact executes two-finger sliding compaction algorithm.
func (c *TwoFingerCompactor) Compact() int {
	if len(c.Blocks) == 0 {
		return 0
	}
	freePtr := 0
	livePtr := len(c.Blocks) - 1
	moved := 0

	for freePtr < livePtr {
		for freePtr < len(c.Blocks) && c.Blocks[freePtr].Live {
			freePtr++
		}
		for livePtr >= 0 && !c.Blocks[livePtr].Live {
			livePtr--
		}
		if freePtr < livePtr {
			// Relocate live block to free slot
			c.Blocks[freePtr] = c.Blocks[livePtr]
			c.Blocks[livePtr].Live = false
			freePtr++
			livePtr--
			moved++
		}
	}
	return moved
}
