package videojobs

import (
	"os/exec"
	"sort"
	"strings"
)

type FormatCapability struct {
	Format    string `json:"format"`
	Supported bool   `json:"supported"`
	Reason    string `json:"reason,omitempty"`
}

type RuntimeCapabilities struct {
	FFmpegAvailable    bool               `json:"ffmpeg_available"`
	FFprobeAvailable   bool               `json:"ffprobe_available"`
	SupportedFormats   []string           `json:"supported_formats"`
	UnsupportedFormats []string           `json:"unsupported_formats"`
	Formats            []FormatCapability `json:"formats"`
}

func DetectRuntimeCapabilities() RuntimeCapabilities {
	result := RuntimeCapabilities{
		Formats: make([]FormatCapability, 0, 6),
	}

	_, ffmpegErr := exec.LookPath("ffmpeg")
	_, ffprobeErr := exec.LookPath("ffprobe")
	result.FFmpegAvailable = ffmpegErr == nil
	result.FFprobeAvailable = ffprobeErr == nil

	add := func(format string, supported bool, reason string) {
		item := FormatCapability{
			Format:    format,
			Supported: supported,
			Reason:    strings.TrimSpace(reason),
		}
		result.Formats = append(result.Formats, item)
		if supported {
			result.SupportedFormats = append(result.SupportedFormats, format)
		} else {
			result.UnsupportedFormats = append(result.UnsupportedFormats, format)
		}
	}

	if !result.FFmpegAvailable {
		reason := "ffmpeg not found in PATH"
		add("jpg", false, reason)
		add("png", false, reason)
		add("gif", false, reason)
		add("webp", false, reason)
		add("mp4", false, reason)
		add("live", false, reason)
		sort.Strings(result.UnsupportedFormats)
		return result
	}

	if !result.FFprobeAvailable {
		reason := "ffprobe not found in PATH"
		add("jpg", false, reason)
		add("png", false, reason)
		add("gif", false, reason)
		add("webp", false, reason)
		add("mp4", false, reason)
		add("live", false, reason)
		sort.Strings(result.UnsupportedFormats)
		return result
	}

	add("jpg", true, "")
	add("png", true, "")
	add("gif", true, "")

	webpEncoder, webpErr := resolveWebPEncoder()
	if webpErr != nil {
		add("webp", false, webpErr.Error())
	} else if strings.TrimSpace(webpEncoder) == "" {
		add("webp", false, "webp encoder unavailable")
	} else {
		add("webp", true, "")
	}

	mp4Supported, mp4Err := supportsFFmpegEncoder("libx264")
	if mp4Err != nil {
		add("mp4", false, mp4Err.Error())
		add("live", false, mp4Err.Error())
	} else if !mp4Supported {
		reason := "ffmpeg missing libx264 encoder"
		add("mp4", false, reason)
		add("live", false, reason+" required by live package")
	} else {
		add("mp4", true, "")
		add("live", true, "")
	}

	sort.Strings(result.SupportedFormats)
	sort.Strings(result.UnsupportedFormats)
	return result
}
