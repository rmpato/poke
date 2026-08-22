// Command pogo is curl with a memory.
//
// Run it with a request and it hands your arguments to the real curl binary,
// then writes down what happened. Run it bare in a terminal and it opens a UI
// over everything it has recorded: find a request, inspect it, replay it,
// change it and run it again, compare two responses.
//
// One binary, because it is one tool. See internal/cli for the command tree.
package main

import (
	"os"

	"github.com/rmpato/pogo/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
