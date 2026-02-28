package main

import (
	"fmt"
	"os"

	"github.com/MikeS071/contentai/cmd"
)

func main() {
	if err := cmd.NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
