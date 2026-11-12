// Command garage is the Builder Garage command-line interface.
package main

import (
	"os"

	"github.com/bdonaldharris/builder-garage/internal/cli"
)

// version is the CLI version reported by "garage version".
//
// It defaults to a development value. Release builds replace it at link time:
//
//	go build -ldflags "-X main.version=1.2.3" ./cmd/garage
var version = "dev"

func main() {
	// Cobra reports the error on stderr; the exit status signals the failure.
	if err := cli.NewRootCommand(version).Execute(); err != nil {
		os.Exit(1)
	}
}
