package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	opencode "github.com/TransmuteLabs/magpie-sdk-plugin-provider-opencode"
	sdk "github.com/yetone/magpie/pkg/extension"
)

func main() {
	contract := sdk.FullContract()
	if contract.Version.Major != 1 || contract.Version.Minor < 12 || !contract.Has(sdk.RequestPolicy) {
		fmt.Fprintln(os.Stderr, "OpenCode request policy requires public SDK API1.12")
		os.Exit(2)
	}
	provider := flag.Bool("compat-provider", false, "emit historical provider-only compatibility fixture")
	agent := flag.Bool("compat-agent", false, "emit historical headless-agent compatibility fixture")
	api8 := flag.Bool("compat-api8", false, "legacy full-package flag; request-policy runtime requires API1.12")
	legacyAgent := flag.Bool("agent", false, "alias for compat-agent")
	window := flag.Bool("window", false, "alias for the default full package")
	flag.Parse()
	choices := 0
	for _, selected := range []bool{*provider, *agent || *legacyAgent, *api8, *window} {
		if selected {
			choices++
		}
	}
	if flag.NArg() != 0 || choices > 1 {
		os.Exit(2)
	}
	manifest := opencode.WindowManifest()
	if *provider {
		manifest = opencode.Manifest()
	}
	if *agent || *legacyAgent {
		manifest = opencode.AgentManifest()
	}
	if *api8 {
		manifest = opencode.API8WindowManifest()
	}
	if json.NewEncoder(os.Stdout).Encode(manifest) != nil {
		os.Exit(1)
	}
}
