package main

import (
	"encoding/json"
	"os"

	qoder "github.com/TransmuteLabs/magpie-sdk-community-plugins/packages/qoder"
)

func main() {
	module := qoder.NewModule(qoder.ModuleOptions{})
	if err := module.Validate(); err != nil {
		panic(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(module.Manifest); err != nil {
		panic(err)
	}
}
