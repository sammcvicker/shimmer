package main

import (
	"os"

	"github.com/sammcvicker/shimmer/internal/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
