package ompsessions

import (
	"encoding/json"
	"fmt"
)

func ompActiveBranch(entries []ompCarryEntry, byID map[string]int) ([]ompCarryEntry, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	leafOf := func(entry ompCarryEntry) (string, bool) {
		if len(entry.ActiveLeafID) == 0 {
			return entry.ID, false
		}
		if string(entry.ActiveLeafID) == "null" {
			return "", true
		}
		var leaf string
		if json.Unmarshal(entry.ActiveLeafID, &leaf) != nil {
			return "\x00invalid-active-leaf", false
		}
		return leaf, false
	}
	last := entries[len(entries)-1]
	leaf, cleared := leafOf(last)
	if !cleared {
		if _, ok := byID[leaf]; !ok {
			leaf = ""
			for index := len(entries) - 1; index >= 0; index-- {
				selected, null := leafOf(entries[index])
				if !null && selected == entries[index].ID {
					leaf = entries[index].ID
					break
				}
			}
		}
	}
	var branch []ompCarryEntry
	seen := make(map[string]bool)
	for id := leaf; id != ""; {
		if seen[id] {
			return nil, fmt.Errorf("omp session parent cycle at %q", id)
		}
		index, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("omp session missing parent %q", id)
		}
		seen[id] = true
		entry := entries[index]
		branch = append(branch, entry)
		if entry.ParentID == nil {
			break
		}
		if *entry.ParentID == "" {
			return nil, fmt.Errorf("omp session missing parent %q", *entry.ParentID)
		}
		id = *entry.ParentID
	}
	for left, right := 0, len(branch)-1; left < right; left, right = left+1, right-1 {
		branch[left], branch[right] = branch[right], branch[left]
	}
	return branch, nil
}
