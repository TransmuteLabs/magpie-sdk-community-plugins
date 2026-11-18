package main

import (
	"encoding/json"
	"os"

	omp "github.com/TransmuteLabs/magpie-sdk-plugin-session-omp"
	"github.com/yetone/magpie/pkg/extension/native"
)

func init() {
	if err := native.Register(omp.Module()); err != nil {
		panic(err)
	}
}

func main() {
	if err := json.NewEncoder(os.Stdout).Encode(omp.Module().Manifest); err != nil {
		panic(err)
	}
}
