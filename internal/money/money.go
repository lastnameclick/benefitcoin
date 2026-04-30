// Package money handles BenefitCoin amounts as integer minor units
// (currency BNC, 1 coin = 1000 units), matching TigerBeetle's integer ledger.
package money

import (
	"errors"
	"fmt"
	"strings"
)

const (
	// Currency is the ISO-style code for BenefitCoins.
	Currency = "BNC"
	// Scale is the number of decimal places (minor-unit exponent).
	Scale = 3
	// MinorPerCoin is the number of minor units in one whole coin.
	MinorPerCoin int64 = 1000
)

// ErrInvalidAmount is returned when a coin string cannot be parsed.
var ErrInvalidAmount = errors.New("invalid coin amount")

// ParseCoins converts a human coin string (e.g. "0.15", "1", "2.5") into minor
// units (150, 1000, 2500). It rejects negatives, blanks, and more than Scale
// decimal places.
func ParseCoins(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("%w: empty", ErrInvalidAmount)
	}
	if strings.HasPrefix(s, "-") {
		return 0, fmt.Errorf("%w: must not be negative", ErrInvalidAmount)
	}

	whole, frac, hasFrac := strings.Cut(s, ".")
	if whole == "" {
		whole = "0"
	}

	var wholeVal int64
	if _, err := fmt.Sscanf(whole, "%d", &wholeVal); err != nil || !isDigits(whole) {
		return 0, fmt.Errorf("%w: %q", ErrInvalidAmount, s)
	}

	var fracVal int64
	if hasFrac {
		if frac == "" || !isDigits(frac) {
			return 0, fmt.Errorf("%w: %q", ErrInvalidAmount, s)
		}
		if len(frac) > Scale {
			return 0, fmt.Errorf("%w: more than %d decimal places", ErrInvalidAmount, Scale)
		}
		// Right-pad to Scale digits: "15" -> "150".
		frac += strings.Repeat("0", Scale-len(frac))
		if _, err := fmt.Sscanf(frac, "%d", &fracVal); err != nil {
			return 0, fmt.Errorf("%w: %q", ErrInvalidAmount, s)
		}
	}

	return wholeVal*MinorPerCoin + fracVal, nil
}

// Format renders minor units as a trimmed coin string: 150 -> "0.15",
// 1000 -> "1", 1050 -> "1.05", 0 -> "0".
func Format(minor int64) string {
	neg := minor < 0
	if neg {
		minor = -minor
	}
	whole := minor / MinorPerCoin
	frac := minor % MinorPerCoin

	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	fmt.Fprintf(&b, "%d", whole)
	if frac != 0 {
		f := strings.TrimRight(fmt.Sprintf("%03d", frac), "0")
		b.WriteByte('.')
		b.WriteString(f)
	}
	return b.String()
}

// FormatFixed renders minor units with exactly Scale decimal places:
// 150 -> "0.150", 1000 -> "1.000".
func FormatFixed(minor int64) string {
	neg := minor < 0
	if neg {
		minor = -minor
	}
	sign := ""
	if neg {
		sign = "-"
	}
	return fmt.Sprintf("%s%d.%03d", sign, minor/MinorPerCoin, minor%MinorPerCoin)
}

// Coin returns n whole coins in minor units.
func Coin(n int64) int64 { return n * MinorPerCoin }

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
