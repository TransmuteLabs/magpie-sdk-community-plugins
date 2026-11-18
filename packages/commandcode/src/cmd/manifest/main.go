package main

import (
	"encoding/json"
	"fmt"
	"os"

	commandcode "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-commandcode"
	sdk "github.com/yetone/magpie/pkg/extension"
)

func main() {
	contract := sdk.FullContract()
	if contract.Version.Major != 1 || contract.Version.Minor < 12 || !contract.Has(sdk.RequestPolicy) {
		fmt.Fprintln(os.Stderr, "CommandCode request policy requires public SDK API1.12")
		os.Exit(2)
	}
	if err := json.NewEncoder(os.Stdout).Encode(commandcode.Manifest()); err != nil {
		panic(err)
	}
}
