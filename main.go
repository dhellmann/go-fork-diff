package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
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
	log.Fatal(err)
	os.Exit(1)
}

func main() {
	var (
		replaceFilterPrefix string
		workDir             string = "/tmp/go-fork-diff"
		verbose             bool
	)

	flag.StringVar(&replaceFilterPrefix, "filter-prefix", "",
		"replacement import path prefix to include")
	flag.StringVar(&replaceFilterPrefix, "f", "",
		"replacement import path prefix to include")
	flag.StringVar(&workDir, "work-dir", workDir,
		"working directory")
	flag.StringVar(&workDir, "w", workDir,
		"working directory")
	flag.BoolVar(&verbose, "v", false, "verbose output")
	flag.Parse()

	if len(flag.Args()) != 1 {
		fmt.Fprintf(os.Stderr, "ERROR: Specify exactly one go.mod file to read\n\n")
		flag.Usage()
		os.Exit(1)
	}

	log.SetFlags(0)

	modFilename := flag.Args()[0]
	modBody, err := ioutil.ReadFile(modFilename)
	handleError(err)

	mod, err := modfile.Parse(modFilename, modBody, nil)
	handleError(err)

	// TODO: Add a command line option for specifying these.
	repoAliases := []vcs.Alias{
		{
			NewPrefix: "github.com/rancher/kubernetes/staging",
			OldRepo:   "github.com/kubernetes/kubernetes",
		},
	}

	repos := make([]*vcs.Repo, 0)
	for _, replace := range mod.Replace {
		if replaceFilterPrefix != "" &&
			!strings.HasPrefix(replace.New.Path, replaceFilterPrefix) {
			continue
		}

		// If we don't have a good version specifier in the replace
		// statement, look for the original version from the thing
		// that was being replaced.
		oldVersion := replace.Old.Version
		if oldVersion == "" {
			for _, req := range mod.Require {
				if req.Mod.Path == replace.Old.Path {
					oldVersion = req.Mod.Version
					break
				}
			}
		}
		// If we still don't have a version, try stripping the suffix
		// (like "-k3s1") from the new version.
		if oldVersion == "" {
			parts := strings.Split(replace.New.Version, "-")
			if len(parts) > 1 {
				oldVersion = parts[0]
			}
		}

		repo, err := vcs.New(
			workDir,
			replace.Old.Path,
			oldVersion,
			replace.New.Path,
			replace.New.Version,
			repoAliases,
		)
		handleError(err)
		repos = append(repos, repo)
	}

	for _, repo := range repos {
		err = repo.Clone(verbose)
		handleError(err)
	}

	for _, repo := range repos {
		fmt.Printf("\n------------------------------------------------------------\n%s\n------------------------------------------------------------\n\n", repo.String())
		err = repo.Log()
		handleError(err)
		fmt.Printf("\n\n")
		err = repo.DiffStat()
		handleError(err)
	}
}
