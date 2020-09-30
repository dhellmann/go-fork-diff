package main

import (
	"flag"
	"fmt"
	urlpkg "net/url"
	"os"
	"strings"

	"github.com/dhellmann/go-fork-diff/discovery"
	"github.com/pkg/errors"
)

func resolveOne(importPath string) (string, error) {
	if strings.HasPrefix(importPath, "github.com/") {
		url, err := urlpkg.Parse(fmt.Sprintf("https://%s", importPath))
		if err != nil {
			return "", errors.Wrap(err, "could not parse github path")
		}
		repoPath := strings.Split(url.Path, "/")
		// The 0th element of repoPath is "" so to get the base path
		// of the repo we join the first 3 elements to get /org/repo
		url.Path = strings.Join(repoPath[:3], "/")
		return url.String(), nil
	}

	repoRoot, err := discovery.RepoRootForImportDynamic(importPath)
	if err != nil {
		return "", errors.Wrap(err, "could not determine repo root")
	}
	return repoRoot, nil
}

func main() {
	flag.Parse()

	if len(flag.Args()) == 0 {
		fmt.Fprintf(os.Stderr, "ERROR: Specify at least one import path\n")
		os.Exit(1)
	}
	for _, importPath := range flag.Args() {
		fmt.Printf("checking %s\n", importPath)
		repoRoot, err := resolveOne(importPath)
		if err != nil {
			fmt.Printf("ERROR: %s\n", err)
			continue
		}
		fmt.Printf("-> %s\n", repoRoot)
	}
}
