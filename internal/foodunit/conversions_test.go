package foodunit

import "testing"

func floatPtr(v float64) *float64 { return &v }

func TestResolveDensity_RealValue(t *testing.T) {
	f := FoodInfo{NameTh: "อะไรก็ได้"}
	got := ResolveDensity(f, floatPtr(1.03))
	if got.GPerML == nil || *got.GPerML != 1.03 {
		t.Fatalf("GPerML = %v, want 1.03", got.GPerML)
	}
	if got.Source != "density" || got.IsFallback {
		t.Errorf("Source=%q IsFallback=%v, want \"density\"/false", got.Source, got.IsFallback)
	}
}

func TestResolveDensity_ZeroOrNegativeIgnored(t *testing.T) {
	f := FoodInfo{NameTh: "เกลือป่น"}
	got := ResolveDensity(f, floatPtr(0))
	if got.Source != "table_salt" {
		t.Errorf("a zero measured density should fall through to fallback matching, got Source=%q", got.Source)
	}
}

func TestResolveDensity_FallbackKeywords(t *testing.T) {
	cases := []struct {
		name       string
		food       FoodInfo
		wantSource string
		wantFound  bool
	}{
		{"salt exact-start Thai", FoodInfo{NameTh: "เกลือป่น"}, "table_salt", true},
		{"salt exact-start English", FoodInfo{NameEn: "Salt, table"}, "table_salt", true},
		{"salt not at start doesn't match", FoodInfo{NameTh: "น้ำเกลือ"}, "", false},
		{"sugar contains anywhere", FoodInfo{NameTh: "ขนมน้ำตาลทรายแดง"}, "granulated_sugar", true},
		{"sauce condiment status N", FoodInfo{NameTh: "น้ำปลา", Status: "N"}, "sauce_without_density", true},
		{"sauce condiment wrong status doesn't match", FoodInfo{NameTh: "น้ำปลา", Status: "G"}, "", false},
		{"soy sauce English condiment", FoodInfo{NameEn: "soy sauce, light", Status: "N"}, "sauce_without_density", true},
		{"no match", FoodInfo{NameTh: "ข้าวสวย", Status: "A"}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveDensity(tc.food, nil)
			if tc.wantFound {
				if got.GPerML == nil {
					t.Fatalf("expected a fallback density, got none")
				}
				if got.Source != tc.wantSource {
					t.Errorf("Source = %q, want %q", got.Source, tc.wantSource)
				}
				if !got.IsFallback {
					t.Errorf("IsFallback = false, want true")
				}
			} else if got.GPerML != nil {
				t.Errorf("expected no density, got %v (source=%q)", *got.GPerML, got.Source)
			}
		})
	}
}

func TestGetUnitConversions_MassAlwaysAvailable(t *testing.T) {
	conv := GetUnitConversions(FoodInfo{NameTh: "ข้าวสวย", Status: "A"}, nil)
	if conv.DensityAvailable {
		t.Fatalf("plain rice should have no density")
	}
	for _, code := range []string{"g", "kg", "oz"} {
		u := findUnit(t, conv, code)
		if !u.Available || u.GramsPerUnit == nil {
			t.Errorf("mass unit %q should always be available", code)
		}
	}
	for _, code := range []string{"ml", "l", "tsp", "tbsp", "cup"} {
		u := findUnit(t, conv, code)
		if u.Available {
			t.Errorf("volume unit %q should be unavailable without density", code)
		}
	}
}

func TestGetUnitConversions_VolumeGramsPerUnit(t *testing.T) {
	// A food with a real, measured density of 1.03 g/mL.
	conv := GetUnitConversions(FoodInfo{NameTh: "น้ำซุป"}, floatPtr(1.03))
	cases := map[string]float64{
		"ml":   1.03,
		"l":    1030,
		"tsp":  5.15,  // 1.03 * 5
		"tbsp": 15.45, // 1.03 * 15
		"cup":  247.2, // 1.03 * 240
	}
	for code, want := range cases {
		u := findUnit(t, conv, code)
		if !u.Available || u.GramsPerUnit == nil {
			t.Fatalf("unit %q should be available", code)
		}
		if *u.GramsPerUnit != want {
			t.Errorf("unit %q grams_per_unit = %v, want %v", code, *u.GramsPerUnit, want)
		}
	}
}

func TestGramsFor(t *testing.T) {
	conv := GetUnitConversions(FoodInfo{}, floatPtr(1.0))
	if g := GramsFor(2, "tbsp", conv); g == nil || *g != 30 {
		t.Errorf("2 tbsp at density 1.0 = %v, want 30", g)
	}
	if g := GramsFor(100, "g", conv); g == nil || *g != 100 {
		t.Errorf("100 g = %v, want 100", g)
	}
	if g := GramsFor(1, "cup", GetUnitConversions(FoodInfo{NameTh: "ข้าวสวย", Status: "A"}, nil)); g != nil {
		t.Errorf("cup should be unavailable without density, got %v", g)
	}
	if g := GramsFor(1, "not-a-real-unit", conv); g != nil {
		t.Errorf("unknown unit code should return nil, got %v", g)
	}
}

func TestScaleValue(t *testing.T) {
	if got := ScaleValue(150, 200); got != 300 {
		t.Errorf("ScaleValue(150, 200) = %v, want 300", got)
	}
	if got := ScaleValue(150, 50); got != 75 {
		t.Errorf("ScaleValue(150, 50) = %v, want 75", got)
	}
}

func findUnit(t *testing.T, conv Conversions, code string) UnitConversion {
	t.Helper()
	for _, u := range conv.Units {
		if u.Code == code {
			return u
		}
	}
	t.Fatalf("unit %q not found in conversions", code)
	return UnitConversion{}
}
