package devin

import (
	_ "embed"
	"encoding/json"
	"runtime"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
	sdk "github.com/yetone/magpie/pkg/extension"
	"github.com/yetone/magpie/pkg/protocolext"
)

//go:embed magpie-plugin.json
var declaration []byte

//go:embed magpie-inbound.json
var inboundDeclaration []byte

func New() (sdk.Module, error) { return NewModule(ModuleOptions{}) }

func NewInbound() (sdk.Module, error) {
	var manifest sdk.Manifest
	if err := json.Unmarshal(inboundDeclaration, &manifest); err != nil {
		return sdk.Module{}, err
	}
	switch runtime.GOOS {
	case "darwin":
		manifest.Entry = "plugin.dylib"
	case "windows":
		manifest.Entry = "plugin.dll"
	default:
		manifest.Entry = "plugin.so"
	}
	module := sdk.Module{
		Manifest: manifest,
		Inbounds: []protocolext.Inbound{
			codec.Adapter{},
			codec.CatalogAdapter{Kind: codec.CliModels},
			codec.CatalogAdapter{Kind: codec.CommandModels},
			codec.CatalogAdapter{Kind: codec.CascadeModels},
			codec.CatalogAdapter{Kind: codec.CliModelsSite},
			codec.CatalogAdapter{Kind: codec.CommandModelsSite},
			codec.CatalogAdapter{Kind: codec.CascadeModelsSite},
			codec.CatalogAdapter{Kind: codec.ModelStatuses},
		},
	}
	return module, module.Validate()
}
