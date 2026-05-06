package main

import "testing"

func TestParsePriceKRWMinorValid(t *testing.T) {
	got, err := ParsePriceKRWMinor("50000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 50000 {
		t.Fatalf("expected 50000, got %d", got)
	}
}

func TestParsePriceKRWMinorInvalid(t *testing.T) {
	cases := []string{"", "0", "-1", "12.3", "abc", "1e3"}
	for _, tc := range cases {
		t.Run(tc, func(t *testing.T) {
			if _, err := ParsePriceKRWMinor(tc); err == nil {
				t.Fatalf("expected error for %q", tc)
			}
		})
	}
}

func TestParseQuantityScaledValid0to8Decimals(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"1", 100000000},
		{"1.2", 120000000},
		{"1.23", 123000000},
		{"1.234", 123400000},
		{"1.2345", 123450000},
		{"1.23456", 123456000},
		{"1.234567", 123456700},
		{"1.2345678", 123456780},
		{"1.23456789", 123456789},
		{"0.00000001", 1},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ParseQuantityScaled(tc.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("want %d, got %d", tc.want, got)
			}
		})
	}
}

func TestParseQuantityScaledInvalid(t *testing.T) {
	cases := []string{
		"", "0", "-1", "1.234567890", "abc", "1..2", ".5", "5.", "1e-8", "-0.1",
	}
	for _, tc := range cases {
		t.Run(tc, func(t *testing.T) {
			if _, err := ParseQuantityScaled(tc); err == nil {
				t.Fatalf("expected error for %q", tc)
			}
		})
	}
}

func TestRoundTripParseFormat(t *testing.T) {
	priceIn := "75001"
	priceScaled, err := ParsePriceKRWMinor(priceIn)
	if err != nil {
		t.Fatalf("parse price: %v", err)
	}
	if got := FormatPriceKRWMinor(priceScaled); got != priceIn {
		t.Fatalf("price round-trip: want %q, got %q", priceIn, got)
	}

	qtyIn := "1.23000001"
	qtyScaled, err := ParseQuantityScaled(qtyIn)
	if err != nil {
		t.Fatalf("parse quantity: %v", err)
	}
	if got := FormatQuantityScaled(qtyScaled); got != qtyIn {
		t.Fatalf("quantity round-trip: want %q, got %q", qtyIn, got)
	}
}

func TestFormatQuantityScaledTrimsTrailingZeros(t *testing.T) {
	if got := FormatQuantityScaled(123000000); got != "1.23" {
		t.Fatalf("want 1.23, got %s", got)
	}
	if got := FormatQuantityScaled(100000000); got != "1" {
		t.Fatalf("want 1, got %s", got)
	}
}

func TestSafeSubInt64(t *testing.T) {
	got, err := SafeSubInt64(10, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 7 {
		t.Fatalf("want 7, got %d", got)
	}

	if _, err := SafeSubInt64(3, 10); err == nil {
		t.Fatalf("expected underflow error")
	}
}
