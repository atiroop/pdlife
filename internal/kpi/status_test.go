package kpi

import "testing"

func TestCapdDailyUF(t *testing.T) {
	cases := []struct {
		ml   float64
		want Status
	}{
		{0, StatusAlert},
		{-50, StatusAlert},
		{100, StatusWatch},
		{399, StatusWatch},
		{400, StatusGood},
		{1500, StatusGood},
		{1501, StatusWatch},
		{2000, StatusWatch},
	}
	for _, tc := range cases {
		if got := CapdDailyUF(tc.ml); got != tc.want {
			t.Errorf("CapdDailyUF(%v) = %v, want %v", tc.ml, got, tc.want)
		}
	}
}

func TestHDPostVsDry(t *testing.T) {
	cases := []struct {
		deltaKg float64
		want    Status
	}{
		{0, StatusGood},
		{0.5, StatusGood},
		{-0.5, StatusGood},
		{0.6, StatusWatch},
		{1.5, StatusWatch},
		{1.6, StatusAlert},
		{-0.6, StatusWatch},
		{-1.0, StatusWatch},
		{-1.1, StatusAlert},
	}
	for _, tc := range cases {
		if got := HDPostVsDry(tc.deltaKg); got != tc.want {
			t.Errorf("HDPostVsDry(%v) = %v, want %v", tc.deltaKg, got, tc.want)
		}
	}
}

func TestHDInterdialyticGain(t *testing.T) {
	cases := []struct {
		gainKg float64
		want   Status
	}{
		{-1, StatusGood},
		{0, StatusGood},
		{2, StatusGood},
		{2.5, StatusWatch},
		{3, StatusWatch},
		{3.1, StatusAlert},
	}
	for _, tc := range cases {
		if got := HDInterdialyticGain(tc.gainKg); got != tc.want {
			t.Errorf("HDInterdialyticGain(%v) = %v, want %v", tc.gainKg, got, tc.want)
		}
	}
}

func TestHDPostDialysisBP(t *testing.T) {
	cases := []struct {
		systolic int
		want     Status
	}{
		{110, StatusGood},
		{100, StatusGood},
		{99, StatusWatch},
		{90, StatusWatch},
		{89, StatusAlert},
		{70, StatusAlert},
	}
	for _, tc := range cases {
		if got := HDPostDialysisBP(tc.systolic); got != tc.want {
			t.Errorf("HDPostDialysisBP(%v) = %v, want %v", tc.systolic, got, tc.want)
		}
	}
}
