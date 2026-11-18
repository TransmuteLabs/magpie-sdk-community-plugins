package ompsessions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	sdk "github.com/yetone/magpie/pkg/extension"
)

const SourceInputsAction = "session.inputs.v1"

type SourceInput struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type SourceInputs struct {
	Version int           `json:"version"`
	Files   []SourceInput `json:"files"`
}

func PlanSourceInputs(ctx context.Context, request sdk.SessionRequest) (SourceInputs, error) {
	if err := ctx.Err(); err != nil {
		return SourceInputs{}, err
	}
	validRoute := request.ID == "omp-to-document" && request.To == DocumentFormat ||
		request.ID == "omp-to-claude" && request.To == "claude" ||
		request.ID == "omp-to-codex" && request.To == "codex"
	reverse := request.ID == "claude-to-omp" && request.From == "claude" ||
		request.ID == "codex-to-omp" && request.From == "codex"
	if reverse && request.To == "omp" && len(request.Files) == 1 && request.Files[0].Format == request.From &&
		safePath(request.Files[0].Path) && len(request.Files[0].Body) > 0 && len(request.Files[0].Body) <= MaxBytes {
		return SourceInputs{Version: 1, Files: []SourceInput{}}, nil
	}
	if !validRoute || !isOMP(request.From) ||
		len(request.Files) != 1 || request.Files[0].Format != request.From ||
		!safePath(request.Files[0].Path) || len(request.Files[0].Body) == 0 ||
		len(request.Files[0].Body) > MaxBytes {
		return SourceInputs{}, errors.New("source input planning requires one bounded OMP journal")
	}
	hashes := map[string]bool{}
	visitHash := func(hash string) error {
		hashes[hash] = true
		if len(hashes) >= MaxFiles {
			return errors.New("OMP attachment count exceeds the input budget")
		}
		return nil
	}
	archiveCustom := request.ID == "omp-to-document"
	hasCustom := false
	var parseErr error
	err := eachLine(ctx, request.Files[0].Body, func(line []byte) {
		if parseErr != nil {
			return
		}
		var record struct {
			Type    string          `json:"type"`
			Message json.RawMessage `json:"message"`
		}
		if json.Unmarshal(line, &record) != nil || record.Type == "" {
			parseErr = errors.New("invalid OMP record during attachment planning")
			return
		}
		attachmentBody := record.Message
		if record.Type == "custom_message" {
			hasCustom = true
			if !archiveCustom {
				return
			}
			attachmentBody = line
		}
		parseErr = visitAttachmentReferences(record.Type, attachmentBody, visitHash)
	})
	if err != nil {
		return SourceInputs{}, err
	}
	if parseErr != nil {
		return SourceInputs{}, parseErr
	}
	if !archiveCustom && hasCustom {
		codex := request.To == "codex"
		branch, err := ompJournalBranch(ctx, request.Files[0].Body, codex)
		if err != nil {
			return SourceInputs{}, err
		}
		retained, _ := ompReplayBounds(branch, codex)
		for _, entry := range branch[retained:] {
			if entry.Type != "custom_message" {
				continue
			}
			_, blocks, err := ompCustomMessageContent(entry.CustomMessage)
			if err != nil {
				return SourceInputs{}, fmt.Errorf("omp entry %q: %w", entry.ID, err)
			}
			for _, block := range blocks {
				if block.Type == "image" {
					if err := visitImageReference(block.Data, visitHash); err != nil {
						return SourceInputs{}, err
					}
				}
			}
		}
	}
	names := make([]string, 0, len(hashes))
	for hash := range hashes {
		names = append(names, hash)
	}
	slices.Sort(names)
	result := SourceInputs{Version: 1, Files: make([]SourceInput, 0, len(names))}
	for _, hash := range names {
		result.Files = append(result.Files, SourceInput{Path: "blobs/" + hash, SHA256: hash})
	}
	return result, nil
}

func SourceInputsHandler(ctx context.Context, call sdk.Call, _ *sdk.Stream) (any, error) {
	var request sdk.SessionRequest
	if err := strictJSON(call.Params, &request); err != nil {
		return nil, &sdk.Error{Code: "invalid_params", Message: "session.inputs.v1 requires a SessionRequest"}
	}
	result, err := PlanSourceInputs(ctx, request)
	if err != nil {
		return nil, &sdk.Error{Code: "invalid_params", Message: err.Error()}
	}
	return result, nil
}
