package main

import (
	"database/sql"
	"testing"
)

func TestNullFloat(t *testing.T) {
	cases := []struct {
		name  string
		input sql.NullString
		want  *float64
	}{
		{"invalid/NULL", sql.NullString{Valid: false}, nil},
		{"empty string", sql.NullString{String: "", Valid: true}, nil},
		{"dash (missing marker)", sql.NullString{String: "-", Valid: true}, nil},
		{"em dash", sql.NullString{String: "—", Valid: true}, nil},
		{"whitespace only", sql.NullString{String: "   ", Valid: true}, nil},
		{"plain integer", sql.NullString{String: "218", Valid: true}, floatPtr(218)},
		{"decimal", sql.NullString{String: "20.9", Valid: true}, floatPtr(20.9)},
		{"thousands separator", sql.NullString{String: "9,272", Valid: true}, floatPtr(9272)},
		{"negative", sql.NullString{String: "-1.5", Valid: true}, floatPtr(-1.5)},
		{"garbage text", sql.NullString{String: "n/a", Valid: true}, nil},
		{"leading/trailing space", sql.NullString{String: "  143  ", Valid: true}, floatPtr(143)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := nullFloat(tc.input, tc.name)
			if (got == nil) != (tc.want == nil) {
				t.Fatalf("nullFloat(%q) = %v, want %v", tc.input.String, got, tc.want)
			}
			if got != nil && *got != *tc.want {
				t.Fatalf("nullFloat(%q) = %v, want %v", tc.input.String, *got, *tc.want)
			}
		})
	}
}

func floatPtr(v float64) *float64 { return &v }
