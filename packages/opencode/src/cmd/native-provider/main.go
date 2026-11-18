package main

import (
	opencode "github.com/TransmuteLabs/magpie-sdk-plugin-provider-opencode"
	"github.com/yetone/magpie/pkg/extension/native"
)

func init() {
	if err := native.Register(opencode.NewModule(opencode.ModuleOptions{})); err != nil {
		panic(err)
	}
}

func main() {}
