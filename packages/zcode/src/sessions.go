package zcode

import (
	"context"
	"io/fs"
	"path"
	"strings"
	"unicode"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/history"
	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/historydb"
	sdk "github.com/yetone/magpie/pkg/extension"
)

const historyAdapterID = "zcode-history"
const sqliteAdapterID = "zcode-sqlite-history"

func historyOutputPath(value string) bool {
	if len(value) > 1024 || value == "." || !fs.ValidPath(value) || path.Clean(value) != value ||
		strings.ContainsAny(value, "\\:") || strings.ContainsFunc(value, unicode.IsControl) {
		return false
	}
	for _, component := range strings.Split(value, "/") {
		if strings.HasPrefix(component, ".magpie-extension") {
			return false
		}
	}
	return true
}

func (owner *module) sessionConvert(ctx context.Context, call sdk.Call, _ *sdk.Stream) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var request sdk.SessionRequest
	if err := decode(call.Params, &request); err != nil {
		return nil, err
	}
	var options struct {
		OutputPath string `json:"outputPath"`
	}
	if request.ID != historyAdapterID && request.ID != sqliteAdapterID || len(request.Files) != 1 || request.From == request.To ||
		decode(request.Options, &options) != nil || !historyOutputPath(options.OutputPath) {
		return nil, invalid("Offline history conversion requires its declared adapter, one document, distinct formats and an explicit relative outputPath")
	}
	source := request.Files[0]
	if source.Format != request.From || !historyOutputPath(source.Path) ||
		strings.EqualFold(source.Path, options.OutputPath) {
		return nil, invalid("Offline conversion cannot mix formats, use unsafe paths or overwrite its source")
	}
	var body []byte
	var identity string
	var err error
	if request.ID == sqliteAdapterID {
		body, identity, err = historydb.Convert(ctx, request.From, request.To, source.Body)
	} else {
		body, err = history.Convert(request.From, request.To, source.Body)
		if err == nil {
			document, readErr := history.Read(request.From, source.Body)
			if readErr != nil {
				err = readErr
			} else {
				identity = document.Summary().SHA256
			}
		}
	}
	if err != nil {
		return nil, invalid(err.Error())
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return sdk.SessionResult{
		ID:    "zcode-history-" + identity[:32],
		Files: []sdk.SessionDocument{{Path: options.OutputPath, Format: request.To, Body: body}},
	}, nil
}
