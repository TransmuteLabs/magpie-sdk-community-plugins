package main

import (
	minimax "github.com/TransmuteLabs/magpie-sdk-community-plugins/packages/minimax"
	"github.com/yetone/magpie/pkg/extension/native"
)

func init() {
	if err := native.Register(minimax.NewModule(minimax.ModuleOptions{})); err != nil {
		panic(err)
	}
}
func main() {}
