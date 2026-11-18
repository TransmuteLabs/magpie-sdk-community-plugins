package ompsessions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type sourceUsage struct {
	Input      uint64 `json:"input"`
	Output     uint64 `json:"output"`
	CacheRead  uint64 `json:"cacheRead"`
	CacheWrite uint64 `json:"cacheWrite"`
}

type sourceRecord struct {
	Type          string          `json:"type"`
	Version       int             `json:"version"`
	ID            string          `json:"id"`
	Cwd           string          `json:"cwd"`
	Title         string          `json:"title"`
	Name          string          `json:"name"`
	Timestamp     string          `json:"timestamp"`
	Model         string          `json:"model"`
	ModelID       string          `json:"modelId"`
	ParentSession string          `json:"parentSession"`
	Usage         *sourceUsage    `json:"usage"`
	Message       json.RawMessage `json:"message"`
}

type sourceMessage struct {
	ompCarryMessage
	Model    string       `json:"model"`
	ToolName string       `json:"toolName"`
	Usage    *sourceUsage `json:"usage"`
}

func decodeSourceJournal(ctx context.Context, body []byte, selectedID string) (sdk.SessionSourceSummary, []ompCarryEntry, error) {
	return decodeSourceJSONL(ctx, body, selectedID, true)
}

func decodeSourceJSONL(ctx context.Context, body []byte, selectedID string, main bool) (sdk.SessionSourceSummary, []ompCarryEntry, error) {
	var summary sdk.SessionSourceSummary
	if len(body) == 0 || len(body) > MaxBytes || !utf8.Valid(body) || body[len(body)-1] != '\n' {
		return summary, nil, errors.New("OMP Source requires a bounded complete UTF-8 JSONL prefix")
	}
	first, _, _ := bytes.Cut(body, []byte{'\n'})
	var slot struct {
		Type      string          `json:"type"`
		V         int             `json:"v"`
		Title     *string         `json:"title"`
		UpdatedAt *string         `json:"updatedAt"`
		Pad       *string         `json:"pad"`
		Source    json.RawMessage `json:"source"`
	}
	hasSlot := json.Unmarshal(first, &slot) == nil && slot.Type == "title"
	if (main || hasSlot) && (len(first) != 255 || slot.Type != "title" || slot.V != 1 ||
		slot.Title == nil || slot.UpdatedAt == nil || slot.Pad == nil || strings.Trim(*slot.Pad, " ") != "") {
		return summary, nil, errors.New("unknown journal format: expected the recovered OMP fixed-width v1 title slot")
	}
	if hasSlot && len(slot.Source) != 0 {
		var source string
		if json.Unmarshal(slot.Source, &source) != nil || source != "auto" && source != "user" {
			return summary, nil, errors.New("unsupported OMP title source")
		}
	}
	var entries []ompCarryEntry
	byID := map[string]int{}
	var started, last time.Time
	var header, forked, usageSeen, usageUnknown bool
	var model, firstPrompt string
	var points []sdk.SessionUsagePoint
	var total uint64
	var parseErr error
	lineNumber := 0
	err := eachLine(ctx, body, func(line []byte) {
		lineNumber++
		if parseErr != nil || hasSlot && lineNumber == 1 {
			return
		}
		if lineNumber > 10002 {
			parseErr = errors.New("OMP Source record count exceeds its budget")
			return
		}
		var record sourceRecord
		if json.Unmarshal(line, &record) != nil || record.Type == "" {
			parseErr = errors.New("invalid OMP Source record")
			return
		}
		at, _ := time.Parse(time.RFC3339Nano, record.Timestamp)
		if record.Type == "session" {
			headerLine := 1
			if hasSlot {
				headerLine = 2
			}
			if header || lineNumber != headerLine || record.Version != 3 || !validUUID(record.ID) ||
				record.ID != selectedID || at.IsZero() {
				parseErr = errors.New("OMP Source requires one v3 header confirming the host-selected UUID")
				return
			}
			header, started, last = true, at, at
			summary.Title, summary.Cwd = record.Title, record.Cwd
			if summary.Title == "" && slot.Title != nil {
				summary.Title = *slot.Title
			}
			forked = record.ParentSession != ""
			if validUUID(record.ParentSession) {
				summary.ParentID = record.ParentSession
			}
			return
		}
		if !header || record.ID == "" {
			parseErr = errors.New("OMP Source entry has no header or ID")
			return
		}
		if _, exists := byID[record.ID]; exists {
			parseErr = errors.New("duplicate OMP Source entry ID")
			return
		}
		var entry ompCarryEntry
		if json.Unmarshal(line, &entry) != nil {
			parseErr = errors.New("invalid OMP Source entry tree")
			return
		}
		if len(entry.ActiveLeafID) != 0 && string(entry.ActiveLeafID) != "null" {
			var leaf string
			if json.Unmarshal(entry.ActiveLeafID, &leaf) != nil || leaf == "" {
				parseErr = errors.New("invalid OMP Source active leaf")
				return
			}
		}
		byID[entry.ID] = len(entries)
		entries = append(entries, entry)
		if at.After(last) {
			last = at
		}
		if record.Type == "title_change" && record.Title != "" {
			summary.Title = record.Title
		}
		if record.Type == "session_info" {
			summary.Title = record.Name
		}
		if record.Type == "model_change" {
			model = record.ModelID
			if model == "" {
				_, model, _ = strings.Cut(record.Model, "/")
			}
		}
		var used *sourceUsage
		var usedModel string
		var work bool
		switch record.Type {
		case "usage", "model_usage":
			used, usedModel, work = record.Usage, record.Model, true
		case "compaction", "branch_summary":
			used, usedModel, work = record.Usage, model, record.Usage != nil
		case "message":
			var message sourceMessage
			if json.Unmarshal(record.Message, &message) != nil {
				parseErr = errors.New("invalid OMP Source message")
				return
			}
			if at.IsZero() && message.Timestamp != nil {
				at = time.UnixMilli(*message.Timestamp)
				if at.After(last) {
					last = at
				}
			}
			switch message.Role {
			case "user", "assistant", "toolResult", "fileMention", "bashExecution", "pythonExecution", "system", "developer":
			default:
				parseErr = errors.New("unsupported OMP Source message role")
				return
			}
			parts, err := sourceEntryParts(entry)
			if err != nil {
				parseErr = err
				return
			}
			if message.Role == "user" && firstPrompt == "" {
				for _, part := range parts {
					if text := strings.TrimSpace(part.Text); text != "" {
						if len(text) > 4096 {
							text = text[:4096]
							for !utf8.ValidString(text) {
								text = text[:len(text)-1]
							}
						}
						firstPrompt = text
						break
					}
				}
			}
			if message.Role == "assistant" {
				if message.Model != "" {
					model = message.Model
				}
				used, usedModel, work = message.Usage, message.Model, true
			} else if message.Usage != nil {
				used, usedModel, work = message.Usage, model, true
			}
		}
		if !work || forked && !at.IsZero() && at.Before(started) {
			return
		}
		if used == nil || usedModel == "" || at.IsZero() {
			usageUnknown = true
			return
		}
		for _, count := range []uint64{used.Input, used.Output, used.CacheRead, used.CacheWrite} {
			if count > math.MaxInt64-total {
				parseErr = errors.New("OMP Source usage counters overflow")
				return
			}
			total += count
		}
		usageSeen = true
		points = append(points, sdk.SessionUsagePoint{At: at.Format(time.RFC3339Nano), Model: usedModel,
			Input: used.Input, Output: used.Output, CacheRead: used.CacheRead, CacheWrite: used.CacheWrite})
	})
	if err != nil {
		return sdk.SessionSourceSummary{}, nil, err
	}
	if parseErr != nil {
		return sdk.SessionSourceSummary{}, nil, parseErr
	}
	if !header {
		return sdk.SessionSourceSummary{}, nil, errors.New("missing OMP Source header")
	}
	summary.Start, summary.Last = started.Format(time.RFC3339Nano), last.Format(time.RFC3339Nano)
	if summary.Title == "" {
		summary.Title = firstPrompt
	}
	if usageSeen && !usageUnknown {
		summary.Usage = points
	}
	branch, err := ompActiveBranch(entries, byID)
	return summary, branch, err
}
