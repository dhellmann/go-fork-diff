package discovery

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	urlpkg "net/url"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// based on https://github.com/golang/go/blob/master/src/cmd/go/internal/vcs/vcs.go

// urlForImportPath returns a partially-populated URL for the given Go import path.
//
// The URL leaves the Scheme field blank so that web.Get will try any scheme
// allowed by the selected security mode.
func urlForImportPath(importPath string) (*urlpkg.URL, error) {
	slash := strings.Index(importPath, "/")
	if slash < 0 {
		slash = len(importPath)
	}
	host, path := importPath[:slash], importPath[slash:]
	if !strings.Contains(host, ".") {
		return nil, errors.New("import path does not begin with hostname")
	}
	if len(path) == 0 {
		path = "/"
	}
	return &urlpkg.URL{Scheme: "https", Host: host, Path: path, RawQuery: "go-get=1"}, nil
}

// RepoRootForImportDynamic finds a repository root for a custom domain
// This handles custom import paths like "name.tld/pkg/foo" or just "name.tld".
func RepoRootForImportDynamic(importPath string) (string, error) {
	url, err := urlForImportPath(importPath)
	if err != nil {
		return "", err
	}

	client := http.Client{
		Timeout: time.Second * 20,
	}
	req, err := http.NewRequest(http.MethodGet, url.String(), nil)
	if err != nil {
		return "", errors.Wrap(err, "unable to build request")
	}
	req.Header.Set("User-Agent", "go-fork-diff")
	resp, err := client.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "unable to fetch request")
	}

	body := resp.Body
	defer body.Close()
	imports, err := parseMetaGoImports(body)
	if err != nil {
		return "", errors.Wrap(err, "could not get meta tag for import instructions")
	}
	if len(imports) == 0 {
		return "", errors.New("no import instructions found for import path")
	}
	// Find the matched meta import.
	mmi, err := matchGoImport(imports, importPath)
	if err != nil {
		if _, ok := err.(ImportMismatchError); !ok {
			return "", fmt.Errorf("parse %s: %v", url, err)
		}
		return "", fmt.Errorf("parse %s: no go-import meta tags (%s)", url, err)
	}
	// If the import was "uni.edu/bob/project", which said the
	// prefix was "uni.edu" and the RepoRoot was "evilroot.com",
	// make sure we don't trust Bob and check out evilroot.com to
	// "uni.edu" yet (possibly overwriting/preempting another
	// non-evil student). Instead, first verify the root and see
	// if it matches Bob's claim.
	if mmi.Prefix != importPath {
		var imports []metaImport
		url2, imports, err := metaImportsForPrefix(mmi.Prefix)
		if err != nil {
			return "", err
		}
		metaImport2, err := matchGoImport(imports, importPath)
		if err != nil || mmi != metaImport2 {
			return "", fmt.Errorf("%s and %s disagree about go-import for %s", url, url2,
				mmi.Prefix)
		}
	}

	if err := validateRepoRoot(mmi.RepoRoot); err != nil {
		return "", fmt.Errorf("%s: invalid repo root %q: %v", url, mmi.RepoRoot, err)
	}

	return mmi.RepoRoot, nil
}

// validateRepoRoot returns an error if repoRoot does not seem to be
// a valid URL with scheme.
func validateRepoRoot(repoRoot string) error {
	url, err := urlpkg.Parse(repoRoot)
	if err != nil {
		return err
	}
	if url.Scheme == "" {
		return errors.New("no scheme")
	}
	if url.Scheme == "file" {
		return errors.New("file scheme disallowed")
	}
	return nil
}

type fetchResult struct {
	url     *urlpkg.URL
	imports []metaImport
	err     error
}

// metaImportsForPrefix takes a package's root import path as declared in a <meta> tag
// and returns its HTML discovery URL and the parsed metaImport lines
// found on the page.
//
// The importPath is of the form "golang.org/x/tools".
// It is an error if no imports are found.
// url will still be valid if err != nil.
// The returned url will be of the form "https://golang.org/x/tools?go-get=1"
func metaImportsForPrefix(importPrefix string) (*urlpkg.URL, []metaImport, error) {
	url, err := urlForImportPath(importPrefix)
	if err != nil {
		return nil, nil, err
	}
	client := http.Client{
		Timeout: time.Second * 20,
	}
	req, err := http.NewRequest(http.MethodGet, url.String(), nil)
	if err != nil {
		return nil, nil, errors.Wrap(err, "unable to build request")
	}
	req.Header.Set("User-Agent", "go-fork-diff")
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, errors.Wrap(err, "unable to fetch request")
	}
	body := resp.Body
	defer body.Close()
	imports, err := parseMetaGoImports(body)
	if len(imports) == 0 {
		return nil, nil, errors.Wrap(err, "found no import instructions")
	}
	if err != nil {
		return url, nil, err
	}
	return url, imports, err
}

