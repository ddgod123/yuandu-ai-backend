package videojobs

import (
	"testing"

	"emoji/internal/models"
)

func TestPackageZipEmojiSkipReason(t *testing.T) {
	tests := []struct {
		name   string
		item   models.Emoji
		reason string
	}{
		{
			name:   "empty file url",
			item:   models.Emoji{SizeBytes: 128},
			reason: "empty_file_url",
		},
		{
			name:   "zero sized output",
			item:   models.Emoji{FileURL: "emoji/demo/a.gif", SizeBytes: 0},
			reason: "non_positive_size",
		},
		{
			name:   "valid output",
			item:   models.Emoji{FileURL: "emoji/demo/a.gif", SizeBytes: 1024},
			reason: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := PackageZipEmojiSkipReason(tc.item); got != tc.reason {
				t.Fatalf("PackageZipEmojiSkipReason()=%q want=%q", got, tc.reason)
			}
		})
	}
}
