package manifest

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

func parseMaxSize(value string) (int64, error) {
	multipliers := map[string]int64{
		"KiB": 1 << 10,
		"MiB": 1 << 20,
		"GiB": 1 << 30,
	}
	for suffix, multiplier := range multipliers {
		if !strings.HasSuffix(value, suffix) {
			continue
		}
		raw := strings.TrimSuffix(value, suffix)
		amount, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || amount <= 0 || amount > math.MaxInt64/multiplier {
			return 0, fmt.Errorf("invalid max_size %q", value)
		}
		return amount * multiplier, nil
	}
	return 0, fmt.Errorf("invalid max_size %q", value)
}
