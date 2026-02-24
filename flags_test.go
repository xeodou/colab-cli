package main

import (
	"testing"
)

func TestHasFlag(t *testing.T) {
	tests := []struct {
		args []string
		flag string
		want bool
	}{
		{[]string{"--json", "file.py"}, "--json", true},
		{[]string{"file.py"}, "--json", false},
		{[]string{}, "--json", false},
		{[]string{"--json"}, "--json", true},
		{[]string{"--gpu", "t4", "--json"}, "--json", true},
	}

	for _, tt := range tests {
		got := hasFlag(tt.args, tt.flag)
		if got != tt.want {
			t.Errorf("hasFlag(%v, %q) = %v, want %v", tt.args, tt.flag, got, tt.want)
		}
	}
}

func TestGetFlagValue(t *testing.T) {
	tests := []struct {
		args       []string
		flag       string
		defaultVal string
		want       string
	}{
		{[]string{"--gpu", "a100"}, "--gpu", "t4", "a100"},
		{[]string{"file.py"}, "--gpu", "t4", "t4"},
		{[]string{"--gpu"}, "--gpu", "t4", "t4"}, // flag without value
		{[]string{}, "--gpu", "t4", "t4"},
		{[]string{"--timeout", "1h", "--gpu", "l4"}, "--gpu", "t4", "l4"},
	}

	for _, tt := range tests {
		got := getFlagValue(tt.args, tt.flag, tt.defaultVal)
		if got != tt.want {
			t.Errorf("getFlagValue(%v, %q, %q) = %q, want %q", tt.args, tt.flag, tt.defaultVal, got, tt.want)
		}
	}
}

func TestPositionalArgs(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		flagsWithValue []string
		want           []string
	}{
		{
			name:           "simple positional",
			args:           []string{"file.py"},
			flagsWithValue: []string{"--gpu"},
			want:           []string{"file.py"},
		},
		{
			name:           "with value flag",
			args:           []string{"--gpu", "t4", "file.py"},
			flagsWithValue: []string{"--gpu"},
			want:           []string{"file.py"},
		},
		{
			name:           "with boolean flag",
			args:           []string{"--json", "file.py"},
			flagsWithValue: []string{"--gpu"},
			want:           []string{"file.py"},
		},
		{
			name:           "multiple positional",
			args:           []string{"local.bin", "remote.bin"},
			flagsWithValue: []string{"--gpu"},
			want:           []string{"local.bin", "remote.bin"},
		},
		{
			name:           "mixed flags and positional",
			args:           []string{"--gpu", "a100", "--json", "train.py"},
			flagsWithValue: []string{"--gpu"},
			want:           []string{"train.py"},
		},
		{
			name:           "no args",
			args:           []string{},
			flagsWithValue: []string{"--gpu"},
			want:           nil,
		},
		{
			name:           "only flags",
			args:           []string{"--json", "--gpu", "t4"},
			flagsWithValue: []string{"--gpu"},
			want:           nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := positionalArgs(tt.args, tt.flagsWithValue...)
			if len(got) != len(tt.want) {
				t.Fatalf("positionalArgs() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("positionalArgs()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		s      string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 5, "hello..."},
		{"", 5, ""},
		{"ab", 1, "a..."},
	}

	for _, tt := range tests {
		got := truncate(tt.s, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.maxLen, got, tt.want)
		}
	}
}
