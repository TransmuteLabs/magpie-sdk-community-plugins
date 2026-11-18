package ompsessions

import (
	"context"
	"encoding/json"
	"unicode/utf8"

	sdk "github.com/yetone/magpie/pkg/extension"
)

const SourceDescriptorID = "omp-history"

func SourcePreviewModule() sdk.Module {
	return Module()
}

func SourceReadHandler(ctx context.Context, call sdk.Call, _ *sdk.Stream) (any, error) {
	permission, err := currentSourceReadPermission(ctx)
	if err != nil {
		return nil, err
	}
	request, err := sdk.DecodeSessionSourceRequest(Manifest(), call.Params)
	if err != nil {
		return nil, &sdk.Error{Code: "invalid_params", Message: err.Error()}
	}
	result, err := ReadSessionSource(ctx, request)
	if err != nil {
		return nil, &sdk.Error{Code: "invalid_params", Message: err.Error()}
	}
	if err := permission.check(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

func sourcePreviewInputsHandler(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request sdk.SessionRequest
	if err := strictJSON(call.Params, &request); err != nil {
		return nil, &sdk.Error{Code: "invalid_params", Message: err.Error()}
	}
	if request.ID != SourceDescriptorID {
		return SourceInputsHandler(ctx, call, stream)
	}
	permission, err := currentSourceReadPermission(ctx)
	if err != nil {
		return nil, err
	}
	if request.From != OMPFormat || request.To != OMPFormat || len(request.Files) != 1 ||
		request.Files[0].Format != OMPFormat {
		return nil, &sdk.Error{Code: "invalid_params", Message: "Source input planning requires one OMP v3 journal"}
	}
	descriptor := Manifest().SessionSources[0]
	id, err := descriptor.Layout.SessionID(request.Files[0].Path)
	if err != nil {
		return nil, err
	}
	if _, _, err := decodeSourceJournal(ctx, request.Files[0].Body, id); err != nil {
		return nil, err
	}
	if err := permission.check(ctx); err != nil {
		return nil, err
	}
	return SourceInputs{Version: 1, Files: []SourceInput{}}, nil
}

func ReadSessionSource(ctx context.Context, request sdk.SessionSourceReadRequest) (sdk.SessionSourceReadResult, error) {
	if err := ctx.Err(); err != nil {
		return sdk.SessionSourceReadResult{}, err
	}
	if err := sdk.ValidateSessionSourceRequest(Manifest(), request); err != nil {
		return sdk.SessionSourceReadResult{}, err
	}
	summary, branch, err := decodeSourceDocuments(ctx, request)
	if err != nil {
		return sdk.SessionSourceReadResult{}, err
	}
	result := sdk.SessionSourceReadResult{Mode: request.Mode, SessionID: request.Locator.SessionID}
	if request.Mode == "summary" {
		result.Summary = &summary
	} else {
		result.Parts = []sdk.SessionSourcePart{}
		remaining := request.Limits.MaxTextBytes
		for _, entry := range branch {
			if err := ctx.Err(); err != nil {
				return sdk.SessionSourceReadResult{}, err
			}
			parts, err := sourceEntryParts(entry)
			if err != nil {
				return sdk.SessionSourceReadResult{}, err
			}
			for _, part := range parts {
				if len(result.Parts) >= request.Limits.MaxParts || remaining == 0 {
					result.Cut = true
					continue
				}
				if len(part.Text) > remaining {
					original := len(part.Text)
					part.Text = part.Text[:remaining]
					for !utf8.ValidString(part.Text) {
						part.Text = part.Text[:len(part.Text)-1]
					}
					part.Cut = original - len(part.Text)
					result.Cut = true
				}
				remaining -= len(part.Text)
				result.Parts = append(result.Parts, part)
			}
		}
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return sdk.SessionSourceReadResult{}, err
	}
	if _, err := sdk.DecodeSessionSourceResult(request, raw); err != nil {
		return sdk.SessionSourceReadResult{}, err
	}
	return result, ctx.Err()
}
