package main

import (
	"encoding/json"
	"os"

	workbuddy "github.com/TransmuteLabs/magpie-sdk-community-plugins/packages/workbuddy"
)

func main() {
	module := workbuddy.NewModule(workbuddy.ModuleOptions{})
	if err := module.Validate(); err != nil {
		panic(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(module.Manifest); err != nil {
		panic(err)
	}
}
