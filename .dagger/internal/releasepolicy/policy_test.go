package releasepolicy

import "testing"

func TestValidateIdentity(t *testing.T) {
	t.Parallel()

	const commit = "0123456789abcdef0123456789abcdef01234567"

	tests := []struct {
		name    string
		tag     string
		commit  string
		wantErr bool
	}{
		{name: "exact semantic version", tag: "v1.2.3", commit: commit},
		{name: "zero version", tag: "v0.0.0", commit: commit},
		{name: "missing prefix", tag: "1.2.3", commit: commit, wantErr: true},
		{name: "prerelease", tag: "v1.2.3-rc.1", commit: commit, wantErr: true},
		{name: "build metadata", tag: "v1.2.3+build", commit: commit, wantErr: true},
		{name: "short version", tag: "v1.2", commit: commit, wantErr: true},
		{name: "leading zero", tag: "v01.2.3", commit: commit, wantErr: true},
		{name: "empty commit", tag: "v1.2.3", wantErr: true},
		{name: "short commit", tag: "v1.2.3", commit: "0123456", wantErr: true},
		{name: "non hex commit", tag: "v1.2.3", commit: "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateIdentity(tt.tag, tt.commit)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateIdentity(%q, %q) error = %v, wantErr %v", tt.tag, tt.commit, err, tt.wantErr)
			}
		})
	}
}
