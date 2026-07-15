package labrange

import "testing"

func TestRange_Between_Boundaries(t *testing.T) {
	r := between(33, 36)
	cases := []struct {
		v        float64
		abnormal bool
		dir      string
	}{
		{32.9, true, "low"},
		{33, false, ""},
		{34.5, false, ""},
		{36, false, ""},
		{36.1, true, "high"},
	}
	for _, tc := range cases {
		abnormal, dir := r.Classify(tc.v)
		if abnormal != tc.abnormal || dir != tc.dir {
			t.Errorf("Classify(%v) = (%v,%q), want (%v,%q)", tc.v, abnormal, dir, tc.abnormal, tc.dir)
		}
	}
}

func TestRange_MinExclusive(t *testing.T) {
	r := minExclusive(65) // URR >65
	if abnormal, _ := r.Classify(65); !abnormal {
		t.Errorf("65 should be abnormal (low) for a strict >65 threshold")
	}
	if abnormal, _ := r.Classify(65.1); abnormal {
		t.Errorf("65.1 should be normal for a strict >65 threshold")
	}
}

func TestRange_MinInclusive(t *testing.T) {
	r := minInclusive(200) // Ferritin >=200
	if abnormal, _ := r.Classify(200); abnormal {
		t.Errorf("200 should be normal for a >=200 threshold")
	}
	if abnormal, dir := r.Classify(199.9); !abnormal || dir != "low" {
		t.Errorf("199.9 should be abnormal/low for a >=200 threshold, got abnormal=%v dir=%q", abnormal, dir)
	}
}

func TestRange_MaxExclusive(t *testing.T) {
	r := maxExclusive(7) // HbA1C <7
	if abnormal, _ := r.Classify(6.9); abnormal {
		t.Errorf("6.9 should be normal for a <7 threshold")
	}
	if abnormal, dir := r.Classify(7); !abnormal || dir != "high" {
		t.Errorf("7 should be abnormal/high for a strict <7 threshold, got abnormal=%v dir=%q", abnormal, dir)
	}
}

func TestRange_String(t *testing.T) {
	cases := []struct {
		r    Range
		want string
	}{
		{between(33, 36), "33-36"},
		{between(0.73, 1.18), "0.73-1.18"},
		{minExclusive(65), ">65"},
		{minInclusive(200), ">=200"},
		{maxExclusive(7), "<7"},
		{maxExclusive(200), "<200"},
	}
	for _, tc := range cases {
		if got := tc.r.String(); got != tc.want {
			t.Errorf("String() = %q, want %q", got, tc.want)
		}
	}
}

// Spot-check every named range against the spec's literal boundary
// values, so a future edit that fat-fingers a threshold fails loudly.
func TestNamedRanges_SpecValues(t *testing.T) {
	cases := []struct {
		name        string
		r           Range
		normalVal   float64
		abnormalVal float64
	}{
		{"Hct", HctRange, 34.5, 32},
		{"Hb", HbRange, 11, 9.2},
		{"WBC", WBCRange, 7000, 11000},
		{"Platelet", PlateletRange, 250000, 50000},
		{"BUN", BUNRange, 14, 25},
		{"Cr", CrRange, 1, 1.5},
		{"Na", NaRange, 140, 130},
		{"K", KRange, 4.5, 6},
		{"CO2", CO2Range, 25, 30},
		{"Ca", CaRange, 9.5, 8},
		{"PO4", PO4Range, 3.5, 5},
		{"Albumin", AlbuminRange, 3.8, 3.0},
		{"URR", URRRange, 70, 60},
		{"NPCR", NPCRRange, 1.5, 1.0},
		{"FBS", FBSRange, 90, 130},
		{"HbA1C", HbA1CRange, 6, 8},
		{"UricAcid", UricAcidRange, 5, 9},
		{"PTH", PTHRange, 250, 320},
		{"Ferritin", FerritinRange, 300, 100},
		{"SerumIron", SerumIronRange, 100, 20},
		{"TIBC", TIBCRange, 300, 200},
		{"TSat", TSatRange, 40, 20},
		{"Chol", CholRange, 180, 220},
		{"HDL", HDLRange, 45, 30},
		{"LDL", LDLRange, 110, 140},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if abnormal, _ := tc.r.Classify(tc.normalVal); abnormal {
				t.Errorf("%v should be classified normal", tc.normalVal)
			}
			if abnormal, _ := tc.r.Classify(tc.abnormalVal); !abnormal {
				t.Errorf("%v should be classified abnormal", tc.abnormalVal)
			}
		})
	}
}
