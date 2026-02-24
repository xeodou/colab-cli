package main

import (
	"testing"
)

func TestFormatMinutes(t *testing.T) {
	tests := []struct {
		mins float64
		want string
	}{
		{0.5, "<1 min"},
		{0, "<1 min"},
		{1, "1m"},
		{30, "30m"},
		{59, "59m"},
		{60, "1h 0m"},
		{90, "1h 30m"},
		{125, "2h 5m"},
	}

	for _, tt := range tests {
		got := formatMinutes(tt.mins)
		if got != tt.want {
			t.Errorf("formatMinutes(%v) = %q, want %q", tt.mins, got, tt.want)
		}
	}
}

func TestJoin(t *testing.T) {
	tests := []struct {
		input []string
		want  string
	}{
		{[]string{"T4", "L4", "A100"}, "T4, L4, A100"},
		{[]string{"T4"}, "T4"},
		{[]string{}, ""},
		{nil, ""},
	}

	for _, tt := range tests {
		got := join(tt.input)
		if got != tt.want {
			t.Errorf("join(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
