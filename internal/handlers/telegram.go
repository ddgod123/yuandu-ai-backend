package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/net/proxy"
)

type TelegramDownloadRequest struct {
	Link     string `json:"link"`
	PackName string `json:"pack_name"`
}

type TelegramDownloadedFile struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Emoji        string `json:"emoji"`
	FilePath     string `json:"file_path"`
	LocalPath    string `json:"local_path"`
	Type         string `json:"type"`
}

type TelegramDownloadResponse struct {
	PackName string                   `json:"pack_name"`
	Title    string                   `json:"title"`
	Total    int                      `json:"total"`
	Saved    int                      `json:"saved"`
	Failed   int                      `json:"failed"`
	BaseDir  string                   `json:"base_dir"`
	Files    []TelegramDownloadedFile `json:"files"`
}

type telegramStickerSetResp struct {
	OK          bool               `json:"ok"`
	Result      telegramStickerSet `json:"result"`
	Description string             `json:"description"`
}

type telegramStickerSet struct {
	Name       string            `json:"name"`
	Title      string            `json:"title"`
	IsAnimated bool              `json:"is_animated"`
	IsVideo    bool              `json:"is_video"`
	Stickers   []telegramSticker `json:"stickers"`
}

type telegramSticker struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Emoji        string `json:"emoji"`
	IsAnimated   bool   `json:"is_animated"`
	IsVideo      bool   `json:"is_video"`
}

type telegramFileResp struct {
	OK          bool         `json:"ok"`
	Result      telegramFile `json:"result"`
	Description string       `json:"description"`
}

type telegramFile struct {
	FileID   string `json:"file_id"`
	FilePath string `json:"file_path"`
	FileSize int64  `json:"file_size"`
}

// DownloadTelegram godoc
// @Summary Download Telegram sticker pack
// @Tags admin
// @Accept json
// @Produce json
// @Param body body TelegramDownloadRequest true "telegram download"
// @Success 200 {object} TelegramDownloadResponse
// @Router /api/admin/telegram/download [post]
func (h *Handler) DownloadTelegram(c *gin.Context) {
	if h.cfg.TelegramBotToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "TELEGRAM_BOT_TOKEN not configured"})
		return
	}

	var req TelegramDownloadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	packName := strings.TrimSpace(req.PackName)
	if packName == "" {
		packName = parseTelegramPackName(req.Link)
	}
	if packName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid telegram link or pack_name"})
		return
	}

	set, err := h.fetchStickerSet(packName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	baseDir := filepath.Join(h.cfg.TelegramDownloadDir, "telegram", packName)
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create base dir"})
		return
	}

	resp := TelegramDownloadResponse{
		PackName: packName,
		Title:    set.Title,
		Total:    len(set.Stickers),
		BaseDir:  baseDir,
	}

	client, err := telegramHTTPClient(h.cfg.TelegramProxy)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	for _, sticker := range set.Stickers {
		file, err := h.fetchFile(client, sticker.FileID)
		if err != nil {
			resp.Failed++
			continue
		}
		ext := strings.ToLower(path.Ext(file.FilePath))
		fileType := "static"
		if sticker.IsVideo || ext == ".webm" {
			fileType = "video"
		} else if sticker.IsAnimated || ext == ".tgs" {
			fileType = "animated"
		}

		folder := filepath.Join(baseDir, fileType)
		if err := os.MkdirAll(folder, 0755); err != nil {
			resp.Failed++
			continue
		}

		filename := sticker.FileUniqueID
		if filename == "" {
			filename = sticker.FileID
		}
		localPath := filepath.Join(folder, filename+ext)
		if _, err := os.Stat(localPath); err == nil {
			resp.Saved++
			resp.Files = append(resp.Files, TelegramDownloadedFile{
				FileID:       sticker.FileID,
				FileUniqueID: sticker.FileUniqueID,
				Emoji:        sticker.Emoji,
				FilePath:     file.FilePath,
				LocalPath:    localPath,
				Type:         fileType,
			})
			continue
		}

		downloadURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", h.cfg.TelegramBotToken, file.FilePath)
		if err := downloadToFile(client, downloadURL, localPath); err != nil {
			resp.Failed++
			continue
		}

		resp.Saved++
		resp.Files = append(resp.Files, TelegramDownloadedFile{
			FileID:       sticker.FileID,
			FileUniqueID: sticker.FileUniqueID,
			Emoji:        sticker.Emoji,
			FilePath:     file.FilePath,
			LocalPath:    localPath,
			Type:         fileType,
		})
	}

	_ = writeTelegramMeta(baseDir, set, resp.Files)

	c.JSON(http.StatusOK, resp)
}

