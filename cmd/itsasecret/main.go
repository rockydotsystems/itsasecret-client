package main

import (
	"os"

	"itsasecret.dev/cli/internal/commands"
)

func main() {
	if err := commands.NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
