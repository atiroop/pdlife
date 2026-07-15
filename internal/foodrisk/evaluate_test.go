package foodrisk

import "testing"

func f(v float64) *float64 { return &v }

func TestEvaluatePotassium(t *testing.T) {
	cases := []struct {
		name  string
		value *float64
		want  Level
	}{
		{"nil is unknown", nil, LevelUnknown},
		{"negative is unknown", f(-1), LevelUnknown},
		{"zero is good", f(0), LevelGood},
		{"just below good boundary", f(199.9), LevelGood},
		{"exactly at good boundary is watch", f(200), LevelWatch},
		{"mid watch band", f(275), LevelWatch},
		{"exactly at alert boundary is watch", f(350), LevelWatch},
		{"just above alert boundary", f(350.1), LevelAlert},
		{"far above alert boundary", f(4413), LevelAlert},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EvaluatePotassium(tc.value)
			if got.Level != tc.want {
				t.Fatalf("EvaluatePotassium(%v) level = %v, want %v", derefOrNil(tc.value), got.Level, tc.want)
			}
			if tc.want != LevelUnknown && got.Detail == "" {
				t.Errorf("expected non-empty Detail for a known value, got Badge=%+v", got)
			}
			if tc.want == LevelUnknown && got.Tooltip == "" {
				t.Errorf("expected non-empty Tooltip for an unknown badge, got Badge=%+v", got)
			}
		})
	}
}

func TestEvaluatePhosphorus(t *testing.T) {
	cases := []struct {
		value *float64
		want  Level
	}{
		{nil, LevelUnknown},
		{f(-0.01), LevelUnknown},
		{f(99.9), LevelGood},
		{f(100), LevelWatch},
		{f(200), LevelWatch},
		{f(200.1), LevelAlert},
	}
	for _, tc := range cases {
		got := EvaluatePhosphorus(tc.value)
		if got.Level != tc.want {
			t.Errorf("EvaluatePhosphorus(%v) = %v, want %v", derefOrNil(tc.value), got.Level, tc.want)
		}
	}
}

func TestEvaluateSodium(t *testing.T) {
	cases := []struct {
		value *float64
		want  Level
	}{
		{nil, LevelUnknown},
		{f(-5), LevelUnknown},
		{f(119.9), LevelGood},
		{f(120), LevelWatch},
		{f(400), LevelWatch},
		{f(400.1), LevelAlert},
		{f(33274), LevelAlert}, // highest real value seen in the migrated Anamai data (rock salt)
	}
	for _, tc := range cases {
		got := EvaluateSodium(tc.value)
		if got.Level != tc.want {
			t.Errorf("EvaluateSodium(%v) = %v, want %v", derefOrNil(tc.value), got.Level, tc.want)
		}
	}
}

func TestEvaluateProtein(t *testing.T) {
	cases := []struct {
		name  string
		value *float64
		want  Level
	}{
		{"nil is unknown", nil, LevelUnknown},
		{"negative is unknown", f(-1), LevelUnknown},
		{"zero has no badge", f(0), LevelNone},
		{"just below threshold has no badge", f(11.9), LevelNone},
		{"exactly at threshold is positive", f(12), LevelPositive},
		{"well above threshold is positive", f(81.7), LevelPositive},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EvaluateProtein(tc.value)
			if got.Level != tc.want {
				t.Fatalf("EvaluateProtein(%v) = %v, want %v", derefOrNil(tc.value), got.Level, tc.want)
			}
		})
	}
}

func TestEvaluateEnergy(t *testing.T) {
	cases := []struct {
		name  string
		value *float64
		want  Level
	}{
		{"nil is unknown", nil, LevelUnknown},
		{"negative is unknown", f(-1), LevelUnknown},
		{"zero is plain (no badge)", f(0), LevelNone},
		{"any positive value is plain (no badge)", f(900), LevelNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EvaluateEnergy(tc.value)
			if got.Level != tc.want {
				t.Fatalf("EvaluateEnergy(%v) = %v, want %v", derefOrNil(tc.value), got.Level, tc.want)
			}
		})
	}
}

func TestEvaluateMoisture(t *testing.T) {
	cases := []struct {
		name  string
		value *float64
		want  Level
	}{
		{"nil is unknown", nil, LevelUnknown},
		{"negative is unknown", f(-1), LevelUnknown},
		{"just below threshold has no badge", f(69.9), LevelNone},
		{"exactly at threshold is info", f(70), LevelInfo},
		{"well above threshold is info", f(99.4), LevelInfo},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EvaluateMoisture(tc.value)
			if got.Level != tc.want {
				t.Fatalf("EvaluateMoisture(%v) = %v, want %v", derefOrNil(tc.value), got.Level, tc.want)
			}
		})
	}
}

func TestEvaluateDispatch(t *testing.T) {
	cases := []struct {
		nutrientName string
		value        *float64
		want         Level
	}{
		{"Potassium", f(500), LevelAlert},
		{"Phosphorus", f(50), LevelGood},
		{"Sodium", f(200), LevelWatch},
		{"Protein, total", f(20), LevelPositive},
		{"Energy, by calculation", f(300), LevelNone},
		{"Moisture", f(80), LevelInfo},
		{"Moisture", nil, LevelUnknown},
		{"Some Unrecognized Nutrient", f(5), LevelNone},
	}
	for _, tc := range cases {
		got := Evaluate(tc.nutrientName, tc.value)
		if got.Level != tc.want {
			t.Errorf("Evaluate(%q, %v) = %v, want %v", tc.nutrientName, derefOrNil(tc.value), got.Level, tc.want)
		}
	}
}

func TestLevelCSSClass(t *testing.T) {
	cases := []struct {
		level Level
		want  string
	}{
		{LevelGood, "pill-good"},
		{LevelPositive, "pill-good"},
		{LevelWatch, "pill-watch"},
		{LevelAlert, "pill-alert"},
		{LevelInfo, "pill-info"},
		{LevelUnknown, "pill-unknown"},
		{LevelNone, ""},
	}
	for _, tc := range cases {
		if got := tc.level.CSSClass(); got != tc.want {
			t.Errorf("Level(%q).CSSClass() = %q, want %q", tc.level, got, tc.want)
		}
	}
}

func derefOrNil(f *float64) interface{} {
	if f == nil {
		return nil
	}
	return *f
}