func (h *Handler) fetchStickerSet(packName string) (telegramStickerSet, error) {
	client, err := telegramHTTPClient(h.cfg.TelegramProxy)
	if err != nil {
		return telegramStickerSet{}, err
	}
	return h.fetchStickerSetWithClient(client, packName)
}

func (h *Handler) fetchStickerSetWithClient(client *http.Client, packName string) (telegramStickerSet, error) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getStickerSet?name=%s", h.cfg.TelegramBotToken, packName)
	resp, err := client.Get(url)
	if err != nil {
		return telegramStickerSet{}, err
	}
	defer resp.Body.Close()
	var data telegramStickerSetResp
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return telegramStickerSet{}, err
	}
	if !data.OK {
		return telegramStickerSet{}, errors.New(data.Description)
	}
	return data.Result, nil
}

func (h *Handler) fetchFile(client *http.Client, fileID string) (telegramFile, error) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getFile?file_id=%s", h.cfg.TelegramBotToken, fileID)
	resp, err := client.Get(url)
	if err != nil {
		return telegramFile{}, err
	}
	defer resp.Body.Close()
	var data telegramFileResp
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return telegramFile{}, err
	}
	if !data.OK {
		return telegramFile{}, errors.New(data.Description)
	}
	return data.Result, nil
}

func downloadToFile(client *http.Client, url, dest string) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("download failed: %s", resp.Status)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func telegramHTTPClient(proxyRaw string) (*http.Client, error) {
	transport := &http.Transport{}
	if proxyRaw != "" {
		proxyURL, err := url.Parse(proxyRaw)
		if err != nil {
			return nil, fmt.Errorf("invalid TELEGRAM_PROXY: %w", err)
		}
		switch strings.ToLower(proxyURL.Scheme) {
		case "socks5", "socks5h":
			dialer, err := proxy.FromURL(proxyURL, proxy.Direct)
			if err != nil {
				return nil, fmt.Errorf("invalid socks5 proxy: %w", err)
			}
			transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
			}
		case "http", "https":
			transport.Proxy = http.ProxyURL(proxyURL)
		default:
			return nil, fmt.Errorf("unsupported proxy scheme: %s", proxyURL.Scheme)
		}
	}
	return &http.Client{Timeout: 60 * time.Second, Transport: transport}, nil
}

func parseTelegramPackName(link string) string {
	link = strings.TrimSpace(link)
	if link == "" {
		return ""
	}
	link = strings.TrimPrefix(link, "https://")
	link = strings.TrimPrefix(link, "http://")
	if strings.Contains(link, "t.me/addstickers/") {
		parts := strings.Split(link, "t.me/addstickers/")
		if len(parts) > 1 {
			return strings.Split(parts[1], "?")[0]
		}
	}
	if strings.Contains(link, "telegram.me/addstickers/") {
		parts := strings.Split(link, "telegram.me/addstickers/")
		if len(parts) > 1 {
			return strings.Split(parts[1], "?")[0]
		}
	}
	if strings.Contains(link, "addstickers/") {
		parts := strings.Split(link, "addstickers/")
		if len(parts) > 1 {
			return strings.Split(parts[1], "?")[0]
		}
	}
	return ""
}

func writeTelegramMeta(baseDir string, set telegramStickerSet, files []TelegramDownloadedFile) error {
	meta := map[string]interface{}{
		"name":        set.Name,
		"title":       set.Title,
		"is_animated": set.IsAnimated,
		"is_video":    set.IsVideo,
		"count":       len(set.Stickers),
		"files":       files,
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(baseDir, "meta.json"), data, 0644)
}
