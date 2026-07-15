package handler

import (
	"strings"
	"testing"
	"time"

	"github.com/atiroop/pdlife/internal/models"
)

func labDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parse date %q: %v", s, err)
	}
	return d
}

func f64(v float64) *float64 { return &v }
func i(v int) *int           { return &v }

// TestBuildLabAbnormalItems_FlagsHighAndLowValues mirrors the spec's own
// test scenario: PTH above range, Hb below range.
func TestBuildLabAbnormalItems_FlagsHighAndLowValues(t *testing.T) {
	rows := []models.LabResult{
		{LogDate: labDate(t, "2026-07-01"), PTH: f64(320), Hb: f64(9.2)},
	}
	latest := resolveLatestLabValues(rows, false)
	items := buildLabAbnormalItems(latest, false)

	var messages []string
	for _, it := range items {
		messages = append(messages, it.Message)
	}
	joined := strings.Join(messages, " | ")

	if !strings.Contains(joined, "PTH สูงกว่าเกณฑ์: 320") || !strings.Contains(joined, "150-300") {
		t.Errorf("expected PTH-high message, got: %s", joined)
	}
	if !strings.Contains(joined, "Hb ต่ำกว่าเกณฑ์: 9.2") || !strings.Contains(joined, "10-12") {
		t.Errorf("expected Hb-low message, got: %s", joined)
	}
}

// TestBuildLabAbnormalItems_AllNormalIsEmpty confirms the "all in range"
// case yields no items (the template shows the green "ปกติทุกรายการ"
// message when len(items) == 0).
func TestBuildLabAbnormalItems_AllNormalIsEmpty(t *testing.T) {
	rows := []models.LabResult{
		{LogDate: labDate(t, "2026-07-01"), Hb: f64(11), K: f64(4.5), Albumin: f64(3.8)},
	}
	latest := resolveLatestLabValues(rows, false)
	items := buildLabAbnormalItems(latest, false)
	if len(items) != 0 {
		t.Errorf("expected no abnormal items, got %d: %+v", len(items), items)
	}
}

// TestBuildLabAbnormalItems_NoSynthesisAcrossValues guards the hard rule
// from the spec: even when PTH, Ca, and PO4 are simultaneously abnormal,
// each gets its own independent line — never a combined interpretive
// sentence spanning multiple values.
func TestBuildLabAbnormalItems_NoSynthesisAcrossValues(t *testing.T) {
	rows := []models.LabResult{
		{LogDate: labDate(t, "2026-07-01"), PTH: f64(500), Ca: f64(7), PO4: f64(6)},
	}
	latest := resolveLatestLabValues(rows, false)
	items := buildLabAbnormalItems(latest, false)
	if len(items) != 3 {
		t.Fatalf("expected exactly 3 independent items (PTH, Ca, PO4), got %d: %+v", len(items), items)
	}
	for _, it := range items {
		if strings.Count(it.Message, "%") > 0 || strings.Contains(it.Message, "โรค") {
			t.Errorf("message must not editorialize/diagnose: %q", it.Message)
		}
	}
}

// TestResolveLatestLabValues_PerFieldIndependence is the core behavior the
// spec calls out: the "latest" value for each field comes from whichever
// row most recently had that field filled in, not from a single "most
// recent row" — since different tests run on different schedules.
func TestResolveLatestLabValues_PerFieldIndependence(t *testing.T) {
	rows := []models.LabResult{
		// Most recent row only has Hb.
		{LogDate: labDate(t, "2026-07-01"), Hb: f64(9.0)},
		// Older row has PTH (not re-tested since, e.g. every-6-months cadence).
		{LogDate: labDate(t, "2026-04-01"), PTH: f64(400), Hb: f64(11.5)},
	}
	// rows must be pre-sorted log_date DESC for resolveLatestLabValues.
	latest := resolveLatestLabValues(rows, false)

	hb, ok := latest.Numeric["hb"]
	if !ok || hb.Value != 9.0 {
		t.Errorf("Hb should resolve to the 07-01 value (9.0), got %+v ok=%v", hb, ok)
	}
	pth, ok := latest.Numeric["pth"]
	if !ok || pth.Value != 400 {
		t.Errorf("PTH should resolve to the 04-01 value (400) since 07-01 has no PTH, got %+v ok=%v", pth, ok)
	}
}

// TestBuildLabAbnormalItems_HDOnlyFieldsGatedByIsHD confirms URR/nPCR are
// only evaluated when isHD is true — a non-HD patient's URR/nPCR (if any
// ever got saved) must never appear on their abnormal-values list.
func TestBuildLabAbnormalItems_HDOnlyFieldsGatedByIsHD(t *testing.T) {
	rows := []models.LabResult{
		{LogDate: labDate(t, "2026-07-01"), URR: f64(40), NPCR: f64(0.8)}, // both abnormal per spec ranges
	}

	latestNonHD := resolveLatestLabValues(rows, false)
	itemsNonHD := buildLabAbnormalItems(latestNonHD, false)
	if len(itemsNonHD) != 0 {
		t.Errorf("non-HD patient should never see URR/nPCR abnormal items, got %+v", itemsNonHD)
	}

	latestHD := resolveLatestLabValues(rows, true)
	itemsHD := buildLabAbnormalItems(latestHD, true)
	if len(itemsHD) != 2 {
		t.Errorf("HD patient should see both URR and nPCR abnormal items, got %d: %+v", len(itemsHD), itemsHD)
	}
}

// TestBuildLabAbnormalItems_EnumFields covers HBsAg (abnormal=positive)
// and HBsAb (abnormal=negative, the reversed one).
func TestBuildLabAbnormalItems_EnumFields(t *testing.T) {
	positive := models.LabResultPositive
	negative := models.LabResultNegative
	rows := []models.LabResult{
		{LogDate: labDate(t, "2026-07-01"), HBsAg: &positive, HBsAb: &negative, AntiHCV: &negative},
	}
	latest := resolveLatestLabValues(rows, false)
	items := buildLabAbnormalItems(latest, false)

	var messages []string
	for _, it := range items {
		messages = append(messages, it.Message)
	}
	joined := strings.Join(messages, " | ")
	if !strings.Contains(joined, "HBsAg ผลบวก") {
		t.Errorf("expected HBsAg-positive-abnormal message, got: %s", joined)
	}
	if !strings.Contains(joined, "HBsAb ผลลบ") {
		t.Errorf("expected HBsAb-negative-abnormal message (reversed normal), got: %s", joined)
	}
	if strings.Contains(joined, "Anti HCV") {
		t.Errorf("Anti HCV negative is normal, should not appear: %s", joined)
	}
}

// TestIntToFloatPtr covers the *int -> *float64 adapter used for
// WBC/PlateletCount.
func TestIntToFloatPtr(t *testing.T) {
	if v := intToFloatPtr(nil); v != nil {
		t.Errorf("nil should stay nil, got %v", v)
	}
	if v := intToFloatPtr(i(7000)); v == nil || *v != 7000 {
		t.Errorf("expected 7000, got %v", v)
	}
}
