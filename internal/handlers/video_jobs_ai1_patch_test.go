package handlers

import (
	"math"
	"testing"
)

func TestNormalizeAI1PatchQualityWeights(t *testing.T) {
	weights, err := normalizeAI1PatchQualityWeights(map[string]float64{
		"semantic":   0.5,
		"clarity":    0.3,
		"loop":       0.1,
		"efficiency": 0.1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sum := weights["semantic"] + weights["clarity"] + weights["loop"] + weights["efficiency"]
	if math.Abs(sum-1.0) > 0.0001 {
		t.Fatalf("expected normalized sum=1, got %.6f", sum)
	}
	if weights["semantic"] <= weights["loop"] {
		t.Fatalf("expected semantic weight > loop weight, got %+v", weights)
	}
}

func TestNormalizeAI1PatchQualityWeights_Invalid(t *testing.T) {
	_, err := normalizeAI1PatchQualityWeights(map[string]float64{
		"semantic": 0,
		"clarity":  0,
	})
	if err == nil {
		t.Fatalf("expected error when all weights are zero")
	}
}

func TestNormalizeAI1PatchMaxBlurTolerance(t *testing.T) {
	tolerance, err := normalizeAI1PatchMaxBlurTolerance("", "png")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tolerance != "low" {
		t.Fatalf("expected default png tolerance low, got %q", tolerance)
	}
	tolerance, err = normalizeAI1PatchMaxBlurTolerance("high", "png")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tolerance != "high" {
		t.Fatalf("expected high, got %q", tolerance)
	}
}

func TestNormalizeAI1PatchRiskFlags(t *testing.T) {
	flags := normalizeAI1PatchRiskFlags([]string{"Low Light", "fast-motion", " fast motion "}, 8)
	if len(flags) != 2 {
		t.Fatalf("expected two unique normalized flags, got %+v", flags)
	}
	if flags[0] != "low_light" || flags[1] != "fast_motion" {
		t.Fatalf("unexpected normalized flags: %+v", flags)
	}
}
