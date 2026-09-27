package main

import (
	"context"
	"os"

	"github.com/jacazul-ai/flow/internal/silo"
)

func main() {
	os.Exit(silo.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}
