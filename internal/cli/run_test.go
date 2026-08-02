package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(help) code = %d, stderr = %q", code, stderr.String())
	}
	for _, command := range []string{"lock", "sync", "verify", "update", "generate-go"} {
		if !strings.Contains(stdout.String(), command) {
			t.Errorf("help missing %q", command)
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestNoArgsShowsHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Run(nil, &stdout, &stderr); code != 0 {
		t.Fatalf("Run(nil) code = %d", code)
	}
	if !strings.Contains(stdout.String(), "Usage: muamba") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestStubCommandsFail(t *testing.T) {
	for _, command := range []string{"lock", "sync", "verify", "update", "generate-go"} {
		t.Run(command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := Run([]string{command}, &stdout, &stderr); code != 1 {
				t.Fatalf("Run(%s) code = %d, stderr = %q", command, code, stderr.String())
			}
			if !strings.Contains(stderr.String(), command+": not implemented") {
				t.Fatalf("stderr = %q", stderr.String())
			}
		})
	}
}

func TestUnknownCommandIsUsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"mystery"}, &stdout, &stderr); code != 2 {
		t.Fatalf("Run(mystery) code = %d", code)
	}
	if !strings.Contains(stderr.String(), "unknown command: mystery") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}
