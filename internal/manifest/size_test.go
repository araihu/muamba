package manifest

import "testing"

func TestParseMaxSize(t *testing.T) {
	for raw, want := range map[string]int64{
		"1KiB":   1 << 10,
		"100MiB": 100 << 20,
		"2GiB":   2 << 30,
	} {
		t.Run(raw, func(t *testing.T) {
			got, err := parseMaxSize(raw)
			if err != nil || got != want {
				t.Fatalf("parseMaxSize(%q) = %d, %v; want %d", raw, got, err, want)
			}
		})
	}
}

func TestParseMaxSizeRejectsInvalidValues(t *testing.T) {
	for _, raw := range []string{"", "0MiB", "-1MiB", "1MB", "1.5MiB", "1TiB", "9223372036854775807GiB"} {
		t.Run(raw, func(t *testing.T) {
			if _, err := parseMaxSize(raw); err == nil {
				t.Fatalf("parseMaxSize(%q) succeeded", raw)
			}
		})
	}
}
