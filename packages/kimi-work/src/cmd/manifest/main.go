package main

import (
	"encoding/json"
	"os"

	kimiwork "github.com/TransmuteLabs/magpie-sdk-community-plugins/packages/kimi-work"
)

func main() {
	module := kimiwork.NewModule(kimiwork.ModuleOptions{})
	if err := module.Validate(); err != nil {
		panic(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(module.Manifest); err != nil {
		panic(err)
	}
}
