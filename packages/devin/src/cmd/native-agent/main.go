package main

import (
	devin "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin"
	"github.com/yetone/magpie/pkg/extension/native"
)

func init() {
	module, err := devin.NewAgent(devin.ModuleOptions{})
	if err != nil {
		panic(err)
	}
	if err := native.Register(module); err != nil {
		panic(err)
	}
}

func main() {}
