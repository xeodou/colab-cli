package main

import (
	"testing"
)

func TestStripXSSI(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  string
	}{
		{
			name:  "with XSSI prefix",
			input: []byte(")]}'\n{\"token\":\"abc\"}"),
			want:  `{"token":"abc"}`,
		},
		{
			name:  "without prefix",
			input: []byte(`{"token":"abc"}`),
			want:  `{"token":"abc"}`,
		},
		{
			name:  "empty",
			input: []byte{},
			want:  "",
		},
		{
			name:  "only prefix",
			input: []byte(")]}'\n"),
			want:  ")]}'\n", // len(data) == len(prefix), not >
		},
		{
			name:  "prefix plus one char",
			input: []byte(")]}'\n{"),
			want:  "{",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(stripXSSI(tt.input))
			if got != tt.want {
				t.Errorf("stripXSSI() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUuidToNbHash(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{
			// UUID is 36 chars, underscored version is 36 chars, needs 8 dots
			input: "550e8400-e29b-41d4-a716-446655440000",
			want:  "550e8400_e29b_41d4_a716_446655440000........",
		},
		{
			// 7 chars after replace, needs 37 dots
			input: "a-b-c-d",
			want:  "a_b_c_d" + ".....................................",
		},
	}

	for _, tt := range tests {
		got := uuidToNbHash(tt.input)
		if got != tt.want {
			t.Errorf("uuidToNbHash(%q) = %q, want %q", tt.input, got, tt.want)
		}
		if len(got) != 44 {
			t.Errorf("uuidToNbHash(%q) length = %d, want 44", tt.input, len(got))
		}
	}
}

func TestOutcomeError(t *testing.T) {
	tests := []struct {
		code    int
		wantErr bool
	}{
		{0, false},  // undefined (ok)
		{4, false},  // success
		{1, true},   // quota denied
		{2, true},   // quota exceeded
		{5, true},   // denylisted
		{99, false}, // unknown code
	}

	for _, tt := range tests {
		err := outcomeError(tt.code)
		if (err != nil) != tt.wantErr {
			t.Errorf("outcomeError(%d) error = %v, wantErr %v", tt.code, err, tt.wantErr)
		}
	}
}
