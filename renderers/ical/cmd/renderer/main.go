// renderer provides the ARPRT iCalendar Job CLI and its private semantic worker.
package main

import (
	"context"
	"fmt"
	ical "github.com/dayflower/arprt-renderers/renderers/ical"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--semantic-worker" {
		if ical.Serve(os.Stdin, os.Stdout) != nil {
			os.Exit(1)
		}
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "input: invalid_config")
		os.Exit(1)
	}
	if err := ical.Run(ctx, os.Args[1:], []string{executable, "--semantic-worker"}, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
