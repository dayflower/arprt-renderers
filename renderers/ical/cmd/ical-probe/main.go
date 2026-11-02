// ical-probe exercises calendar semantics only. It does not fetch or render.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	ical "github.com/dayflower/arprt-renderers/renderers/ical"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--semantic-worker" {
		if ical.Serve(os.Stdin, os.Stdout) != nil {
			os.Exit(1)
		}
		return
	}
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "invalid_arguments")
		os.Exit(1)
	}
	data, err := io.ReadAll(io.LimitReader(os.Stdin, 6*ical.MaxBody+1))
	var req ical.Request
	if err != nil || len(data) > 6*ical.MaxBody || json.Unmarshal(data, &req) != nil {
		fmt.Fprintln(os.Stderr, "invalid_request")
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	executable, err := os.Executable()
	if err != nil {
		os.Exit(1)
	}
	instances, err := ical.Supervise(ctx, []string{executable, "--semantic-worker"}, req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if json.NewEncoder(os.Stdout).Encode(instances) != nil {
		os.Exit(1)
	}
}
