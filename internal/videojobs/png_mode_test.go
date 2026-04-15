package videojobs

import "testing"

func TestNormalizePNGMode(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{raw: "", want: PNGModeSmartLLM},
		{raw: "smart_llm", want: PNGModeSmartLLM},
		{raw: "large-video", want: PNGModeSmartLLM},
		{raw: "fast_extract", want: PNGModeFastExtract},
		{raw: "normal", want: PNGModeFastExtract},
		{raw: "unknown", want: PNGModeSmartLLM},
	}
	for _, tc := range cases {
		if got := NormalizePNGMode(tc.raw); got != tc.want {
			t.Fatalf("NormalizePNGMode(%q)=%q want=%q", tc.raw, got, tc.want)
		}
	}
}

func TestResolvePNGFastExtractFPS(t *testing.T) {
	if got := ResolvePNGFastExtractFPS(map[string]interface{}{"fast_extract_fps": 2}); got != 2 {
		t.Fatalf("expected fps=2, got=%d", got)
	}
	if got := ResolvePNGFastExtractFPS(map[string]interface{}{"frame_interval_sec": 0.5}); got != 2 {
		t.Fatalf("expected derived fps=2, got=%d", got)
	}
	if got := ResolvePNGFastExtractFPS(map[string]interface{}{}); got != 1 {
		t.Fatalf("expected default fps=1, got=%d", got)
	}
}
