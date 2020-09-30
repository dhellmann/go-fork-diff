package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	urlpkg "net/url"
	"os"
	"strings"

	"github.com/dhellmann/go-fork-diff/discovery"
	"github.com/pkg/errors"
	"golang.org/x/mod/modfile"
)

func init() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "%s [options] go-mod-file\n\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  go-mod-file\n")
		fmt.Fprintf(flag.CommandLine.Output(), "    path to a go.mod file\n")
		fmt.Fprintf(flag.CommandLine.Output(), "\n")
		flag.PrintDefaults()
		fmt.Fprintf(flag.CommandLine.Output(), "\n")
	}
}

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

func handleError(err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "%s\n", err)
	os.Exit(1)
}

func main() {
	var (
		replaceFilterPrefix string
	)

	flag.StringVar(&replaceFilterPrefix, "filter-prefix", "",
		"replacement import path prefix to include")
	flag.StringVar(&replaceFilterPrefix, "f", "",
		"replacement import path prefix to include")
	flag.Parse()

	if len(flag.Args()) != 1 {
		fmt.Fprintf(os.Stderr, "ERROR: Specify exactly one go.mod file to read\n\n")
		flag.Usage()
		os.Exit(1)
	}

	modFilename := flag.Args()[0]
	modBody, err := ioutil.ReadFile(modFilename)
	handleError(err)

	mod, err := modfile.Parse(modFilename, modBody, nil)
	handleError(err)

	for _, replace := range mod.Replace {
		if replaceFilterPrefix != "" && !strings.HasPrefix(replace.New.Path, replaceFilterPrefix) {
			continue
		}
		fmt.Printf("%s @ %s replaces %s @ %s\n",
			replace.New.Path,
			replace.New.Version,
			replace.Old.Path,
			replace.Old.Version,
		)
	}

	// for _, importPath := range flag.Args() {
	// 	fmt.Printf("checking %s\n", importPath)
	// 	repoRoot, err := resolveOne(importPath)
	// 	if err != nil {
	// 		fmt.Printf("ERROR: %s\n", err)
	// 		continue
	// 	}
	// 	fmt.Printf("-> %s\n", repoRoot)
	// }
}
