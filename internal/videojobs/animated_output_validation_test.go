package videojobs

import "testing"

func TestInvalidAnimatedOutputReason(t *testing.T) {
	tests := []struct {
		name   string
		format string
		size   int64
		want   string
	}{
		{name: "zero gif", format: "gif", size: 0, want: "non_positive_size"},
		{name: "negative mp4", format: "mp4", size: -1, want: "non_positive_size"},
		{name: "empty format", format: "", size: 10, want: "unknown_format"},
		{name: "valid gif", format: "gif", size: 10, want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := invalidAnimatedOutputReason(tc.format, tc.size); got != tc.want {
				t.Fatalf("invalidAnimatedOutputReason(%q, %d)=%q want=%q", tc.format, tc.size, got, tc.want)
			}
		})
	}
}
