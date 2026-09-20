package main

import (
	"context"
	"os"

	flow "github.com/jacazul-ai/flow"
)

func main() {
	streams := flow.Streams{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}
	os.Exit(flow.Run(context.Background(), os.Args[1:], flow.EnvFromOS(), streams))
}
