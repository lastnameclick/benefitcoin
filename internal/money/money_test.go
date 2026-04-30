package money

import "testing"

func TestParseCoins(t *testing.T) {
	tests := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"0.15", 150, false},
		{".15", 150, false},
		{"1", 1000, false},
		{"1.000", 1000, false},
		{"2.5", 2500, false},
		{"0", 0, false},
		{"0.001", 1, false},
		{"  0.25 ", 250, false},
		{"10.125", 10125, false},
		{"", 0, true},
		{"-1", 0, true},
		{"1.2345", 0, true}, // too many decimals
		{"abc", 0, true},
		{"1.", 0, true},
		{"1.2x", 0, true},
	}
	for _, tt := range tests {
		got, err := ParseCoins(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseCoins(%q) = %d, want error", tt.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseCoins(%q) unexpected error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseCoins(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestFormat(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, "0"},
		{150, "0.15"},
		{1000, "1"},
		{1050, "1.05"},
		{1, "0.001"},
		{10125, "10.125"},
		{-150, "-0.15"},
	}
	for _, tt := range tests {
		if got := Format(tt.in); got != tt.want {
			t.Errorf("Format(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFormatFixed(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, "0.000"},
		{150, "0.150"},
		{1000, "1.000"},
		{10125, "10.125"},
	}
	for _, tt := range tests {
		if got := FormatFixed(tt.in); got != tt.want {
			t.Errorf("FormatFixed(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseFormatRoundTrip(t *testing.T) {
	for _, s := range []string{"0", "0.15", "1", "2.5", "10.125", "0.001"} {
		minor, err := ParseCoins(s)
		if err != nil {
			t.Fatalf("ParseCoins(%q): %v", s, err)
		}
		if got := Format(minor); got != s {
			t.Errorf("round-trip %q -> %d -> %q", s, minor, got)
		}
	}
}
