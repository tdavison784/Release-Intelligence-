// Command schemagen generates the JSON Schemas (draft 2020-12) of the
// platform's output documents from the Go types in package domain:
//
//	schemas/upgrade-edge.schema.json   domain.UpgradeEdge, the primary output
//	schemas/release.schema.json        domain.Release, what an edge is built from
//
// The schemas are derived by reflection over the `json` struct tags (see
// gen.go); the closed enum sets, descriptions and the few constraints that
// do not follow from the types alone live in meta.go.
//
// Regenerate after changing anything in internal/domain:
//
//	go run ./internal/domain/schemagen        # from anywhere inside the module
//	go generate ./internal/domain/schemagen
//
// With -check the files are not written; the command exits non-zero when the
// checked-in schemas are stale (the package test performs the same check).
package main

//go:generate go run .

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func main() {
	check := flag.Bool("check", false, "do not write; exit 1 if the checked-in schemas differ from the generated ones")
	out := flag.String("out", "", "output directory (default: schemas/ at the module root)")
	flag.Parse()
	if err := run(*out, *check); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(out string, check bool) error {
	files, err := generate()
	if err != nil {
		return err
	}
	if out == "" {
		root, err := moduleRoot()
		if err != nil {
			return err
		}
		out = filepath.Join(root, "schemas")
	}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	stale := false
	for _, n := range names {
		path := filepath.Join(out, n)
		if check {
			have, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(have, files[n]) {
				fmt.Fprintf(os.Stderr, "stale: %s\n", path)
				stale = true
			}
			continue
		}
		if err := os.MkdirAll(out, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, files[n], 0o644); err != nil {
			return err
		}
		fmt.Printf("wrote %s\n", path)
	}
	if stale {
		return errors.New("schemas are stale; run: go run ./internal/domain/schemagen")
	}
	return nil
}

// moduleRoot finds the directory containing go.mod, starting at the working
// directory, so the command works from any directory of the module.
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("schemagen: go.mod not found above the working directory")
		}
		dir = parent
	}
}
