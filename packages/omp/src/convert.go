package ompsessions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

const (
	ModuleID = "omp-sessions"
	// The archived journal, assets and portable projection can exceed the
	// original input size. The host's checked-plan payload limit is 16 MiB.
	MaxDocumentBytes = 16 << 20
	maxSessionWire   = sdk.MaxFrameBytes - 4096
)

type Options struct {
	// Output is a document filename when importing, a session bucket directory
	// when exporting. Blob outputs are relative to the same checked root.
	Output string `json:"output"`
}

func isOMP(format string) bool { return format == "omp" || format == OMPFormat }

func safePath(name string) bool {
	if !fs.ValidPath(name) || name == "." || strings.ContainsAny(name, "\\:\x00\r\n") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(part, ".magpie-extension") {
			return false
		}
	}
	return true
}

// Convert returns bytes for a host checked plan. It opens no files and never
// discovers a user's session or asset stores.
func Convert(ctx context.Context, request sdk.SessionRequest) (sdk.SessionResult, error) {
	if err := ctx.Err(); err != nil {
		return sdk.SessionResult{}, err
	}
	if err := sdk.ValidateSessionRelatedInputs(Manifest(), request); err != nil {
		return sdk.SessionResult{}, err
	}
	if request.Related != nil && (request.ID != "codex-to-omp" || request.To != "omp") {
		return sdk.SessionResult{}, errors.New("captured Codex lineage requires the codex-to-omp route")
	}
	if request.ID == "omp-to-claude" && isOMP(request.From) && request.To == "claude" {
		return convertToClaude(ctx, request)
	}
	if request.ID == "omp-to-codex" && isOMP(request.From) && request.To == "codex" {
		return convertToCodex(ctx, request)
	}
	if (request.ID == "claude-to-omp" && request.From == "claude" || request.ID == "codex-to-omp" && request.From == "codex") && request.To == "omp" {
		return convertNativeToOMP(ctx, request)
	}
	importing := isOMP(request.From) && request.To == DocumentFormat && request.ID == "omp-to-document"
	exporting := request.From == DocumentFormat && isOMP(request.To) && request.ID == "document-to-omp"
	if !importing && !exporting {
		return sdk.SessionResult{}, errors.New("unsupported OMP conversion route or descriptor")
	}
	var options Options
	if strictJSON(request.Options, &options) != nil || !safePath(options.Output) {
		return sdk.SessionResult{}, errors.New("an explicit relative output path is required")
	}
	journal, assets, err := sessionInputDocuments(ctx, request, importing)
	if err != nil {
		return sdk.SessionResult{}, err
	}
	var result sdk.SessionResult
	if importing {
		document, err := Decode(ctx, journal, assets)
		if err != nil {
			return sdk.SessionResult{}, err
		}
		body, err := json.MarshalIndent(document, "", "  ")
		if err != nil {
			return sdk.SessionResult{}, err
		}
		result.ID = document.ID
		result.Files = []sdk.SessionDocument{{Path: options.Output, Format: request.To, Body: append(body, '\n')}}
	} else {
		if len(request.Files) != 1 {
			return sdk.SessionResult{}, errors.New("one self-contained OMP conversation document is required")
		}
		var document Document
		if err := strictJSON(journal, &document); err != nil {
			return sdk.SessionResult{}, fmt.Errorf("invalid OMP conversation document: %w", err)
		}
		encoded, err := Encode(ctx, document, time.Now())
		if err != nil {
			return sdk.SessionResult{}, err
		}
		result.ID = encoded.ID
		result.Files = []sdk.SessionDocument{{Path: path.Join(options.Output, encoded.Filename), Format: request.To, Body: encoded.Journal}}
		for _, asset := range encoded.Assets {
			result.Files = append(result.Files, sdk.SessionDocument{Path: "blobs/" + asset.SHA256, Format: request.To, Body: asset.Body})
		}
	}
	size := 0
	outputLimit := MaxDocumentBytes
	if exporting {
		outputLimit = MaxBytes
	}
	for _, file := range result.Files {
		if !safePath(file.Path) {
			return sdk.SessionResult{}, errors.New("conversion produced an invalid output path")
		}
		size += len(file.Body)
	}
	wire, err := json.Marshal(result)
	if err != nil || size > outputLimit || len(result.Files) > MaxFiles || len(wire) > maxSessionWire {
		return sdk.SessionResult{}, errors.New("session output exceeds checked-plan or SDK frame budgets")
	}
	return result, ctx.Err()
}

func sessionInputDocuments(ctx context.Context, request sdk.SessionRequest, importing bool) ([]byte, []Asset, error) {
	if len(request.Files) == 0 || len(request.Files) > MaxFiles {
		return nil, nil, errors.New("session input file count exceeds its budget")
	}
	seen := map[string]bool{}
	total := 0
	inputLimit := MaxBytes
	if !importing {
		inputLimit = MaxDocumentBytes
	}
	var journal []byte
	mainSeen := false
	var assets []Asset
	for _, file := range request.Files {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if !safePath(file.Path) || file.Format != request.From || seen[file.Path] {
			return nil, nil, errors.New("session inputs need unique relative paths and the declared format")
		}
		seen[file.Path] = true
		total += len(file.Body)
		if total > inputLimit {
			return nil, nil, errors.New("session inputs exceed their byte budget")
		}
		if importing && strings.HasPrefix(file.Path, "blobs/") {
			hash := strings.TrimPrefix(file.Path, "blobs/")
			if !validHash(hash) {
				return nil, nil, errors.New("asset path must be blobs/<lowercase-sha256>")
			}
			assets = append(assets, Asset{SHA256: hash, Body: file.Body})
		} else if !mainSeen {
			journal = file.Body
			mainSeen = true
		} else {
			return nil, nil, errors.New("one main journal or conversation document is required; subagent artifacts need a separate explicit conversion")
		}
	}
	requestWire, err := json.Marshal(request)
	if err != nil || len(requestWire) > maxSessionWire {
		return nil, nil, errors.New("session input exceeds the SDK frame budget")
	}
	return journal, assets, nil
}

func Handler(ctx context.Context, call sdk.Call, _ *sdk.Stream) (any, error) {
	var request sdk.SessionRequest
	if err := strictJSON(call.Params, &request); err != nil {
		return nil, &sdk.Error{Code: "invalid_params", Message: "session.convert requires a valid explicit SessionRequest"}
	}
	result, err := Convert(ctx, request)
	if err != nil {
		return nil, &sdk.Error{Code: "invalid_params", Message: err.Error()}
	}
	return result, nil
}
