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
			oldPath = alias.OldRepo
			repo.aliased, _ = resolveOne(repo.oldPath)
			if repo.aliased == "" {
				repo.aliased = repo.oldPath
			}
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

	// aliased holds the oldPath value that was replaced by the alias
	aliased string
}

func (r *Repo) String() string {
	s := fmt.Sprintf("%s @ %s (%s)\n  replace: %s @ %s (%s)\n  locally: %s",
		r.oldPath, r.oldVersion, r.oldRepo,
		r.newPath, r.newVersion, r.newRepo,
		r.localPath,
	)
	if r.aliased != "" {
		s = fmt.Sprintf("%s\n  aliased: %s", s, r.aliased)
	}
	return s
}

func git(verbose bool, directory string, args ...string) error {
	cmdArgs := []string{"--no-pager", "-C", directory}
	cmdArgs = append(cmdArgs, args...)
	if verbose {
		printableArgs := []string{}
		for _, a := range cmdArgs {
			if strings.Contains(a, " ") {
				a = fmt.Sprintf("\"%s\"", a)
			}
			printableArgs = append(printableArgs, a)
		}
		log.Printf("git %s\n\n", strings.Join(printableArgs, " "))
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
	if version == "" || version == "v0.0.0" {
		return ""
	}

	result := version

	// if the version look like
	// v11.0.1-0.20190409021438-1a26190bd76a+incompatible start with
	// the 3rd part
	parts := strings.Split(result, "-")
	if len(parts) >= 3 {
		result = parts[len(parts)-1]
	}

	// if the version looks like 1a26190bd76a+incompatible take the
	// first part
	parts = strings.Split(result, "+")
	if len(parts) > 1 {
		result = parts[0]
	}

	// if the version is now all zeros, return empty string
	if strings.Trim(result, "0") == "" {
		result = ""
	}

	return result
}

func (r *Repo) gitRefs() (string, string) {
	oldRef := refFromVersion(r.oldVersion)
	if oldRef == "" {
		oldRef = "origin/master"
	}
	newRef := refFromVersion(r.newVersion)
	if newRef == "" {
		newRef = "remotes/replace/master"
	}
	return oldRef, newRef
}

func (r *Repo) gitRange() string {
	oldRef, newRef := r.gitRefs()
	result := fmt.Sprintf("%s..%s", oldRef, newRef)
	return result
}

func (r *Repo) commonAncestor() bool {
	oldRef, newRef := r.gitRefs()
	err := r.git(false, "merge-base", oldRef, newRef)
	if err != nil {
		return false
	}
	return true
}

func (r *Repo) path() string {
	parts := strings.SplitN(r.newPath, "/", 4)
	if len(parts) > 3 {
		return parts[3]
	}
	return ""
}

// Log shows the simple log output between the two versions
func (r *Repo) Log() error {

	startEnd := r.gitRange()

	if !r.commonAncestor() {
		fmt.Printf("No common ancestor, not logging %s.\n", startEnd)
		return nil
	}

	args := []string{
		"log",
		"--pretty=format:%h %cd %s",
		"--date=iso",
		"--decorate",
		startEnd,
	}
	path := r.path()
	if path != "" {
		args = append(args, "--", path)
	}

	return r.git(true, args...)
}

// DiffStat shows the diff statistics between the two versions
func (r *Repo) DiffStat() error {

	startEnd := r.gitRange()

	if !r.commonAncestor() {
		fmt.Printf("No common ancestor, not diffing %s.\n", startEnd)
		return nil
	}

	args := []string{"diff", "--stat=120", r.gitRange(), "--"}
	path := r.path()
	if path != "" {
		args = append(args, path)
	} else {
		args = append(args, ".", ":!vendor")
	}

	return r.git(true, args...)
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
