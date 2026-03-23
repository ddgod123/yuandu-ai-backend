package videojobs

import "testing"

func TestBuildGIFRenderSchedule_SortByCostDesc(t *testing.T) {
	tasks := []animatedTask{
		{Format: "gif", RenderCostUnits: 1.2},
		{Format: "gif", RenderCostUnits: 3.4},
		{Format: "gif", RenderCostUnits: 2.1},
		{Format: "jpg"},
	}
	items := buildGIFRenderSchedule(tasks)
	if len(items) != len(tasks) {
		t.Fatalf("unexpected schedule size: got=%d want=%d", len(items), len(tasks))
	}
	for i := 1; i < len(items); i++ {
		if items[i-1].CostUnits < items[i].CostUnits {
			t.Fatalf("schedule is not sorted desc at %d: prev=%.3f curr=%.3f", i, items[i-1].CostUnits, items[i].CostUnits)
		}
	}
}

func TestEstimateGIFRenderScheduleCost_BundleAndMezzanineDiscount(t *testing.T) {
	baseTask := animatedTask{Format: "gif", RenderCostUnits: 3.0}
	bundleFirst := animatedTask{Format: "gif", RenderCostUnits: 3.0, BundleID: "gif_bundle_001"}
	bundleLaterWithMezz := animatedTask{
		Format:          "gif",
		RenderCostUnits: 3.0,
		BundleID:        "gif_bundle_001",
		MezzaninePath:   "/tmp/mezz.mp4",
	}
	base := estimateGIFRenderScheduleCost(baseTask, 0)
	first := estimateGIFRenderScheduleCost(bundleFirst, 0)
	later := estimateGIFRenderScheduleCost(bundleLaterWithMezz, 1)
	if !(first < base) {
		t.Fatalf("expected bundle first cost lower than base: base=%.3f first=%.3f", base, first)
	}
	if !(later < first) {
		t.Fatalf("expected bundle+mezz later cost lower than bundle first: first=%.3f later=%.3f", first, later)
	}
}

func TestResolveGIFRenderMaxCostUnits_Positive(t *testing.T) {
	meta := videoProbeMeta{Width: 1080, Height: 1920, DurationSec: 95}
	settings := NormalizeQualitySettings(DefaultQualitySettings())
	tasks := []animatedTask{
		{Format: "gif"},
		{Format: "gif"},
		{Format: "gif"},
	}
	got := resolveGIFRenderMaxCostUnits(meta, tasks, settings, 2)
	if got < 1 {
		t.Fatalf("expected max cost units >= 1, got %.3f", got)
	}
}

func TestResolveGIFRenderMaxCostUnits_ShortClipBoost(t *testing.T) {
	meta := videoProbeMeta{Width: 720, Height: 1280, DurationSec: 12}
	settings := NormalizeQualitySettings(DefaultQualitySettings())
	tasks := []animatedTask{
		{Format: "gif"},
		{Format: "gif"},
		{Format: "gif"},
		{Format: "gif"},
	}
	got := resolveGIFRenderMaxCostUnits(meta, tasks, settings, 4)
	if got <= 8.0 {
		t.Fatalf("expected short-clip boosted max units > 8, got %.3f", got)
	}
}
