// Package secretfile resolves values from files mounted as Docker secrets.
//
// The *_FILE env var convention is the pattern used by Docker Official Images
// to source sensitive values from files (typically /run/secrets/*) rather than
// inline env vars — keeping them out of process environments and
// `docker inspect` output.
package secretfile

import (
	"fmt"
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
