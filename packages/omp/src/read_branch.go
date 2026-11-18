package ompsessions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

func ompJournalBranch(ctx context.Context, body []byte, codex bool) ([]ompCarryEntry, error) {
	var entries []ompCarryEntry
	byID := make(map[string]int)
	var parseErr error
	if err := eachLine(ctx, body, func(line []byte) {
		var entry ompCarryEntry
		if codex {
			if json.Unmarshal(line, &entry) != nil {
				parseErr = errors.New("invalid OMP native history entry")
				return
			}
		} else {
			var envelope struct {
				Type         string          `json:"type"`
				ID           string          `json:"id"`
				ParentID     *string         `json:"parentId"`
				ActiveLeafID json.RawMessage `json:"activeLeafId"`
				Timestamp    string          `json:"timestamp"`
			}
			if json.Unmarshal(line, &envelope) != nil {
				return
			}
			entry = ompCarryEntry{Type: envelope.Type, ID: envelope.ID, ParentID: envelope.ParentID,
				ActiveLeafID: envelope.ActiveLeafID, Timestamp: envelope.Timestamp}
			if entry.Type == "message" || entry.Type == "compaction" || entry.Type == "branch_summary" {
				var payload ompCarryEntry
				if json.Unmarshal(line, &payload) == nil {
					entry = payload
				} else if entry.Type == "compaction" {
					entry.Unreadable = true
				}
			}
		}
		if entry.ID == "" || entry.Type == "session" || entry.Type == "title" {
			return
		}
		if entry.Type == "custom_message" {
			entry.CustomMessage = line
		}
		byID[entry.ID] = len(entries)
		entries = append(entries, entry)
	}); err != nil {
		return nil, err
	}
	if parseErr != nil {
		return nil, parseErr
	}
	return ompActiveBranch(entries, byID)
}

func ompReplayBounds(branch []ompCarryEntry, codex bool) (retained, compact int) {
	root := 0
	for index := len(branch) - 1; index >= 0; index-- {
		if branch[index].Type == "reset_boundary" {
			root = index + 1
			break
		}
	}
	for index := len(branch) - 1; index >= root; index-- {
		entry := branch[index]
		if entry.Type != "compaction" || !ompCarryPortable(entry) {
			continue
		}
		first := 0
		if codex {
			first = root
		}
		for retained := first; retained < index; retained++ {
			if entry.FirstKeptEntryID != "" && entry.FirstKeptEntryID == branch[retained].ID {
				return max(root, retained), index
			}
			if entry.FirstKeptEntryID == "" && ompCarryPresent(entry.PreserveData["anthropicCompaction"]) &&
				entry.ProviderReplayThroughEntryID == branch[retained].ID {
				return max(root, retained+1), index
			}
		}
		return index, index
	}
	return root, -1
}

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
