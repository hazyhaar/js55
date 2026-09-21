// SPDX-License-Identifier: BUSL-1.1

package heap

// CardTable tracks modified memory pages for generational GC.
type CardTable struct {
	Cards []bool
}

// NewCardTable creates card table for heap size.
func NewCardTable(numCards int) *CardTable {
	return &CardTable{Cards: make([]bool, numCards)}
}

// MarkDirty marks a card index dirty upon object pointer mutation.
func (ct *CardTable) MarkDirty(cardIdx int) {
	if cardIdx >= 0 && cardIdx < len(ct.Cards) {
		ct.Cards[cardIdx] = true
	}
}

// WriteBarrier enforces generational write barrier for old-to-young pointers.
type WriteBarrier struct {
	Table *CardTable
}

// OnFieldWrite intercepts pointer writes and dirties corresponding card.
func (wb *WriteBarrier) OnFieldWrite(cardIdx int) {
	if wb.Table != nil {
		wb.Table.MarkDirty(cardIdx)
	}
}
