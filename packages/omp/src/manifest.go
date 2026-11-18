package ompsessions

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"runtime"

	sdk "github.com/yetone/magpie/pkg/extension"
)

//go:embed magpie-plugin.json
var declaration []byte

func Manifest() sdk.Manifest {
	var manifest sdk.Manifest
	if err := json.Unmarshal(declaration, &manifest); err != nil {
		panic(fmt.Errorf("invalid embedded OMP manifest: %w", err))
	}
	switch runtime.GOOS {
	case "darwin":
		manifest.Entry = "plugin.dylib"
	case "windows":
		manifest.Entry = "plugin.dll"
	default:
		manifest.Entry = "plugin.so"
	}
	manifest.Compatibility.Platforms = []string{runtime.GOOS + "/" + runtime.GOARCH}
	return manifest
}

func Module() sdk.Module {
	return sdk.Module{Manifest: Manifest(), Handlers: map[string]sdk.Handler{
		sdk.MethodSessionConvert:    Handler,
		sdk.MethodSessionSourceRead: SourceReadHandler,
	}, Actions: map[string]sdk.Handler{SourceInputsAction: sourcePreviewInputsHandler},
		Start: sourcePreviewStart}
}
