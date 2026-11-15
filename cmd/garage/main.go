// Command garage is the Builder Garage command-line interface.
package main

import (
	"os"

	"github.com/bdonaldharris/builder-garage/internal/cli"
)

// version is the CLI version reported by "garage version".
//
// It defaults to a development value, which a plain "go build" keeps. The
// Makefile and install.sh replace it at link time with a version derived from
// the checkout's Git tags, such as v0.1.0 (docs/adr/0002-semantic-versioning.md),
// and a VERSION setting overrides that:
//
//	go build -ldflags "-X main.version=1.2.3" ./cmd/garage
var version = "dev"

func main() {
	// Cobra reports the error on stderr; the exit status signals the failure.
	if err := cli.NewRootCommand(version).Execute(); err != nil {
		os.Exit(1)
	}
}
