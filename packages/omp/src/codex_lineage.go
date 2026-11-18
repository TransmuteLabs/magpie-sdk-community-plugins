package ompsessions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path"
	"strconv"
	"strings"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type codexLineagePosition struct {
	RolloutID string  `json:"thread_id"`
	Ordinal   *uint64 `json:"end_ordinal_exclusive"`
	Offset    *uint64 `json:"end_byte_offset"`
}

type codexLineagePart struct {
	header   map[string]json.RawMessage
	metadata map[string]json.RawMessage
	base     *codexLineagePosition
	records  []byte
}

func materializeCodexLineage(ctx context.Context, request sdk.SessionRequest) ([]byte, error) {
	if err := sdk.ValidateSessionRelatedInputs(Manifest(), request); err != nil {
		return nil, err
	}
	if request.Related == nil {
		return request.Files[0].Body, nil
	}
	inputs := append([]sdk.SessionRelatedInput(nil), request.Related.Ancestors...)
	inputs = append(inputs, sdk.SessionRelatedInput{Body: request.Files[0].Body, Source: request.Related.Selected})
	parts := make([]codexLineagePart, len(inputs))
	for index, input := range inputs {
		part, err := readCodexLineagePart(ctx, input)
		if err != nil {
			return nil, err
		}
		if index == 0 {
			if part.base != nil {
				return nil, errors.New("Codex related history is missing its oldest captured ancestor")
			}
		} else {
			parent := inputs[index-1].Source
			if part.base == nil || part.base.RolloutID != parent.RolloutID ||
				strconv.FormatUint(*part.base.Offset, 10) != parent.ByteEndExclusive ||
				strconv.FormatUint(*part.base.Ordinal, 10) != parent.OrdinalEndExclusive {
				return nil, errors.New("Codex related history does not match the ordered captured ranges")
			}
		}
		parts[index] = part
	}
	selected := parts[len(parts)-1]
	delete(selected.metadata, "history_base")
	delete(selected.metadata, "subagent_history_start_ordinal")
	payload, err := json.Marshal(selected.metadata)
	if err != nil {
		return nil, err
	}
	selected.header["payload"], selected.header["ordinal"] = payload, json.RawMessage("0")
	body, err := json.Marshal(selected.header)
	if err != nil {
		return nil, err
	}
	body = append(body, '\n')
	ordinal := uint64(1)
	for _, part := range parts {
		var parseErr error
		err := eachLine(ctx, part.records, func(line []byte) {
			if parseErr != nil {
				return
			}
			var record map[string]json.RawMessage
			if json.Unmarshal(line, &record) != nil {
				parseErr = errors.New("invalid captured Codex history record")
				return
			}
			record["ordinal"] = json.RawMessage(strconv.FormatUint(ordinal, 10))
			line, parseErr = json.Marshal(record)
			if parseErr != nil {
				return
			}
			if len(body)+len(line)+1 > MaxBytes {
				parseErr = errors.New("materialized Codex history exceeds the OMP input budget")
				return
			}
			body = append(body, line...)
			body = append(body, '\n')
			ordinal++
		})
		if err != nil || parseErr != nil {
			return nil, errors.Join(err, parseErr)
		}
	}
	return body, ctx.Err()
}

func readCodexLineagePart(ctx context.Context, input sdk.SessionRelatedInput) (codexLineagePart, error) {
	part := codexLineagePart{}
	if len(input.Body) == 0 || input.Body[len(input.Body)-1] != '\n' {
		return part, errors.New("Codex captured prefix must end at a complete JSONL record")
	}
	first, rest, _ := bytes.Cut(bytes.TrimLeft(input.Body, " \r\n\t"), []byte{'\n'})
	if json.Unmarshal(first, &part.header) != nil || string(part.header["type"]) != `"session_meta"` ||
		json.Unmarshal(part.header["payload"], &part.metadata) != nil || part.metadata == nil {
		return part, errors.New("Codex related input requires its original metadata header")
	}
	var owner, mode string
	if json.Unmarshal(part.metadata["id"], &owner) != nil ||
		json.Unmarshal(part.metadata["history_mode"], &mode) != nil || mode != "paginated" {
		return part, errors.New("invalid captured Codex metadata identity or history mode")
	}
	filename := strings.TrimSuffix(strings.TrimSuffix(path.Base(input.Source.Path), ".zst"), ".jsonl")
	if !strings.HasPrefix(filename, "rollout-") || !strings.HasSuffix(filename, input.Source.RolloutID) {
		return part, errors.New("captured Codex rollout identity differs from its native filename")
	}
	nativeOwner := input.Source.RolloutID
	if len(filename) >= 73 && filename[len(filename)-37] == '_' {
		nativeOwner = filename[len(filename)-73 : len(filename)-37]
	}
	if owner != nativeOwner {
		return part, errors.New("captured Codex metadata belongs to another native thread")
	}
	if ompCarryPresent(part.metadata["history_base"]) {
		if json.Unmarshal(part.metadata["history_base"], &part.base) != nil || part.base == nil ||
			part.base.Ordinal == nil || part.base.Offset == nil || *part.base.Ordinal == 0 || *part.base.Offset == 0 {
			return part, errors.New("invalid captured Codex history_base")
		}
	}
	start, _ := strconv.ParseUint(input.Source.OrdinalStart, 10, 64)
	expected := uint64(0)
	if part.base != nil {
		expected = *part.base.Ordinal
	}
	if start != expected {
		return part, errors.New("captured Codex ordinal start differs from its original history_base")
	}
	firstRecord := true
	var parseErr error
	err := eachLine(ctx, input.Body, func(line []byte) {
		if parseErr != nil {
			return
		}
		var record struct {
			Type    string  `json:"type"`
			Ordinal *uint64 `json:"ordinal"`
		}
		if json.Unmarshal(line, &record) != nil || record.Ordinal == nil || *record.Ordinal != expected ||
			record.Type == "" || firstRecord && record.Type != "session_meta" ||
			!firstRecord && record.Type == "session_meta" || expected == ^uint64(0) {
			parseErr = errors.New("captured Codex records disagree with their original ordinal range")
			return
		}
		firstRecord = false
		expected++
	})
	if err != nil || parseErr != nil {
		return part, errors.Join(err, parseErr)
	}
	if strconv.FormatUint(expected, 10) != input.Source.OrdinalEndExclusive {
		return part, errors.New("captured Codex prefix differs from its exclusive ordinal")
	}
	part.records = rest
	return part, nil
}
