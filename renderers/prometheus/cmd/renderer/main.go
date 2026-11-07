package main

import (
	"context"
	"fmt"
	renderer "github.com/dayflower/arprt-renderers/renderers/prometheus"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := renderer.Run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "prometheus:", err)
		os.Exit(1)
	}
}
