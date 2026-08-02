package integrity

import (
	"crypto"
	_ "crypto/sha256"
	_ "crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

type Digest struct {
	Algorithm crypto.Hash
	Sum       []byte
}

func Parse(value string) (Digest, error) {
	if strings.ContainsAny(value, " \t\r\n") {
		return Digest{}, fmt.Errorf("integrity must contain exactly one digest")
	}
	separator := ":"
	encoding := "hex"
	if strings.Contains(value, "-") {
		separator, encoding = "-", "base64"
	}
	parts := strings.SplitN(value, separator, 2)
	if len(parts) != 2 || parts[1] == "" {
		return Digest{}, fmt.Errorf("invalid integrity %q", value)
	}
	algorithm, err := parseHash(parts[0])
	if err != nil {
		return Digest{}, err
	}
	var sum []byte
	if encoding == "base64" {
		sum, err = base64.StdEncoding.DecodeString(parts[1])
	} else {
		sum, err = hex.DecodeString(parts[1])
	}
	if err != nil {
		return Digest{}, fmt.Errorf("decode %s integrity: %w", encoding, err)
	}
	if len(sum) != algorithm.Size() {
		return Digest{}, fmt.Errorf("%s digest length = %d, want %d", parts[0], len(sum), algorithm.Size())
	}
	return Digest{Algorithm: algorithm, Sum: sum}, nil
}

func Compute(reader io.Reader, algorithm crypto.Hash) ([]byte, error) {
	if !algorithm.Available() {
		return nil, fmt.Errorf("hash %v is unavailable", algorithm)
	}
	h := algorithm.New()
	if _, err := io.Copy(h, reader); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

func Verify(reader io.Reader, expected Digest) ([]byte, error) {
	actual, err := Compute(reader, expected.Algorithm)
	if err != nil {
		return nil, err
	}
	if subtle.ConstantTimeCompare(actual, expected.Sum) != 1 {
		return actual, fmt.Errorf("integrity mismatch: expected %s, actual %s", FormatSRI(expected.Algorithm, expected.Sum), FormatSRI(expected.Algorithm, actual))
	}
	return actual, nil
}

func FormatSRI(algorithm crypto.Hash, sum []byte) string {
	return hashName(algorithm) + "-" + base64.StdEncoding.EncodeToString(sum)
}

func FormatHash(digest Digest) string {
	return hashName(digest.Algorithm) + ":" + hex.EncodeToString(digest.Sum)
}

func parseHash(name string) (crypto.Hash, error) {
	switch strings.ToLower(name) {
	case "sha256":
		return crypto.SHA256, nil
	case "sha384":
		return crypto.SHA384, nil
	case "sha512":
		return crypto.SHA512, nil
	default:
		return 0, fmt.Errorf("unsupported integrity algorithm %q", name)
	}
}

func hashName(algorithm crypto.Hash) string {
	switch algorithm {
	case crypto.SHA256:
		return "sha256"
	case crypto.SHA384:
		return "sha384"
	case crypto.SHA512:
		return "sha512"
	default:
		return "unknown"
	}
}
