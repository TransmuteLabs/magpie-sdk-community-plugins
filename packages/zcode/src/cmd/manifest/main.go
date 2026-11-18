package main

import (
	"encoding/json"
	"flag"
	"os"

	zcode "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode"
)

func main() {
	headless := flag.Bool("headless", false, "declare the historical provider-only SDK 1.4 compatibility artifact")
	flag.Parse()
	if flag.NArg() != 0 {
		panic("unexpected manifest arguments")
	}
	manifest := zcode.ControlManifest()
	if *headless {
		manifest = zcode.Manifest()
	}
	if err := manifest.Validate(); err != nil {
		panic(err)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(manifest); err != nil {
		panic(err)
	}
}
