package program

import "github.com/google/uuid"

// BlockVisibleToClient reports whether a client sees this block.
// A block without rules is shared and visible to everyone.
func BlockVisibleToClient(block DayBlock, clientUserID uuid.UUID) bool {
	if len(block.ClientUserIDs) == 0 {
		return true
	}
	for _, id := range block.ClientUserIDs {
		if id == clientUserID {
			return true
		}
	}
	return false
}

// FilterDetailForClient returns a copy of d with blocks the client must not see
// removed. The receiver is not mutated.
func FilterDetailForClient(d Detail, clientUserID uuid.UUID) Detail {
	out := d
	out.Weeks = make([]Week, 0, len(d.Weeks))
	for _, week := range d.Weeks {
		w := week
		w.Days = make([]Day, 0, len(week.Days))
		for _, day := range week.Days {
			dd := day
			dd.Blocks = make([]DayBlock, 0, len(day.Blocks))
			for _, block := range day.Blocks {
				if BlockVisibleToClient(block, clientUserID) {
					dd.Blocks = append(dd.Blocks, block)
				}
			}
			w.Days = append(w.Days, dd)
		}
		out.Weeks = append(out.Weeks, w)
	}
	return out
}

// dayHasSharedBlock reports whether the day keeps at least one block without rules.
func dayHasSharedBlock(day Day) bool {
	for _, block := range day.Blocks {
		if len(block.ClientUserIDs) == 0 {
			return true
		}
	}
	return false
}

// daySharedAfterRestrict reports whether the day still holds a shared block once
// the block with blockKey becomes restricted. A day that does not hold the key,
// or holds no blocks at all, is unaffected.
func daySharedAfterRestrict(day Day, blockKey uuid.UUID) bool {
	holdsKey := false
	for _, block := range day.Blocks {
		if block.BlockKey == blockKey {
			holdsKey = true
			break
		}
	}
	if !holdsKey {
		return true
	}
	for _, block := range day.Blocks {
		if block.BlockKey == blockKey {
			continue
		}
		if len(block.ClientUserIDs) == 0 {
			return true
		}
	}
	return false
}
