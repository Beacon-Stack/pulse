// Package secretfile resolves values from files mounted as Docker secrets.
//
// The *_FILE env var convention is the pattern used by Postgres, MySQL,
// Redis, and other Docker Official Images to source sensitive values from
// files (typically /run/secrets/*) rather than inline env vars — keeping
// them out of process environments and `docker inspect` output.
package secretfile

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// Read returns the contents of the file at path with trailing whitespace
// trimmed. Empty path returns an empty string and no error so callers can
// pass an unset env var through without branching.
func Read(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading secret file %q: %w", path, err)
	}
	return strings.TrimRight(string(b), " \t\r\n"), nil
}

// OverrideDSNPassword returns dsn with its password component replaced by
// the contents of the file at pwFile. When pwFile is empty, dsn is returned
// unchanged. The password is URL-encoded by url.UserPassword, so callers
// may store raw special characters in the secret file.
//
// Returns an error if dsn is not a URL-style DSN or has no user component.
func OverrideDSNPassword(dsn, pwFile string) (string, error) {
	if pwFile == "" {
		return dsn, nil
	}
	pw, err := Read(pwFile)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("parsing dsn: %w", err)
	}
	if u.User == nil {
		return "", fmt.Errorf("dsn has no user component; cannot override password")
	}
	u.User = url.UserPassword(u.User.Username(), pw)
	return u.String(), nil
}
