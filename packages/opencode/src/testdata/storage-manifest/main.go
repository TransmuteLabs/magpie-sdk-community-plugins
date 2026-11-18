package main

import (
	"encoding/json"
	"os"

	opencode "github.com/TransmuteLabs/magpie-sdk-plugin-provider-opencode"
)

func main() {
	module := opencode.NewWindowModule(opencode.ModuleOptions{})
	if err := json.NewEncoder(os.Stdout).Encode(module.Manifest); err != nil {
		panic(err)
	}
}
