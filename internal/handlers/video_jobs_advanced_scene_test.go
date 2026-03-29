package handlers

import (
	"testing"

	"emoji/internal/videojobs"
)

func TestBuildVideoJobAdvancedSceneOptionItems_OrderAndFields(t *testing.T) {
	items := buildVideoJobAdvancedSceneOptionItems(map[string]videojobs.VideoJobAI1StrategyProfile{
		"ecom_cover": {
			Scene:             "ecom_cover",
			SceneLabel:        "电商封面",
			DirectiveHint:     "封面导向",
			OperatorIdentity:  "电商编辑",
			CandidateCountMin: 5,
			CandidateCountMax: 12,
		},
		videojobs.AdvancedScenarioXiaohongshu: {
			Scene:             videojobs.AdvancedScenarioXiaohongshu,
			SceneLabel:        "小红书网感",
			DirectiveHint:     "高吸引力",
			OperatorIdentity:  "时尚视觉总监",
			CandidateCountMin: 4,
			CandidateCountMax: 8,
		},
		videojobs.AdvancedScenarioDefault: {
			Scene:             videojobs.AdvancedScenarioDefault,
			SceneLabel:        "通用截图",
			DirectiveHint:     "平衡策略",
			OperatorIdentity:  "视觉总监",
			CandidateCountMin: 4,
			CandidateCountMax: 8,
		},
	})

	if len(items) != 3 {
		t.Fatalf("expected 3 scene options, got %d", len(items))
	}
	if items[0].Scene != videojobs.AdvancedScenarioDefault {
		t.Fatalf("expected default scene first, got %q", items[0].Scene)
	}
	if items[1].Scene != videojobs.AdvancedScenarioXiaohongshu {
		t.Fatalf("expected built-in scene xiaohongshu second, got %q", items[1].Scene)
	}
	if items[2].Scene != "ecom_cover" {
		t.Fatalf("expected custom scene at tail, got %q", items[2].Scene)
	}
	if items[2].OperatorIdentity != "电商编辑" {
		t.Fatalf("expected operator identity carried, got %q", items[2].OperatorIdentity)
	}
	if items[2].CandidateCountMin != 5 || items[2].CandidateCountMax != 12 {
		t.Fatalf("unexpected candidate count range: %+v", items[2])
	}
}

func TestFilterVideoJobAdvancedSceneProfilesByMainline_PNGOnlyDefaultAndXHS(t *testing.T) {
	profiles := map[string]videojobs.VideoJobAI1StrategyProfile{
		videojobs.AdvancedScenarioDefault: {
			Scene:      videojobs.AdvancedScenarioDefault,
			SceneLabel: "通用截图",
		},
		videojobs.AdvancedScenarioXiaohongshu: {
			Scene:      videojobs.AdvancedScenarioXiaohongshu,
			SceneLabel: "小红书网感",
		},
		videojobs.AdvancedScenarioWallpaper: {
			Scene:      videojobs.AdvancedScenarioWallpaper,
			SceneLabel: "手机壁纸",
		},
		"custom_scene": {
			Scene:      "custom_scene",
			SceneLabel: "自定义场景",
		},
	}

	filtered := filterVideoJobAdvancedSceneProfilesByMainline("png", profiles)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 scenes for png mainline, got %d", len(filtered))
	}
	if _, ok := filtered[videojobs.AdvancedScenarioDefault]; !ok {
		t.Fatalf("expected default scene kept")
	}
	if _, ok := filtered[videojobs.AdvancedScenarioXiaohongshu]; !ok {
		t.Fatalf("expected xiaohongshu scene kept")
	}
	if _, ok := filtered[videojobs.AdvancedScenarioWallpaper]; ok {
		t.Fatalf("expected wallpaper scene dropped in png mainline")
	}
	if _, ok := filtered["custom_scene"]; ok {
		t.Fatalf("expected custom scene dropped in png mainline")
	}
}

func TestFilterVideoJobAdvancedSceneProfilesByMainline_NonPNGKeepOriginal(t *testing.T) {
	profiles := map[string]videojobs.VideoJobAI1StrategyProfile{
		videojobs.AdvancedScenarioDefault: {
			Scene: videojobs.AdvancedScenarioDefault,
		},
		videojobs.AdvancedScenarioXiaohongshu: {
			Scene: videojobs.AdvancedScenarioXiaohongshu,
		},
		videojobs.AdvancedScenarioWallpaper: {
			Scene: videojobs.AdvancedScenarioWallpaper,
		},
	}

	filtered := filterVideoJobAdvancedSceneProfilesByMainline("gif", profiles)
	if len(filtered) != 3 {
		t.Fatalf("expected non-png keep full set, got %d", len(filtered))
	}
	if _, ok := filtered[videojobs.AdvancedScenarioWallpaper]; !ok {
		t.Fatalf("expected wallpaper scene kept for non-png")
	}
}
