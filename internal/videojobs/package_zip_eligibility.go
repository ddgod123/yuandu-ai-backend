package videojobs

import (
	"strings"

	"emoji/internal/models"
)

// PackageZipEmojiSkipReason returns the reason why an emoji should be skipped
// when building a job package zip. Empty string means the emoji is eligible.
func PackageZipEmojiSkipReason(item models.Emoji) string {
	if strings.TrimSpace(item.FileURL) == "" {
		return "empty_file_url"
	}
	if item.SizeBytes <= 0 {
		return "non_positive_size"
	}
	return ""
}
