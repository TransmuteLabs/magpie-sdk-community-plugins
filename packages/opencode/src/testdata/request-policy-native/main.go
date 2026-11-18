package main

import (
	opencode "github.com/TransmuteLabs/magpie-sdk-plugin-provider-opencode"
	"github.com/yetone/magpie/pkg/extension/native"
)

var endpoint string

func init() {
	if endpoint == "" {
		panic("policy fixture requires its compiled loopback endpoint")
	}
	native.Register(opencode.NewWindowModule(opencode.ModuleOptions{
		Endpoints: opencode.Endpoints{Console: endpoint + "/console", Inference: endpoint},
	}))
}

func main() {}
