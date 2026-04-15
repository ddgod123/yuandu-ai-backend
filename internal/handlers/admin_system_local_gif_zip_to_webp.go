package handlers

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
)

const (
	adminLocalGIFZipToWebPMaxUploadBytes       int64 = 512 << 20
	adminLocalGIFZipToWebPMaxEntries                 = 3000
	adminLocalGIFZipToWebPMaxSingleEntryBytes  int64 = 150 << 20
	adminLocalGIFZipToWebPMaxUncompressedBytes int64 = 2 << 30
	adminLocalGIFZipToWebPMultipartMemory      int64 = 64 << 20
	adminLocalGIFZipToWebPPerFileTimeout             = 2 * time.Minute
	adminLocalGIFZipToWebPDetectEncoderTimeout       = 10 * time.Second
	adminLocalGIFZipToWebPMaxFailureDetails          = 30
)

type AdminLocalGIFZipToWebPResponse struct {
	InputFile      string   `json:"input_file"`
	DesktopDir     string   `json:"desktop_dir"`
	OutputFile     string   `json:"output_file"`
	OutputPath     string   `json:"output_path"`
	TotalFiles     int      `json:"total_files"`
	ConvertedFiles int      `json:"converted_files"`
	SkippedFiles   int      `json:"skipped_files"`
	FailedFiles    int      `json:"failed_files"`
	Failures       []string `json:"failures,omitempty"`
	Warnings       []string `json:"warnings,omitempty"`
	ElapsedMS      int64    `json:"elapsed_ms"`
}

