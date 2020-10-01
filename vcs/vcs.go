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

const remoteName = "replace"

type Alias struct {
	NewPrefix string
	OldRepo   string
}

// New creates a new Repo
func New(workDir, oldPath, oldVersion, newPath, newVersion string, repoAliases []Alias) (*Repo, error) {
	repo := Repo{
		workDir:    workDir,
		localPath:  filepath.Join(workDir, oldPath),
		oldPath:    oldPath,
		oldVersion: oldVersion,
		newPath:    newPath,
		newVersion: newVersion,
	}

	for _, alias := range repoAliases {
		if strings.HasPrefix(newPath, alias.NewPrefix) {
			fmt.Fprintf(os.Stderr, "replacing %s for %s with alias %s\n",
				oldPath, newPath, alias.OldRepo)
			oldPath = alias.OldRepo
			break
		}
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

// Repo holds all of the information about one dependency
type Repo struct {
	workDir string

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

func git(verbose bool, directory string, args ...string) error {
	cmdArgs := []string{"--no-pager", "-C", directory}
	cmdArgs = append(cmdArgs, args...)
	if verbose {
		log.Printf("git %s", strings.Join(cmdArgs, " "))
	}
	cmd := exec.Command("git", cmdArgs...)
	if verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	return cmd.Run()
}

func cloneToCache(verbose bool, cachePath string, repoURL string) error {
	_, err := os.Stat(cachePath)
	if err == nil {
		// cache exists
		if verbose {
			log.Printf("have cache for %s", repoURL)
		}
		return nil
	}

	if !os.IsNotExist(err) {
		// real error
		return errors.Wrap(err, "error checking cache")
	}

	cacheParentDir := filepath.Dir(cachePath)
	err = os.MkdirAll(cacheParentDir, 0755)
	if err != nil {
		return errors.Wrap(err, "failed to create cache directory for cache")
	}

	log.Printf("caching %s in %s", repoURL, cachePath)
	err = git(verbose, cacheParentDir, "clone", repoURL)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to clone %s", repoURL))
	}
	return nil
}

// Clone configures the local copy of the repository with the relevant
// remotes
func (r *Repo) Clone(verbose bool) error {
	parentDir := filepath.Dir(r.localPath)

	err := os.MkdirAll(parentDir, 0755)
	if err != nil {
		return errors.Wrap(err, "failed to create output directory for clone")
	}

	oldCachePath := filepath.Join(r.workDir, "_cache", r.oldRepo[8:])
	err = cloneToCache(verbose, oldCachePath, r.oldRepo)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to create cache of %s", r.oldRepo))
	}

	newCachePath := filepath.Join(r.workDir, "_cache", r.newRepo[8:])
	err = cloneToCache(verbose, newCachePath, r.newRepo)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to create cache of %s", r.newRepo))
	}

	if _, err := os.Stat(r.localPath); os.IsNotExist(err) {
		log.Printf("%s: cloning %s", r.oldPath, r.oldRepo)
		err := git(verbose, parentDir, "clone", oldCachePath, filepath.Base(r.localPath))
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("failed to clone %s", r.oldRepo))
		}
	} else {
		if verbose {
			log.Printf("%s: found %s", r.oldPath, r.localPath)
		}
	}

	err = r.git(false, "remote", "get-url", remoteName)
	if err != nil {
		log.Printf("%s: adding fork remote for %s", r.oldPath, r.newRepo)
		err = r.git(verbose, "remote", "add", remoteName, newCachePath)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("could not add remote %s", r.newRepo))
		}

		err = r.git(verbose, "fetch", "--all", "--tags")
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("could not update remote %s", r.newRepo))
		}
	} else {
		if verbose {
			log.Printf("%s: remote: %s", r.oldPath, r.newRepo)
		}
	}

	return nil
}

func refFromVersion(version string) string {
	if version == "" {
		return "origin/master"
	}
	parts := strings.Split(version, "-")
	if len(parts) >= 3 {
		return parts[len(parts)-1]
	}
	return version
}

func (r *Repo) gitRange() string {
	return fmt.Sprintf("%s..%s", refFromVersion(r.oldVersion), refFromVersion(r.newVersion))
}

func (r *Repo) commonAncestor() bool {
	err := r.git(false, "merge-base", "origin/master", fmt.Sprintf("%s/master", remoteName))
	if err != nil {
		return false
	}
	return true
}

// Log shows the simple log output between the two versions
func (r *Repo) Log() error {
	if !r.commonAncestor() {
		fmt.Printf("No common ancestor, not logging.\n")
		return nil
	}
	return r.git(true, "log", "--oneline", r.gitRange())
}

// DiffStat shows the diff statistics between the two versions
func (r *Repo) DiffStat() error {
	if !r.commonAncestor() {
		fmt.Printf("No common ancestor, not diffing.\n")
		return nil
	}
	return r.git(true, "diff", "--stat=120", r.gitRange())
}

func (r *Repo) git(verbose bool, args ...string) error {
	return git(verbose, r.localPath, args...)
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
