package storage

import (
	"path"
	"strings"
)

const defaultRootPrefix = "emoji"

func NormalizeRootPrefix(raw string) string {
	root := strings.TrimSpace(raw)
	root = strings.Trim(root, "/")
	root = strings.ReplaceAll(root, "\\", "")
	root = strings.ReplaceAll(root, "//", "/")
	if root == "" {
		root = defaultRootPrefix
	}
	return root + "/"
}

func EnsureRootPrefix(key, rootPrefix string) string {
	key = strings.TrimSpace(key)
	key = strings.TrimLeft(key, "/")
	if key == "" {
		return strings.TrimSuffix(NormalizeRootPrefix(rootPrefix), "/")
	}
	root := NormalizeRootPrefix(rootPrefix)
	if strings.HasPrefix(key, root) {
		return key
	}
	return path.Join(strings.TrimSuffix(root, "/"), key)
}

func HasRootPrefix(key, rootPrefix string) bool {
	key = strings.TrimLeft(strings.TrimSpace(key), "/")
	if key == "" {
		return false
	}
	return strings.HasPrefix(key, NormalizeRootPrefix(rootPrefix))
}

func TrashPrefix(rootPrefix string) string {
	root := strings.TrimSuffix(NormalizeRootPrefix(rootPrefix), "/")
	return path.Join(root, "_trash") + "/"
}

func CollectionsPrefix(rootPrefix string) string {
	root := strings.TrimSuffix(NormalizeRootPrefix(rootPrefix), "/")
	return path.Join(root, "collections") + "/"
}

func VideoImagePrefix(rootPrefix string) string {
	root := strings.TrimSuffix(NormalizeRootPrefix(rootPrefix), "/")
	return path.Join(root, "video-image")
}
