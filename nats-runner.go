package main

import (
	"os"

	"nats-runner/internal/cli"
)

// version is injected at build time via:
//
//	go build -ldflags "-X main.version=1.2.3"
var version = "dev"

func main() {
	cli.Execute(os.Args, version)
}
