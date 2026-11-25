package main

import (
	kimiwork "github.com/TransmuteLabs/magpie-sdk-community-plugins/packages/kimi-work"
	"github.com/yetone/magpie/pkg/extension/native"
)

func init() {
	if err := native.Register(kimiwork.NewModule(kimiwork.ModuleOptions{})); err != nil {
		panic(err)
	}
}

func main() {}
