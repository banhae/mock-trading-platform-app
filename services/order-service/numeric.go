package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	QuantityScale       int64 = 100000000
	maxQuantityDecimals       = 8
)

var (
	ErrInvalidPriceFormat    = errors.New("invalid price format")
	ErrInvalidQuantityFormat = errors.New("invalid quantity format")
	ErrNonPositiveValue      = errors.New("value must be positive")
	ErrScaleOverflow         = errors.New("scaled value overflows int64")
	ErrUnderflow             = errors.New("subtraction underflow")
)

// ParsePriceKRWMinor parses a decimal string price into KRW minor-unit int64.
// KRW minor unit is 1 KRW, so decimal fractions are not allowed.
func ParsePriceKRWMinor(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, ErrInvalidPriceFormat
	}
	if strings.Contains(s, ".") {
		return 0, fmt.Errorf("%w: fractional KRW is not allowed", ErrInvalidPriceFormat)
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrInvalidPriceFormat, err)
	}
	if v <= 0 {
		return 0, ErrNonPositiveValue
	}
	return v, nil
}

// FormatPriceKRWMinor formats internal KRW minor-unit int64 to decimal string.
func FormatPriceKRWMinor(v int64) string {
	return strconv.FormatInt(v, 10)
}

// ParseQuantityScaled parses a decimal quantity string into int64 scaled by 1e8.
func ParseQuantityScaled(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, ErrInvalidQuantityFormat
	}

	parts := strings.Split(s, ".")
	if len(parts) > 2 {
		return 0, ErrInvalidQuantityFormat
	}
	intPart := parts[0]
	if intPart == "" {
		return 0, ErrInvalidQuantityFormat
	}
	for _, ch := range intPart {
		if ch < '0' || ch > '9' {
			return 0, ErrInvalidQuantityFormat
		}
	}

	fracPart := ""
	if len(parts) == 2 {
		fracPart = parts[1]
		if fracPart == "" {
			return 0, ErrInvalidQuantityFormat
		}
		if len(fracPart) > maxQuantityDecimals {
			return 0, fmt.Errorf("%w: max %d decimals", ErrInvalidQuantityFormat, maxQuantityDecimals)
		}
		for _, ch := range fracPart {
			if ch < '0' || ch > '9' {
				return 0, ErrInvalidQuantityFormat
			}
		}
	}

	normalized := intPart + fracPart + strings.Repeat("0", maxQuantityDecimals-len(fracPart))
	scaled, err := strconv.ParseInt(normalized, 10, 64)
	if err != nil {
		return 0, ErrScaleOverflow
	}
	if scaled <= 0 {
		return 0, ErrNonPositiveValue
	}
	return scaled, nil
}

// FormatQuantityScaled formats int64 (scaled by 1e8) to decimal string.
func FormatQuantityScaled(v int64) string {
	if v == 0 {
		return "0"
	}
	if v < 0 {
		return "-" + FormatQuantityScaled(-v)
	}

	intPart := v / QuantityScale
	frac := v % QuantityScale
	if frac == 0 {
		return strconv.FormatInt(intPart, 10)
	}

	fracStr := fmt.Sprintf("%08d", frac)
	fracStr = strings.TrimRight(fracStr, "0")
	return strconv.FormatInt(intPart, 10) + "." + fracStr
}

// SafeSubInt64 subtracts b from a and blocks negative results.
func SafeSubInt64(a, b int64) (int64, error) {
	if b < 0 || a < b {
		return 0, ErrUnderflow
	}
	return a - b, nil
}
