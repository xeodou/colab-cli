package main

import (
	"encoding/json"
	"testing"
)

func TestParseNotebookCells(t *testing.T) {
	// Minimal .ipynb with mixed cell types
	nb := map[string]interface{}{
		"cells": []interface{}{
			map[string]interface{}{
				"cell_type": "markdown",
				"source":    "# Title",
			},
			map[string]interface{}{
				"cell_type": "code",
				"source":    "print('hello')",
			},
			map[string]interface{}{
				"cell_type": "code",
				"source":    []interface{}{"import ", "torch\n", "print(torch.__version__)"},
			},
			map[string]interface{}{
				"cell_type": "code",
				"source":    "", // empty code cell should be skipped
			},
			map[string]interface{}{
				"cell_type": "code",
				"source":    "   \n  \t  ", // whitespace-only should be skipped
			},
		},
	}

	data, err := json.Marshal(nb)
	if err != nil {
		t.Fatal(err)
	}

	cells, err := parseNotebookCells(data)
	if err != nil {
		t.Fatalf("parseNotebookCells() error: %v", err)
	}

	if len(cells) != 2 {
		t.Fatalf("got %d cells, want 2", len(cells))
	}

	if cells[0] != "print('hello')" {
		t.Errorf("cells[0] = %q, want %q", cells[0], "print('hello')")
	}

	want1 := "import torch\nprint(torch.__version__)"
	if cells[1] != want1 {
		t.Errorf("cells[1] = %q, want %q", cells[1], want1)
	}
}

func TestParseNotebookCells_NoCells(t *testing.T) {
	data := []byte(`{"cells": []}`)
	cells, err := parseNotebookCells(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cells) != 0 {
		t.Errorf("got %d cells, want 0", len(cells))
	}
}

func TestParseNotebookCells_InvalidJSON(t *testing.T) {
	_, err := parseNotebookCells([]byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestExtractSource(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  string
	}{
		{"string source", "print('hello')", "print('hello')"},
		{"array source", []interface{}{"line1\n", "line2"}, "line1\nline2"},
		{"empty array", []interface{}{}, ""},
		{"nil", nil, ""},
		{"number", 42, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSource(tt.input)
			if got != tt.want {
				t.Errorf("extractSource() = %q, want %q", got, tt.want)
			}
		})
	}
}
