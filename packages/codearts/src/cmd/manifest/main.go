package main

import (
	"encoding/json"
	"os"

	codearts "github.com/TransmuteLabs/magpie-sdk-community-plugins/packages/codearts"
)

func main() {
	module := codearts.NewModule(codearts.ModuleOptions{})
	if err := module.Validate(); err != nil {
		panic(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(module.Manifest); err != nil {
		panic(err)
	}
}
