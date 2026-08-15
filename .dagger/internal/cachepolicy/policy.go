// Package cachepolicy isolates mutable CI inputs and validates freshness keys.
package cachepolicy

import (
	"fmt"
	"regexp"
)

var runNoncePattern = regexp.MustCompile(`^[1-9][0-9]*-[1-9][0-9]*$`)

var trustDomains = map[string]struct{}{
	"fork":     {},
	"internal": {},
	"main":     {},
	"release":  {},
}

// Validate requires an adapter-derived trust domain and per-attempt nonce.
func Validate(trustDomain, runNonce string) error {
	if err := ValidateTrustDomain(trustDomain); err != nil {
		return err
	}
	if !runNoncePattern.MatchString(runNonce) {
		return fmt.Errorf("run nonce must be github.run_id-github.run_attempt")
	}
	return nil
}

// ValidateTrustDomain accepts only adapter-owned isolation domains.
func ValidateTrustDomain(trustDomain string) error {
	if _, ok := trustDomains[trustDomain]; !ok {
		return fmt.Errorf("trust domain %q must be one of fork, internal, main, release", trustDomain)
	}
	return nil
}

// ValidateRelease prevents release jobs from consuming mutable PR cache state.
func ValidateRelease(trustDomain, runNonce string) error {
	if err := Validate(trustDomain, runNonce); err != nil {
		return err
	}
	if trustDomain != "release" {
		return fmt.Errorf("release functions require trust domain release")
	}
	return nil
}

// Partition maps every pull-request-controlled domain to the host-isolated PR
// Engine namespace. Protected domains keep distinct mutable cache state.
func Partition(trustDomain string) string {
	if trustDomain == "fork" || trustDomain == "internal" {
		return "pr"
	}
	return trustDomain
}

// Volume returns a cache name partitioned by the host-owned Engine boundary.
func Volume(trustDomain, purpose string) string {
	return fmt.Sprintf("muamba-%s-%s-v2", Partition(trustDomain), purpose)
}
