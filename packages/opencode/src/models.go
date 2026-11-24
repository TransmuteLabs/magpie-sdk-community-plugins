package opencode

import (
	_ "embed"
	"encoding/json"
	"runtime"

	sdk "github.com/yetone/magpie/pkg/extension"
)

// The entries are selected from the donor's reviewed public catalog
// fixture opencode-go-models-2026-10-31.json, not guessed from model names.
// Prices are omitted because that fixture includes peak multipliers.
//
//go:embed testdata/compat/provider-0.2.0.json
var declaration []byte

func Manifest() sdk.Manifest {
	var manifest sdk.Manifest
	if err := json.Unmarshal(declaration, &manifest); err != nil {
		panic("invalid packaged OpenCode manifest")
	}
	manifest.Contract.Version.Minor = 12
	manifest.Compatibility.HostAPI = "^1.12"
	manifest.Contract.Capabilities = append(manifest.Contract.Capabilities, sdk.RequestPolicy)
	for index := range manifest.Providers[0].Models {
		model := &manifest.Providers[0].Models[index]
		if model.ID == "glm-5.2" {
			model.ID, model.Name, model.Released = "glm-5.3", "GLM-5.3", "2026-08-14"
			model.Efforts = []string{"low", "high", "max"}
		}
	}
	switch runtime.GOOS {
	case "darwin":
		manifest.Entry = "lib/opencode.dylib"
	case "windows":
		manifest.Entry = "lib/opencode.dll"
	}
	return manifest
}

func Models() []sdk.Model {
	return Manifest().Providers[0].Models
}

func modelSupports(id, protocol string) bool {
	_, supported := supportsCatalogModel(Models(), id, protocol)
	return supported
}
