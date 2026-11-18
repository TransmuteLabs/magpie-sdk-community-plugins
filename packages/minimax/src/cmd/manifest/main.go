package main

import (
	"encoding/json"
	minimax "github.com/TransmuteLabs/magpie-sdk-community-plugins/packages/minimax"
	"os"
)

func main() {
	module := minimax.NewModule(minimax.ModuleOptions{})
	if err := module.Validate(); err != nil {
		panic(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(module.Manifest); err != nil {
		panic(err)
	}
}
