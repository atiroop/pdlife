package models

import "testing"

func TestHdLogEntry_ComputeUFRemoved(t *testing.T) {
	cases := []struct {
		pre, post float64
		want      int
	}{
		{70.5, 68.5, 2000},
		{68.0, 68.0, 0},
		{65.0, 65.5, -500}, // gained weight during the session — negative UF
		{67.0, 65.2, 1800}, // 67.0-65.2 == 1.7999999999999972 in IEEE 754 float64 — must round, not truncate, to 1800
	}
	for _, tc := range cases {
		e := HdLogEntry{PreDialysisWeightKG: tc.pre, PostDialysisWeightKG: tc.post}
		e.ComputeUFRemoved()
		if e.UFRemovedML != tc.want {
			t.Errorf("ComputeUFRemoved() pre=%v post=%v: UFRemovedML = %d, want %d", tc.pre, tc.post, e.UFRemovedML, tc.want)
		}
	}
}