// ConvertLocalGIFZipToWebP godoc
// @Summary Convert uploaded GIF zip into WebP zip and save to local desktop (admin)
// @Tags admin
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "zip file containing gif files"
// @Success 200 {object} AdminLocalGIFZipToWebPResponse
// @Router /api/admin/system/local/gif-zip-to-webp [post]
func (h *Handler) ConvertLocalGIFZipToWebP(c *gin.Context) {
	startedAt := time.Now()

	if _, err := exec.LookPath("ffmpeg"); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未检测到 ffmpeg，请先在本机安装 ffmpeg"})
		return
	}
	webpCodec := detectAdminLocalWebPEncoder(c.Request.Context())

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, adminLocalGIFZipToWebPMaxUploadBytes+(10<<20))
	if err := c.Request.ParseMultipartForm(adminLocalGIFZipToWebPMultipartMemory); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "request body too large") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "zip 文件过大，最大支持 512MB"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "解析上传文件失败"})
		return
	}
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请上传 zip 文件"})
		return
	}
	if fileHeader.Size <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "上传文件为空"})
		return
	}
	if fileHeader.Size > adminLocalGIFZipToWebPMaxUploadBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "zip 文件过大，最大支持 512MB"})
		return
	}
	if !strings.HasSuffix(strings.ToLower(strings.TrimSpace(fileHeader.Filename)), ".zip") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "仅支持 .zip 文件"})
		return
	}

	uploaded, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "打开上传文件失败"})
		return
	}
	defer uploaded.Close()

	tempZipFile, err := os.CreateTemp("", "admin-gif-zip-*.zip")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建临时文件失败"})
		return
	}
	defer os.Remove(tempZipFile.Name())

	copied, err := io.Copy(tempZipFile, io.LimitReader(uploaded, adminLocalGIFZipToWebPMaxUploadBytes+1))
	if err != nil {
		_ = tempZipFile.Close()
		c.JSON(http.StatusBadRequest, gin.H{"error": "保存上传文件失败"})
		return
	}
	if copied > adminLocalGIFZipToWebPMaxUploadBytes {
		_ = tempZipFile.Close()
		c.JSON(http.StatusBadRequest, gin.H{"error": "zip 文件过大，最大支持 512MB"})
		return
	}
	if err := tempZipFile.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存上传文件失败"})
		return
	}

	zipReader, err := zip.OpenReader(tempZipFile.Name())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "zip 文件损坏或格式不正确"})
		return
	}
	defer zipReader.Close()

	desktopDir, err := resolveAdminLocalDesktopDir()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	outputFileName := buildAdminLocalWebPZipName(fileHeader.Filename)
	outputPath, err := buildAdminLocalUniqueOutputPath(desktopDir, outputFileName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建输出文件路径失败"})
		return
	}

	workDir, err := os.MkdirTemp("", "admin-gif-webp-*")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建工作目录失败"})
		return
	}
	defer os.RemoveAll(workDir)

	outputFile, err := os.Create(outputPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建输出 zip 失败"})
		return
	}
	zipWriter := zip.NewWriter(outputFile)
	outputClosed := false
	outputCommitted := false
	defer func() {
		if !outputClosed {
			_ = zipWriter.Close()
			_ = outputFile.Close()
		}
		if !outputCommitted {
			_ = os.Remove(outputPath)
		}
	}()

	totalFiles := 0
	convertedFiles := 0
	skippedFiles := 0
	failedFiles := 0
	failures := make([]string, 0, 8)
	seenOutputEntries := map[string]struct{}{}
	var totalUncompressed int64

	appendFailure := func(message string) {
		failedFiles++
		if len(failures) < adminLocalGIFZipToWebPMaxFailureDetails {
			failures = append(failures, message)
		}
	}

	for _, file := range zipReader.File {
		if file == nil || file.FileInfo().IsDir() {
			continue
		}
		totalFiles++
		if totalFiles > adminLocalGIFZipToWebPMaxEntries {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("zip 文件数量过多，最多支持 %d 个文件", adminLocalGIFZipToWebPMaxEntries)})
			return
		}

		normalizedPath, normalizeErr := normalizeAdminLocalZipEntryPath(file.Name)
		if normalizeErr != nil {
			appendFailure(fmt.Sprintf("%s: %v", file.Name, normalizeErr))
			continue
		}
		if strings.Contains(strings.ToLower(normalizedPath), "__macosx/") || strings.EqualFold(path.Base(normalizedPath), ".ds_store") {
			skippedFiles++
			continue
		}
		if strings.ToLower(path.Ext(normalizedPath)) != ".gif" {
			skippedFiles++
			continue
		}

		if file.UncompressedSize64 > uint64(adminLocalGIFZipToWebPMaxSingleEntryBytes) {
			appendFailure(fmt.Sprintf("%s: 文件过大（单文件最大支持 150MB）", normalizedPath))
			continue
		}
		inputGIFPath, copiedBytes, writeErr := writeAdminLocalGIFTempFile(file, workDir, adminLocalGIFZipToWebPMaxSingleEntryBytes)
		if writeErr != nil {
			appendFailure(fmt.Sprintf("%s: %v", normalizedPath, writeErr))
			continue
		}
		totalUncompressed += copiedBytes
		if totalUncompressed > adminLocalGIFZipToWebPMaxUncompressedBytes {
			_ = os.Remove(inputGIFPath)
			c.JSON(http.StatusBadRequest, gin.H{"error": "zip 解压总大小超过限制（最大 2GB）"})
			return
		}

		outputWebPPath := strings.TrimSuffix(inputGIFPath, filepath.Ext(inputGIFPath)) + ".webp"
		convertErr := convertAdminLocalGIFToWebP(c.Request.Context(), inputGIFPath, outputWebPPath, webpCodec)
		_ = os.Remove(inputGIFPath)
		if convertErr != nil {
			_ = os.Remove(outputWebPPath)
			appendFailure(fmt.Sprintf("%s: %v", normalizedPath, convertErr))
			continue
		}

		outputEntry := strings.TrimSuffix(normalizedPath, path.Ext(normalizedPath)) + ".webp"
		outputEntry = ensureAdminLocalUniqueZipEntryName(outputEntry, seenOutputEntries)
		if err := copyAdminLocalFileToZip(zipWriter, outputWebPPath, outputEntry, file.Modified); err != nil {
			_ = os.Remove(outputWebPPath)
			appendFailure(fmt.Sprintf("%s: 写入输出 zip 失败", normalizedPath))
			continue
		}
		_ = os.Remove(outputWebPPath)
		convertedFiles++
	}

	if convertedFiles == 0 {
		message := "zip 中没有可转换的 GIF 文件"
		if failedFiles > 0 {
			message = "所有 GIF 文件转换失败，请检查文件内容或 ffmpeg 编码能力"
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"error":           message,
			"total_files":     totalFiles,
			"converted_files": convertedFiles,
			"skipped_files":   skippedFiles,
			"failed_files":    failedFiles,
			"failures":        failures,
		})
		return
	}

	if err := zipWriter.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "关闭输出 zip 失败"})
		return
	}
	if err := outputFile.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "写入输出 zip 失败"})
		return
	}
	outputClosed = true
	outputCommitted = true

	warnings := make([]string, 0, 4)
	if webpCodec != "libwebp_anim" {
		warnings = append(warnings, "当前 ffmpeg 未启用 libwebp_anim，部分动图可能降级为静态 WebP")
	}
	if skippedFiles > 0 {
		warnings = append(warnings, fmt.Sprintf("已跳过 %d 个非 GIF 文件", skippedFiles))
	}
	if failedFiles > len(failures) {
		warnings = append(warnings, fmt.Sprintf("另有 %d 个失败文件未在列表中展示", failedFiles-len(failures)))
	}
	if failedFiles > 0 {
		warnings = append(warnings, "部分文件转换失败，详情见 failures")
	}

	c.JSON(http.StatusOK, AdminLocalGIFZipToWebPResponse{
		InputFile:      fileHeader.Filename,
		DesktopDir:     desktopDir,
		OutputFile:     filepath.Base(outputPath),
		OutputPath:     outputPath,
		TotalFiles:     totalFiles,
		ConvertedFiles: convertedFiles,
		SkippedFiles:   skippedFiles,
		FailedFiles:    failedFiles,
		Failures:       failures,
		Warnings:       warnings,
		ElapsedMS:      time.Since(startedAt).Milliseconds(),
	})
}

func resolveAdminLocalDesktopDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", errors.New("无法定位当前用户目录，无法保存到桌面")
	}

	candidates := []string{
		filepath.Join(home, "Desktop"),
		filepath.Join(home, "桌面"),
	}
	for _, candidate := range candidates {
		info, statErr := os.Stat(candidate)
		if statErr == nil && info.IsDir() {
			return candidate, nil
		}
	}

	fallback := filepath.Join(home, "Desktop")
	if mkErr := os.MkdirAll(fallback, 0o755); mkErr != nil {
		return "", fmt.Errorf("未找到桌面目录，且创建失败: %w", mkErr)
	}
	return fallback, nil
}

func buildAdminLocalWebPZipName(inputFilename string) string {
	base := sanitizeAdminLocalFileBase(inputFilename)
	return fmt.Sprintf("%s_webp_%s.zip", base, time.Now().Format("20060102_150405"))
}

func sanitizeAdminLocalFileBase(raw string) string {
	base := strings.TrimSpace(filepath.Base(raw))
	if base == "" {
		return "emoji_bundle"
	}
	base = strings.TrimSuffix(base, filepath.Ext(base))
	base = strings.TrimSpace(base)
	if base == "" {
		return "emoji_bundle"
	}

	var b strings.Builder
	for _, r := range base {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
		case r == '-', r == '_':
			b.WriteRune(r)
		case unicode.IsSpace(r):
			b.WriteRune('_')
		}
		if b.Len() >= 80 {
			break
		}
	}
	clean := strings.Trim(b.String(), "_-")
	if clean == "" {
		return "emoji_bundle"
	}
	return clean
}

func buildAdminLocalUniqueOutputPath(dir, filename string) (string, error) {
	name := strings.TrimSpace(filename)
	if name == "" {
		name = buildAdminLocalWebPZipName("emoji_bundle.zip")
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	candidate := filepath.Join(dir, name)

	for i := 0; i < 1000; i++ {
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		}
		candidate = filepath.Join(dir, fmt.Sprintf("%s_%d%s", base, i+1, ext))
	}

	candidate = filepath.Join(dir, fmt.Sprintf("%s_%d%s", base, time.Now().UnixNano(), ext))
	if _, err := os.Stat(candidate); os.IsNotExist(err) {
		return candidate, nil
	}
	return "", errors.New("输出文件命名冲突，请重试")
}

func normalizeAdminLocalZipEntryPath(raw string) (string, error) {
	name := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if name == "" {
		return "", errors.New("文件路径为空")
	}
	if strings.ContainsRune(name, '\x00') {
		return "", errors.New("文件名包含非法字符")
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return "", errors.New("检测到非法路径")
		}
	}
	name = strings.TrimLeft(name, "/")
	name = strings.TrimPrefix(name, "./")
	name = path.Clean(name)
	if name == "." || name == "" || strings.HasPrefix(name, "../") {
		return "", errors.New("检测到非法路径")
	}
	return name, nil
}

