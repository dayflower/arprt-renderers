// Picsum downloads a photograph and writes an ARPRT renderer PNG.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func run(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("picsum", flag.ContinueOnError)
	flags.SetOutput(stderr)
	contextPath := flags.String("context", "/arprt/context.json", "Render Context v1 JSON")
	outputPath := flags.String("output", "/arprt/output.png", "Output PNG path")
	inputPath := flags.String("input", "", "Read a local image instead of downloading")
	palette := flags.String("palette", "gray4", "Output palette: gray4, bw or rgb")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	data, err := os.ReadFile(*contextPath)
	if err != nil {
		return err
	}
	context, err := parseContext(data, *palette)
	if err != nil {
		return err
	}
	var source []byte
	if *inputPath != "" {
		source, err = os.ReadFile(*inputPath)
		if err != nil {
			return err
		}
	}
	png, err := newRenderer(stdout).render(context, source, *palette)
	if err != nil {
		return err
	}
	return writeOutput(*outputPath, png)
}

func writeOutput(path string, data []byte) error {
	// Keep every write on the output mount; rename publishes only a complete PNG.
	file, err := os.CreateTemp(filepath.Dir(path), ".picsum-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Chmod(0o644); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "picsum:", err)
		os.Exit(1)
	}
}
