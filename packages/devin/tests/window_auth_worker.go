package main

import (
	"context"
	"encoding/json"
	"os"

	devin "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin"
	sdk "github.com/yetone/magpie/pkg/extension"
)

func main() {
	module, err := devin.NewAgent(devin.ModuleOptions{})
	if err != nil {
		os.Exit(1)
	}
	module.Manifest.Runtime = "exec"
	module.Manifest.Entry = "bin/devin"
	if module.Validate() != nil {
		os.Exit(1)
	}
	if len(os.Args) == 2 && os.Args[1] == "--manifest" {
		if json.NewEncoder(os.Stdout).Encode(module.Manifest) != nil {
			os.Exit(1)
		}
		return
	}
	if err := sdk.Serve(context.Background(), os.Stdin, os.Stdout, module); err != nil {
		os.Exit(1)
	}
}
