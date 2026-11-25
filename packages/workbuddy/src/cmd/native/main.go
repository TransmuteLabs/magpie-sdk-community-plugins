package main

import (
	workbuddy "github.com/TransmuteLabs/magpie-sdk-community-plugins/packages/workbuddy"
	"github.com/yetone/magpie/pkg/extension/native"
)

func init() {
	if err := native.Register(workbuddy.NewModule(workbuddy.ModuleOptions{})); err != nil {
		panic(err)
	}
}

func main() {}
