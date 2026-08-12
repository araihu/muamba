package cachepolicy

import "testing"

func TestValidate(t *testing.T) {
	t.Parallel()

	for _, domain := range []string{"fork", "internal", "main", "release"} {
		domain := domain
		t.Run(domain, func(t *testing.T) {
			t.Parallel()
			if err := Validate(domain, "12345-2"); err != nil {
				t.Fatalf("Validate(%q) error = %v", domain, err)
			}
		})
	}

	for _, tt := range []struct {
		name   string
		domain string
		nonce  string
	}{
		{name: "empty domain", nonce: "1-1"},
		{name: "unknown domain", domain: "pull-request", nonce: "1-1"},
		{name: "empty nonce", domain: "internal"},
		{name: "space in nonce", domain: "internal", nonce: "1 1"},
		{name: "slash in nonce", domain: "internal", nonce: "1/1"},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := Validate(tt.domain, tt.nonce); err == nil {
				t.Fatalf("Validate(%q, %q) unexpectedly passed", tt.domain, tt.nonce)
			}
		})
	}
}

func TestValidateRelease(t *testing.T) {
	t.Parallel()

	if err := ValidateRelease("release", "1-1"); err != nil {
		t.Fatalf("ValidateRelease(release) error = %v", err)
	}
	for _, domain := range []string{"fork", "internal", "main"} {
		if err := ValidateRelease(domain, "1-1"); err == nil {
			t.Errorf("ValidateRelease(%q) unexpectedly passed", domain)
		}
	}
}

func TestVolume(t *testing.T) {
	t.Parallel()

	if got, want := Volume("fork", "go-mod"), "muamba-pr-go-mod-v2"; got != want {
		t.Fatalf("Volume() = %q, want %q", got, want)
	}
	if got, want := Volume("internal", "go-build"), "muamba-pr-go-build-v2"; got != want {
		t.Fatalf("Volume() = %q, want %q", got, want)
	}
	if got, want := Volume("release", "go-build"), "muamba-release-go-build-v2"; got != want {
		t.Fatalf("Volume() = %q, want %q", got, want)
	}
}

func TestPartition(t *testing.T) {
	t.Parallel()

	for _, domain := range []string{"fork", "internal"} {
		if got := Partition(domain); got != "pr" {
			t.Errorf("Partition(%q) = %q, want pr", domain, got)
		}
	}
	for _, domain := range []string{"main", "release"} {
		if got := Partition(domain); got != domain {
			t.Errorf("Partition(%q) = %q, want %q", domain, got, domain)
		}
	}
}
