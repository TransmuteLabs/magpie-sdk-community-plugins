package main

import (
	qoder "github.com/TransmuteLabs/magpie-sdk-community-plugins/packages/qoder"
	"github.com/yetone/magpie/pkg/extension/native"
)

func init() {
	if err := native.Register(qoder.NewModule(qoder.ModuleOptions{})); err != nil {
		panic(err)
	}
}

func main() {}
