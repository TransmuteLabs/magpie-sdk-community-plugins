package main

import (
	zcode "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode"
	"github.com/yetone/magpie/pkg/extension/native"
)

func init() {
	if err := native.Register(zcode.NewControlModule(zcode.ModuleOptions{})); err != nil {
		panic(err)
	}
}

func main() {}