func ensureAdminLocalUniqueZipEntryName(entry string, seen map[string]struct{}) string {
	name := strings.TrimSpace(strings.ReplaceAll(entry, "\\", "/"))
	if name == "" {
		name = fmt.Sprintf("converted_%d.webp", time.Now().UnixNano())
	}
	if _, exists := seen[name]; !exists {
		seen[name] = struct{}{}
		return name
	}
	ext := path.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 2; i <= 9999; i++ {
		candidate := fmt.Sprintf("%s_%d%s", base, i, ext)
		if _, exists := seen[candidate]; !exists {
			seen[candidate] = struct{}{}
			return candidate
		}
	}
	fallback := fmt.Sprintf("%s_%d%s", base, time.Now().UnixNano(), ext)
	seen[fallback] = struct{}{}
	return fallback
}

func writeAdminLocalGIFTempFile(file *zip.File, workDir string, maxBytes int64) (string, int64, error) {
	rc, err := file.Open()
	if err != nil {
		return "", 0, errors.New("读取 zip 文件失败")
	}
	defer rc.Close()

	tmpFile, err := os.CreateTemp(workDir, "gif-src-*.gif")
	if err != nil {
		return "", 0, errors.New("创建临时文件失败")
	}

	copied, copyErr := io.Copy(tmpFile, io.LimitReader(rc, maxBytes+1))
	closeErr := tmpFile.Close()
	if copyErr != nil {
		_ = os.Remove(tmpFile.Name())
		return "", 0, errors.New("写入临时文件失败")
	}
	if closeErr != nil {
		_ = os.Remove(tmpFile.Name())
		return "", 0, errors.New("写入临时文件失败")
	}
	if copied == 0 {
		_ = os.Remove(tmpFile.Name())
		return "", 0, errors.New("文件为空")
	}
	if copied > maxBytes {
		_ = os.Remove(tmpFile.Name())
		return "", 0, errors.New("文件过大（单文件最大支持 150MB）")
	}
	return tmpFile.Name(), copied, nil
}

func detectAdminLocalWebPEncoder(parent context.Context) string {
	ctx, cancel := context.WithTimeout(parent, adminLocalGIFZipToWebPDetectEncoderTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-encoders")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "libwebp"
	}
	if strings.Contains(strings.ToLower(out.String()), "libwebp_anim") {
		return "libwebp_anim"
	}
	return "libwebp"
}

func convertAdminLocalGIFToWebP(parent context.Context, inputGIFPath, outputWebPPath, codec string) error {
	ctx, cancel := context.WithTimeout(parent, adminLocalGIFZipToWebPPerFileTimeout)
	defer cancel()

	if strings.TrimSpace(codec) == "" {
		codec = "libwebp_anim"
	}

	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-i", inputGIFPath,
		"-c:v", codec,
		"-q:v", "80",
		"-compression_level", "6",
		"-loop", "0",
		"-an",
		"-fps_mode", "passthrough",
		"-pix_fmt", "yuva420p",
		outputWebPPath,
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return errors.New("转换超时")
		}
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("ffmpeg 转换失败：%s", message)
	}

	info, err := os.Stat(outputWebPPath)
	if err != nil || info.Size() <= 0 {
		return errors.New("输出文件为空")
	}
	return nil
}

func copyAdminLocalFileToZip(zipWriter *zip.Writer, sourcePath, entryName string, modified time.Time) error {
	src, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer src.Close()

	hdr := &zip.FileHeader{
		Name: strings.ReplaceAll(strings.TrimSpace(entryName), "\\", "/"),
		// WebP 本身已是压缩格式，zip 继续 Deflate 的收益很低但会明显拖慢整体速度。
		// 本地工具优先转换速度，因此这里使用 Store。
		Method: zip.Store,
	}
	if !modified.IsZero() {
		hdr.SetModTime(modified)
	} else {
		hdr.SetModTime(time.Now())
	}

	writer, err := zipWriter.CreateHeader(hdr)
	if err != nil {
		return err
	}
	_, err = io.Copy(writer, src)
	return err
}
