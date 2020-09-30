package vcs

import (
	"fmt"
	urlpkg "net/url"
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
	return fmt.Sprintf("%s @ %s (%s) -> %s @ %s (%s)",
		r.oldPath, r.oldVersion, r.oldRepo,
		r.newPath, r.newVersion, r.newRepo,
	)
}

// New creates a new Repo
func New(oldPath, oldVersion, newPath, newVersion string) (*Repo, error) {
	repo := Repo{
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
