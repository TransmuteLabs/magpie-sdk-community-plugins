package main

import (
	codearts "github.com/TransmuteLabs/magpie-sdk-community-plugins/packages/codearts"
	"github.com/yetone/magpie/pkg/extension/native"
)

func init() {
	if err := native.Register(codearts.NewModule(codearts.ModuleOptions{})); err != nil {
		panic(err)
	}
}

func main() {}