// A ImportMismatchError is returned where metaImport/s are present
// but none match our import path.
type ImportMismatchError struct {
	importPath string
	mismatches []string // the meta imports that were discarded for not matching our importPath
}

func (m ImportMismatchError) Error() string {
	formattedStrings := make([]string, len(m.mismatches))
	for i, pre := range m.mismatches {
		formattedStrings[i] = fmt.Sprintf("meta tag %s did not match import path %s", pre, m.importPath)
	}
	return strings.Join(formattedStrings, ", ")
}

// pathPrefix reports whether sub is a prefix of s,
// only considering entire path components.
func pathPrefix(s, sub string) bool {
	// strings.HasPrefix is necessary but not sufficient.
	if !strings.HasPrefix(s, sub) {
		return false
	}
	// The remainder after the prefix must either be empty or start with a slash.
	rem := s[len(sub):]
	return rem == "" || rem[0] == '/'
}

// matchGoImport returns the metaImport from imports matching importPath.
// An error is returned if there are multiple matches.
// An ImportMismatchError is returned if none match.
func matchGoImport(imports []metaImport, importPath string) (metaImport, error) {
	match := -1

	errImportMismatch := ImportMismatchError{importPath: importPath}
	for i, im := range imports {
		if !pathPrefix(importPath, im.Prefix) {
			errImportMismatch.mismatches = append(errImportMismatch.mismatches, im.Prefix)
			continue
		}

		if match >= 0 {
			if imports[match].VCS == "mod" && im.VCS != "mod" {
				// All the mod entries precede all the non-mod entries.
				// We have a mod entry and don't care about the rest,
				// matching or not.
				break
			}
			return metaImport{}, fmt.Errorf("multiple meta tags match import path %q", importPath)
		}
		match = i
	}

	if match == -1 {
		return metaImport{}, errImportMismatch
	}
	return imports[match], nil
}

// metaImport represents the parsed <meta name="go-import"
// content="prefix vcs reporoot" /> tags from HTML files.
type metaImport struct {
	Prefix, VCS, RepoRoot string
}

// from https://github.com/golang/go/blob/master/src/cmd/go/internal/vcs/discovery.go

// charsetReader returns a reader that converts from the given charset to UTF-8.
// Currently it only supports UTF-8 and ASCII. Otherwise, it returns a meaningful
// error which is printed by go get, so the user can find why the package
// wasn't downloaded if the encoding is not supported. Note that, in
// order to reduce potential errors, ASCII is treated as UTF-8 (i.e. characters
// greater than 0x7f are not rejected).
func charsetReader(charset string, input io.Reader) (io.Reader, error) {
	switch strings.ToLower(charset) {
	case "utf-8", "ascii":
		return input, nil
	default:
		return nil, fmt.Errorf("can't decode XML document using charset %q", charset)
	}
}

// parseMetaGoImports returns meta imports from the HTML in r.
// Parsing ends at the end of the <head> section or the beginning of the <body>.
func parseMetaGoImports(r io.Reader) ([]metaImport, error) {
	d := xml.NewDecoder(r)
	d.CharsetReader = charsetReader
	d.Strict = false
	var imports []metaImport
	for {
		t, err := d.RawToken()
		if err != nil {
			if err != io.EOF && len(imports) == 0 {
				return nil, err
			}
			break
		}
		if e, ok := t.(xml.StartElement); ok && strings.EqualFold(e.Name.Local, "body") {
			break
		}
		if e, ok := t.(xml.EndElement); ok && strings.EqualFold(e.Name.Local, "head") {
			break
		}
		e, ok := t.(xml.StartElement)
		if !ok || !strings.EqualFold(e.Name.Local, "meta") {
			continue
		}
		if attrValue(e.Attr, "name") != "go-import" {
			continue
		}
		if f := strings.Fields(attrValue(e.Attr, "content")); len(f) == 3 {
			imports = append(imports, metaImport{
				Prefix:   f[0],
				VCS:      f[1],
				RepoRoot: f[2],
			})
		}
	}

	var list []metaImport
	var have map[string]bool

	// // Extract mod entries if we are paying attention to them.
	// if mod == PreferMod {
	// 	have = make(map[string]bool)
	// 	for _, m := range imports {
	// 		if m.VCS == "mod" {
	// 			have[m.Prefix] = true
	// 			list = append(list, m)
	// 		}
	// 	}
	// }

	// Append non-mod entries, ignoring those superseded by a mod entry.
	for _, m := range imports {
		if m.VCS != "mod" && !have[m.Prefix] {
			list = append(list, m)
		}
	}
	return list, nil
}

// attrValue returns the attribute value for the case-insensitive key
// `name', or the empty string if nothing is found.
func attrValue(attrs []xml.Attr, name string) string {
	for _, a := range attrs {
		if strings.EqualFold(a.Name.Local, name) {
			return a.Value
		}
	}
	return ""
}
