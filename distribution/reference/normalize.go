package reference

import (
	"errors"
	"fmt"
	"strings"
)

var (
	legacyDefaultDomain = "index.docker.io"
	defaultDomain       = "docker.io"
	officialRepoName    = "library"
)

// normalizedNamed represents a name which has been
// normalized and has a familiar form. A familiar name
// is what is used in Docker UI. An example normalized
// name is "docker.io/library/ubuntu" and corresponding
// familiar name of "ubuntu".
type normalizedNamed interface {
	Named
	Familiar() Named
}

// ParseNormalizedNamed parses a string into a named reference
// transforming a familiar name from Docker UI to a fully
// qualified reference. If the value may be an identifier
// use ParseAnyReference.
func ParseNormalizedNamed(s string) (Named, error) {
	if ok := anchoredIdentifierRegexp.MatchString(s); ok {
		return nil, fmt.Errorf("invalid repository name (%s), cannot specify 64-byte hexadecimal strings", s)
	}
	domain, remainder := splitDockerDomain(s)
	var remoteName string
	if tagSep := strings.IndexRune(remainder, ':'); tagSep > -1 {
		remoteName = remainder[:tagSep]
	} else {
		remoteName = remainder
	}
	if strings.ToLower(remoteName) != remoteName {
		return nil, errors.New("invalid reference format: repository name must be lowercase")
	}

	ref, err := Parse(domain + "/" + remainder)
	if err != nil {
		return nil, err
	}
	named, isNamed := ref.(Named)
	if !isNamed {
		return nil, fmt.Errorf("reference %s has no name", ref.String())
	}
	return named, nil
}

// splitDockerDomain splits a repository name to domain and remotename string.
// If no valid domain is found, the default domain is used. Repository name
// needs to be already validated before.
func splitDockerDomain(name string) (domain, remainder string) {
	i := strings.IndexRune(name, '/')
	if i == -1 || (!strings.ContainsAny(name[:i], ".:") && name[:i] != "localhost") {
		domain, remainder = defaultDomain, name
	} else {
		domain, remainder = name[:i], name[i+1:]
	}
	if domain == legacyDefaultDomain {
		domain = defaultDomain
	}
	if domain == defaultDomain && !strings.ContainsRune(remainder, '/') {
		remainder = officialRepoName + "/" + remainder
	}
	return
}

// familiarizeName returns a shortened version of the name familiar
// to to the Docker UI. Familiar names have the default domain
// "docker.io" and "library/" repository prefix removed.
// For example, "docker.io/library/redis" will have the familiar
// name "redis" and "docker.io/dmcgowan/myapp" will be "dmcgowan/myapp".
// Returns a familiarized named only reference.
func familiarizeName(named namedRepository) repository {
	repo := repository{
		domain: named.Domain(),
		path:   named.Path(),
	}

	if repo.domain == defaultDomain {
		repo.domain = ""
		// Handle official repositories which have the pattern "library/<official repo name>"
		if split := strings.Split(repo.path, "/"); len(split) == 2 && split[0] == officialRepoName {
			repo.path = split[1]
		}
	}
	return repo
}

func (r reference) Familiar() Named {
	return reference{
		namedRepository: familiarizeName(r.namedRepository),
		tag:             r.tag,
		digest:          r.digest,
	}
}

func (r repository) Familiar() Named {
	return familiarizeName(r)
}

func (t taggedReference) Familiar() Named {
	return taggedReference{
		namedRepository: familiarizeName(t.namedRepository),
		tag:             t.tag,
	}
}

func (c canonicalReference) Familiar() Named {
	return canonicalReference{
		namedRepository: familiarizeName(c.namedRepository),
		digest:          c.digest,
	}
}