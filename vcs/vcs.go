package vcs

import (
	"fmt"
	"log"
	urlpkg "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dhellmann/go-fork-diff/discovery"
	"github.com/pkg/errors"
)

// Repo holds all of the information about one dependency
type Repo struct {
	// LocalPath is the directory on the local filesystem with the
	// cloned repo
	localPath string

	// OldPath is the old module path
	oldPath string

	// OldVersion is the old module version
	oldVersion string

	// OldRepo is the URL to the old repository
	oldRepo string

	// NewPath is the new module path
	newPath string

	// NewVersion is the new module version
	newVersion string

	// NewRepo is the URL to the new repository
	newRepo string
}

func (r *Repo) String() string {
	return fmt.Sprintf("%s @ %s (%s) -> %s @ %s (%s) [%s]",
		r.oldPath, r.oldVersion, r.oldRepo,
		r.newPath, r.newVersion, r.newRepo,
		r.localPath,
	)
}

// Clone configures the local copy of the repository with the relevant
// remotes
func (r *Repo) Clone(verbose bool) error {
	parentDir := filepath.Dir(r.localPath)

	err := os.MkdirAll(parentDir, 0755)
	if err != nil {
		return errors.Wrap(err, "failed to create output directory for clone")
	}

	if _, err := os.Stat(r.localPath); os.IsNotExist(err) {
		log.Printf("cloning %s", r.oldRepo)
		cmd := exec.Command("git", "-C", parentDir, "clone", r.oldRepo)
		if verbose {
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		}
		err := cmd.Run()
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("failed to clone %s", r.oldRepo))
		}
	} else {
		if verbose {
			log.Printf("found %s", r.localPath)
		}
	}

	return nil
}

// New creates a new Repo
func New(workDir, oldPath, oldVersion, newPath, newVersion string) (*Repo, error) {
	repo := Repo{
		localPath:  filepath.Join(workDir, oldPath),
		oldPath:    oldPath,
		oldVersion: oldVersion,
		newPath:    newPath,
		newVersion: newVersion,
	}

	oldRepo, err := resolveOne(oldPath)
	if err != nil {
		return nil, errors.Wrap(err, "could not resolve old repository from module path")
	}
	repo.oldRepo = oldRepo

	newRepo, err := resolveOne(newPath)
	if err != nil {
		return nil, errors.Wrap(err, "could not resolve new repository from module path")
	}
	repo.newRepo = newRepo

	return &repo, nil
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
