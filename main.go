package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/dhellmann/go-fork-diff/vcs"
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
		workDir             string = "/tmp/go-fork-diff"
	)

	flag.StringVar(&replaceFilterPrefix, "filter-prefix", "",
		"replacement import path prefix to include")
	flag.StringVar(&replaceFilterPrefix, "f", "",
		"replacement import path prefix to include")
	flag.StringVar(&workDir, "work-dir", workDir,
		"working directory")
	flag.StringVar(&workDir, "w", workDir,
		"working directory")
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
		repo, err := vcs.New(
			workDir,
			replace.Old.Path,
			replace.Old.Version,
			replace.New.Path,
			replace.New.Version,
		)
		handleError(err)
		fmt.Printf("%v\n", repo)
		err = repo.Clone()
		handleError(err)
	}
}
