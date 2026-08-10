// Command brain is the WOSY "brain" CLI — the SCRIBE/router's hands.
// The get/set/append/find/list interface (master §B6) lives in internal/cli.
package main

import (
	"os"

	"wosy.local/brain/internal/cli"
)

func main() { os.Exit(cli.Run(os.Args[1:])) }
