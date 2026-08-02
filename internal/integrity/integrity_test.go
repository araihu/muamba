package integrity

import (
	"crypto"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
)

func TestParseAndVerifySupportedFormats(t *testing.T) {
	contents := []byte("abc")
	for _, algorithm := range []crypto.Hash{crypto.SHA256, crypto.SHA384, crypto.SHA512} {
		h := algorithm.New()
		_, _ = h.Write(contents)
		sum := h.Sum(nil)
		name := hashName(algorithm)
		values := []string{
			name + "-" + base64.StdEncoding.EncodeToString(sum),
			name + ":" + hex.EncodeToString(sum),
			name + ":" + strings.ToUpper(hex.EncodeToString(sum)),
		}
		for _, value := range values {
			t.Run(value[:6], func(t *testing.T) {
				digest, err := Parse(value)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := Verify(strings.NewReader("abc"), digest); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestParseRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"deadbeef", "md5:abc", "sha384-!!!", "sha256:abcd", "sha384-abc sha512-def"} {
		if _, err := Parse(value); err == nil {
			t.Errorf("Parse(%q) succeeded", value)
		}
	}
}

func TestDefaultFormatIsSHA384SRI(t *testing.T) {
	sum, err := Compute(strings.NewReader("abc"), crypto.SHA384)
	if err != nil {
		t.Fatal(err)
	}
	got := FormatSRI(crypto.SHA384, sum)
	want := "sha384-ywB1P0WjXou1oD1pmsZQBycsMqsO3tFjGotgWkP/W+2AhgcroefMI1i67KE0yCWn"
	if got != want {
		t.Fatalf("FormatSRI = %q, want %q", got, want)
	}
}

func TestVerifyReportsMismatch(t *testing.T) {
	digest, err := Parse("sha256-ungWv48Bz+pBQUDeXa4iI7ADYaOWF3qctBD/YfIAFa0=")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(strings.NewReader("different"), digest); err == nil || !strings.Contains(err.Error(), "integrity mismatch") {
		t.Fatalf("Verify error = %v", err)
	}
}
