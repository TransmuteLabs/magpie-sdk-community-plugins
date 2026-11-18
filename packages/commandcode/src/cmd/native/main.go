package main

import (
	commandcode "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-commandcode"
	"github.com/yetone/magpie/pkg/extension/native"
)

func init() {
	if err := native.Register(commandcode.Module()); err != nil {
		panic(err)
	}
}

func main() {}
