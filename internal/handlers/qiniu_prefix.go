package handlers

import (
	"path"
	"strconv"
	"strings"

	"emoji/internal/storage"
)

const qiniuLegacyRootPrefix = "emoji"

func (h *Handler) qiniuRootPrefix() string {
	return storage.NormalizeRootPrefix(h.cfg.QiniuRootPrefix)
}

func (h *Handler) qiniuLegacyRootPrefix() string {
	return storage.NormalizeRootPrefix(qiniuLegacyRootPrefix)
}

func (h *Handler) qiniuAllowedRootPrefixes() []string {
	current := h.qiniuRootPrefix()
	legacy := h.qiniuLegacyRootPrefix()
	if current == legacy {
		return []string{current}
	}
	return []string{current, legacy}
}

func (h *Handler) hasQiniuAllowedRootPrefix(key string) bool {
	for _, prefix := range h.qiniuAllowedRootPrefixes() {
		if storage.HasRootPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func (h *Handler) qiniuTrashPrefix() string {
	return storage.TrashPrefix(h.cfg.QiniuRootPrefix)
}

func (h *Handler) qiniuCollectionsPrefix() string {
	return storage.CollectionsPrefix(h.cfg.QiniuRootPrefix)
}

func (h *Handler) qiniuVideoImagePrefix() string {
	return storage.VideoImagePrefix(h.cfg.QiniuRootPrefix)
}

func (h *Handler) qiniuUserVideoPrefixForRoot(userID uint64, rootPrefix string) string {
	root := strings.TrimSuffix(storage.NormalizeRootPrefix(rootPrefix), "/")
	return path.Join(root, "user-video", strconv.FormatUint(userID, 10)) + "/"
}

func (h *Handler) qiniuUserVideoPrefix(userID uint64) string {
	return h.qiniuUserVideoPrefixForRoot(userID, h.cfg.QiniuRootPrefix)
}

func (h *Handler) qiniuLegacyUserVideoPrefix(userID uint64) string {
	return h.qiniuUserVideoPrefixForRoot(userID, qiniuLegacyRootPrefix)
}
