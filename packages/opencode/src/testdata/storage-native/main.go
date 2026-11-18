package main

import (
	opencode "github.com/TransmuteLabs/magpie-sdk-plugin-provider-opencode"
	"github.com/yetone/magpie/pkg/extension/native"
)

func init() {
	module := opencode.NewWindowModule(opencode.ModuleOptions{})
	if err := native.Register(module); err != nil {
		panic(err)
	}
}

func main() {}
