package ompsessions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func decodeSourceDocuments(ctx context.Context, request sdk.SessionSourceReadRequest) (sdk.SessionSourceSummary, []ompCarryEntry, error) {
	summary, branch, err := decodeSourceJournal(ctx, request.Documents[0].Body, request.Locator.SessionID)
	if err != nil {
		return summary, nil, err
	}
	seen := map[string]bool{request.Locator.SessionID: true}
	admitted := map[string]bool{}
	for _, name := range request.Descendants {
		admitted[name] = true
	}
	rule := Manifest().SessionSources[0].Descendants
	unknownUsage := summary.Usage == nil
	for _, document := range request.Documents[1:] {
		if !admitted[document.Path] || !sdk.SessionSourceDescendantPath(rule, request.Locator.Path, document.Path) {
			return sdk.SessionSourceSummary{}, nil, errors.New("OMP Source artifact is outside its selected journal")
		}
		first, rest, _ := bytes.Cut(document.Body, []byte{'\n'})
		var header sourceRecord
		if json.Unmarshal(first, &header) == nil && header.Type == "title" {
			first, _, _ = bytes.Cut(rest, []byte{'\n'})
		}
		if json.Unmarshal(first, &header) != nil || header.Type != "session" || !validUUID(header.ID) || seen[header.ID] {
			return sdk.SessionSourceSummary{}, nil, errors.New("invalid or duplicate OMP artifact session identity")
		}
		seen[header.ID] = true
		child, _, err := decodeSourceJSONL(ctx, document.Body, header.ID, false)
		if err != nil {
			return sdk.SessionSourceSummary{}, nil, err
		}
		if child.Usage == nil {
			unknownUsage = true
		}
		summary.Subs = append(summary.Subs, sdk.SessionSourceIdentity{Path: document.Path, SessionID: header.ID})
		summary.Usage = append(summary.Usage, child.Usage...)
		last, _ := time.Parse(time.RFC3339Nano, summary.Last)
		childLast, _ := time.Parse(time.RFC3339Nano, child.Last)
		if childLast.After(last) {
			summary.Last = child.Last
		}
	}
	var total uint64
	for _, point := range summary.Usage {
		for _, count := range []uint64{point.Input, point.Output, point.CacheRead, point.CacheWrite} {
			if count > math.MaxInt64-total {
				return sdk.SessionSourceSummary{}, nil, errors.New("OMP Source aggregate usage counters overflow")
			}
			total += count
		}
	}
	if unknownUsage {
		summary.Usage = nil
	}
	return summary, branch, nil
}
