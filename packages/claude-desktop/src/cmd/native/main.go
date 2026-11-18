package main

import (
	"encoding/json"
	"os"

	desktop "github.com/TransmuteLabs/magpie-sdk-plugin-agent-claude-desktop"
	"github.com/yetone/magpie/pkg/extension/native"
)

func init() {
	if err := native.Register(desktop.Module()); err != nil {
		panic(err)
	}
}

func main() {
	if err := json.NewEncoder(os.Stdout).Encode(desktop.Manifest()); err != nil {
		panic(err)
	}
}
