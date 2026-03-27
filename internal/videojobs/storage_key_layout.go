package videojobs

import (
	"fmt"
	"path"
	"strings"
)

const defaultVideoImageStorageRoot = "emoji/video-image"

type VideoImageStorageLayout struct {
	RootPrefix string
	Env        string
}

func NewVideoImageStorageLayout(env string) VideoImageStorageLayout {
	root := defaultVideoImageStorageRoot
	env = strings.TrimSpace(strings.ToLower(env))
	if env == "" {
		env = "prod"
	}
	return VideoImageStorageLayout{
		RootPrefix: root,
		Env:        env,
	}
}

func (l VideoImageStorageLayout) UserShard(userID uint64) string {
	return fmt.Sprintf("%02d", userID%100)
}

func (l VideoImageStorageLayout) JobPrefix(userID, jobID uint64) string {
	return l.JobPrefixByFormat(userID, jobID, "")
}

func (l VideoImageStorageLayout) JobPrefixByFormat(userID, jobID uint64, primaryFormat string) string {
	primaryFormat = NormalizeRequestedFormat(primaryFormat)
	prefix := path.Join(
		strings.Trim(l.RootPrefix, "/"),
		l.Env,
		func() string {
			if primaryFormat != "" {
				return path.Join("f", primaryFormat)
			}
			return ""
		}(),
		"u",
		l.UserShard(userID),
		fmt.Sprintf("%d", userID),
		"j",
		fmt.Sprintf("%d", jobID),
	)
	return ensureTrailingSlash(prefix)
}

func (l VideoImageStorageLayout) SourceKey(userID, jobID uint64, filename string) string {
	filename = sanitizeFileComponent(filename)
	if filename == "" {
		filename = "source.mp4"
	}
	return path.Join(strings.TrimSuffix(l.JobPrefix(userID, jobID), "/"), "source", filename)
}

func (l VideoImageStorageLayout) OutputKey(userID, jobID uint64, format string, seq int, ext string) string {
	format = sanitizeFileComponent(strings.ToLower(format))
	if format == "" {
		format = "unknown"
	}
	ext = normalizeExt(ext)
	if ext == "" {
		ext = format
	}
	name := fmt.Sprintf("%03d.%s", maxIntValue(1, seq), ext)
	return path.Join(strings.TrimSuffix(l.JobPrefix(userID, jobID), "/"), "outputs", format, name)
}

func (l VideoImageStorageLayout) ThumbnailKey(userID, jobID uint64, format string, seq int) string {
	format = sanitizeFileComponent(strings.ToLower(format))
	if format == "" {
		format = "unknown"
	}
	name := fmt.Sprintf("thumb_%03d.jpg", maxIntValue(1, seq))
	return path.Join(strings.TrimSuffix(l.JobPrefix(userID, jobID), "/"), "outputs", format, name)
}

func (l VideoImageStorageLayout) PackageKey(userID, jobID uint64, format string, version int) string {
	format = sanitizeFileComponent(strings.ToLower(format))
	if format == "" {
		format = "mixed"
	}
	version = maxIntValue(1, version)
	name := fmt.Sprintf("%d_%s_v%d.zip", jobID, format, version)
	return path.Join(strings.TrimSuffix(l.JobPrefix(userID, jobID), "/"), "package", name)
}

func (l VideoImageStorageLayout) ManifestKey(userID, jobID uint64) string {
	return path.Join(strings.TrimSuffix(l.JobPrefix(userID, jobID), "/"), "manifest", "result_manifest_v1.json")
}

func ensureTrailingSlash(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "/")
	if s == "" {
		return ""
	}
	if !strings.HasSuffix(s, "/") {
		s += "/"
	}
	return s
}

func sanitizeFileComponent(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.ReplaceAll(s, "\\", "")
	s = strings.ReplaceAll(s, "..", "")
	s = strings.ReplaceAll(s, "//", "/")
	s = strings.Trim(s, "/")
	return s
}

func normalizeExt(ext string) string {
	ext = strings.TrimSpace(strings.ToLower(ext))
	ext = strings.TrimPrefix(ext, ".")
	ext = sanitizeFileComponent(ext)
	return ext
}
