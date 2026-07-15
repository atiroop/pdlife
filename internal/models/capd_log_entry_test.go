package models

import "testing"

func TestCapdLogEntry_ComputeUF(t *testing.T) {
	e := CapdLogEntry{FillVolumeML: 2000, DrainVolumeML: 2450}
	e.ComputeUF()
	if e.UFVolumeML != 450 {
		t.Fatalf("UFVolumeML = %d, want 450", e.UFVolumeML)
	}
}

func TestCapdLogEntry_IsPeritonitisRisk(t *testing.T) {
	cases := []struct {
		appearance DialysateAppearance
		want       bool
	}{
		{DialysateClear, false},
		{DialysateCloudy, true},
		{DialysateBloody, true},
	}
	for _, tc := range cases {
		e := CapdLogEntry{DialysateAppearance: tc.appearance}
		if got := e.IsPeritonitisRisk(); got != tc.want {
			t.Errorf("IsPeritonitisRisk() with appearance=%s = %v, want %v", tc.appearance, got, tc.want)
		}
	}
}
