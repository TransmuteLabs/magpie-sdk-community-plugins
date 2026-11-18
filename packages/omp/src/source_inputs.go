package ompsessions

import (
	"context"
	"encoding/json"
	"errors"
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
		parseErr = visitAttachmentReferences(record.Type, record.Message, func(hash string) error {
			hashes[hash] = true
			if len(hashes) >= MaxFiles {
				return errors.New("OMP attachment count exceeds the input budget")
			}
			return nil
		})
	})
	if err != nil {
		return SourceInputs{}, err
	}
	if parseErr != nil {
		return SourceInputs{}, parseErr
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
