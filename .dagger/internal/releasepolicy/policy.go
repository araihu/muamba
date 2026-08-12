// Package releasepolicy validates immutable release identity inputs before any
// external publication is attempted.
package releasepolicy

import (
	"fmt"
	"regexp"
)

var (
	releaseTagPattern = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	commitPattern     = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// ValidateIdentity requires the exact tag and commit identity supplied by the
// CI provider. The ancestry check still runs against origin/main in Dagger.
func ValidateIdentity(tag, commit string) error {
	if !releaseTagPattern.MatchString(tag) {
		return fmt.Errorf("release tag %q must be an exact vMAJOR.MINOR.PATCH version", tag)
	}
	if !commitPattern.MatchString(commit) {
		return fmt.Errorf("release commit must be a full lowercase Git SHA-1")
	}
	return nil
}
