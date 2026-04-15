package handlers

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const adminLocalGIFZipToWebPJobTTL = 6 * time.Hour

type AdminLocalGIFZipToWebPJobStartResponse struct {
	JobID                string   `json:"job_id"`
	TargetFormat         string   `json:"target_format"`
	Status               string   `json:"status"`
	InputFile            string   `json:"input_file"`
	TotalEntries         int      `json:"total_entries"`
	TotalTargetFiles     int      `json:"total_target_files"`
	ProcessedTargetFiles int      `json:"processed_target_files"`
	ConvertedFiles       int      `json:"converted_files"`
	SkippedFiles         int      `json:"skipped_files"`
	FailedFiles          int      `json:"failed_files"`
	Failures             []string `json:"failures,omitempty"`
	Warnings             []string `json:"warnings,omitempty"`
	CreatedAt            string   `json:"created_at"`
}

type AdminLocalGIFZipToWebPJobStatusResponse struct {
	JobID                string                          `json:"job_id"`
	TargetFormat         string                          `json:"target_format"`
	Status               string                          `json:"status"`
	InputFile            string                          `json:"input_file"`
	TotalEntries         int                             `json:"total_entries"`
	TotalTargetFiles     int                             `json:"total_target_files"`
	ProcessedTargetFiles int                             `json:"processed_target_files"`
	ProgressPercent      float64                         `json:"progress_percent"`
	ConvertedFiles       int                             `json:"converted_files"`
	SkippedFiles         int                             `json:"skipped_files"`
	FailedFiles          int                             `json:"failed_files"`
	OutputPath           string                          `json:"output_path,omitempty"`
	OutputFile           string                          `json:"output_file,omitempty"`
	DesktopDir           string                          `json:"desktop_dir,omitempty"`
	Error                string                          `json:"error,omitempty"`
	Failures             []string                        `json:"failures,omitempty"`
	Warnings             []string                        `json:"warnings,omitempty"`
	ElapsedMS            int64                           `json:"elapsed_ms"`
	CreatedAt            string                          `json:"created_at"`
	StartedAt            string                          `json:"started_at,omitempty"`
	UpdatedAt            string                          `json:"updated_at"`
	FinishedAt           string                          `json:"finished_at,omitempty"`
	Result               *AdminLocalGIFZipToWebPResponse `json:"result,omitempty"`
}

type adminLocalGIFZipToWebPJob struct {
	ID                   string
	TargetFormat         string
	Status               string
	InputFile            string
	TempZipPath          string
	Encoder              string
	WebPCodec            string
	TotalEntries         int
	TotalTargetFiles     int
	ProcessedTargetFiles int
	ConvertedFiles       int
	SkippedFiles         int
	FailedFiles          int
	Failures             []string
	Warnings             []string
	OutputPath           string
	OutputFile           string
	DesktopDir           string
	ErrorMessage         string
	ElapsedMS            int64
	CreatedAt            time.Time
	StartedAt            *time.Time
	UpdatedAt            time.Time
	FinishedAt           *time.Time
}

type adminLocalGIFZipToWebPJobStore struct {
	mu   sync.RWMutex
	jobs map[string]*adminLocalGIFZipToWebPJob
}

var adminLocalGIFZipToWebPJobs = &adminLocalGIFZipToWebPJobStore{
	jobs: map[string]*adminLocalGIFZipToWebPJob{},
}

func cloneAdminLocalGIFZipToWebPJob(src *adminLocalGIFZipToWebPJob) adminLocalGIFZipToWebPJob {
	if src == nil {
		return adminLocalGIFZipToWebPJob{}
	}
	out := *src
	if src.Failures != nil {
		out.Failures = append([]string{}, src.Failures...)
	}
	if src.Warnings != nil {
		out.Warnings = append([]string{}, src.Warnings...)
	}
	if src.StartedAt != nil {
		t := *src.StartedAt
		out.StartedAt = &t
	}
	if src.FinishedAt != nil {
		t := *src.FinishedAt
		out.FinishedAt = &t
	}
	return out
}

func (s *adminLocalGIFZipToWebPJobStore) cleanupLocked(now time.Time) {
	if len(s.jobs) == 0 {
		return
	}
	for id, job := range s.jobs {
		if job == nil {
			delete(s.jobs, id)
			continue
		}
		reference := job.UpdatedAt
		if reference.IsZero() {
			reference = job.CreatedAt
		}
		if !reference.IsZero() && now.Sub(reference) > adminLocalGIFZipToWebPJobTTL {
			if strings.TrimSpace(job.TempZipPath) != "" {
				_ = os.Remove(job.TempZipPath)
			}
			delete(s.jobs, id)
		}
	}
}

func (s *adminLocalGIFZipToWebPJobStore) create(job *adminLocalGIFZipToWebPJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.cleanupLocked(now)
	s.jobs[job.ID] = job
}

func (s *adminLocalGIFZipToWebPJobStore) get(id string) (adminLocalGIFZipToWebPJob, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.cleanupLocked(now)
	job, ok := s.jobs[id]
	if !ok || job == nil {
		return adminLocalGIFZipToWebPJob{}, false
	}
	return cloneAdminLocalGIFZipToWebPJob(job), true
}

func (s *adminLocalGIFZipToWebPJobStore) update(id string, mut func(job *adminLocalGIFZipToWebPJob)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.cleanupLocked(now)
	job, ok := s.jobs[id]
	if !ok || job == nil {
		return false
	}
	mut(job)
	job.UpdatedAt = time.Now()
	return true
}

func newAdminLocalGIFZipToWebPJobID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("gifwebp-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("gifwebp-%d-%s", time.Now().Unix(), hex.EncodeToString(buf))
}

type adminLocalGIFZipEntryCheck struct {
	NormalizedPath string
	Candidate      bool
	Skip           bool
	Failure        string
}

func checkAdminLocalGIFZipEntry(file *zip.File) adminLocalGIFZipEntryCheck {
	if file == nil || file.FileInfo().IsDir() {
		return adminLocalGIFZipEntryCheck{Skip: true}
	}
	normalizedPath, normalizeErr := normalizeAdminLocalZipEntryPath(file.Name)
	if normalizeErr != nil {
		return adminLocalGIFZipEntryCheck{Failure: fmt.Sprintf("%s: %v", file.Name, normalizeErr)}
	}
	if strings.Contains(strings.ToLower(normalizedPath), "__macosx/") || strings.EqualFold(path.Base(normalizedPath), ".ds_store") {
		return adminLocalGIFZipEntryCheck{NormalizedPath: normalizedPath, Skip: true}
	}
	if strings.ToLower(path.Ext(normalizedPath)) != ".gif" {
		return adminLocalGIFZipEntryCheck{NormalizedPath: normalizedPath, Skip: true}
	}
	if file.UncompressedSize64 > uint64(adminLocalGIFZipToWebPMaxSingleEntryBytes) {
		return adminLocalGIFZipEntryCheck{NormalizedPath: normalizedPath, Failure: fmt.Sprintf("%s: 文件过大（单文件最大支持 150MB）", normalizedPath)}
	}
	return adminLocalGIFZipEntryCheck{NormalizedPath: normalizedPath, Candidate: true}
}

func preScanAdminLocalGIFZip(zipPath string) (totalEntries, totalTargets, skipped, failed int, failures []string, err error) {
	reader, openErr := zip.OpenReader(zipPath)
	if openErr != nil {
		return 0, 0, 0, 0, nil, errors.New("zip 文件损坏或格式不正确")
	}
	defer reader.Close()

	failures = make([]string, 0, 8)
	appendFailure := func(message string) {
		failed++
		if len(failures) < adminLocalGIFZipToWebPMaxFailureDetails {
			failures = append(failures, message)
		}
	}

	var totalUncompressed int64
	for _, file := range reader.File {
		if file == nil || file.FileInfo().IsDir() {
			continue
		}
		totalEntries++
		if totalEntries > adminLocalGIFZipToWebPMaxEntries {
			return 0, 0, 0, 0, nil, fmt.Errorf("zip 文件数量过多，最多支持 %d 个文件", adminLocalGIFZipToWebPMaxEntries)
		}

		checked := checkAdminLocalGIFZipEntry(file)
		if checked.Skip {
			skipped++
			continue
		}
		if checked.Failure != "" {
			appendFailure(checked.Failure)
			continue
		}
		if !checked.Candidate {
			continue
		}
		totalTargets++
		totalUncompressed += int64(file.UncompressedSize64)
		if totalUncompressed > adminLocalGIFZipToWebPMaxUncompressedBytes {
			return 0, 0, 0, 0, nil, errors.New("zip 解压总大小超过限制（最大 2GB）")
		}
	}

	return totalEntries, totalTargets, skipped, failed, failures, nil
}

func buildAdminLocalGIFZipToWebPJobStatus(job adminLocalGIFZipToWebPJob) AdminLocalGIFZipToWebPJobStatusResponse {
	progress := 0.0
	if job.TotalTargetFiles > 0 {
		progress = (float64(job.ProcessedTargetFiles) / float64(job.TotalTargetFiles)) * 100
	}
	if job.Status == "completed" {
		progress = 100
	}
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}
	elapsedMS := job.ElapsedMS
	if elapsedMS <= 0 && (job.Status == "queued" || job.Status == "processing") {
		start := job.CreatedAt
		if job.StartedAt != nil && !job.StartedAt.IsZero() {
			start = *job.StartedAt
		}
		if !start.IsZero() {
			elapsedMS = time.Since(start).Milliseconds()
		}
	}

	resp := AdminLocalGIFZipToWebPJobStatusResponse{
		JobID:                job.ID,
		TargetFormat:         strings.TrimSpace(job.TargetFormat),
		Status:               job.Status,
		InputFile:            job.InputFile,
		TotalEntries:         job.TotalEntries,
		TotalTargetFiles:     job.TotalTargetFiles,
		ProcessedTargetFiles: job.ProcessedTargetFiles,
		ProgressPercent:      progress,
		ConvertedFiles:       job.ConvertedFiles,
		SkippedFiles:         job.SkippedFiles,
		FailedFiles:          job.FailedFiles,
		OutputPath:           job.OutputPath,
		OutputFile:           job.OutputFile,
		DesktopDir:           job.DesktopDir,
		Error:                job.ErrorMessage,
		Failures:             append([]string{}, job.Failures...),
		Warnings:             append([]string{}, job.Warnings...),
		ElapsedMS:            elapsedMS,
		CreatedAt:            job.CreatedAt.Format(time.RFC3339),
		UpdatedAt:            job.UpdatedAt.Format(time.RFC3339),
	}
	if job.StartedAt != nil {
		resp.StartedAt = job.StartedAt.Format(time.RFC3339)
	}
	if job.FinishedAt != nil {
		resp.FinishedAt = job.FinishedAt.Format(time.RFC3339)
	}
	if job.Status == "completed" {
		resp.Result = &AdminLocalGIFZipToWebPResponse{
			InputFile:      job.InputFile,
			DesktopDir:     job.DesktopDir,
			OutputFile:     job.OutputFile,
			OutputPath:     job.OutputPath,
			TotalFiles:     job.TotalEntries,
			ConvertedFiles: job.ConvertedFiles,
			SkippedFiles:   job.SkippedFiles,
			FailedFiles:    job.FailedFiles,
			Failures:       append([]string{}, job.Failures...),
			Warnings:       append([]string{}, job.Warnings...),
			ElapsedMS:      elapsedMS,
		}
	}
	return resp
}

func (h *Handler) failAdminLocalGIFZipToWebPJob(jobID, message string) {
	adminLocalGIFZipToWebPJobs.update(jobID, func(job *adminLocalGIFZipToWebPJob) {
		now := time.Now()
		job.Status = "failed"
		job.ErrorMessage = strings.TrimSpace(message)
		if job.ErrorMessage == "" {
			job.ErrorMessage = "转换失败"
		}
		job.ElapsedMS = now.Sub(job.CreatedAt).Milliseconds()
		job.FinishedAt = &now
	})
}

func (h *Handler) runAdminLocalGIFZipToWebPJob(jobID string) {
	jobSnapshot, ok := adminLocalGIFZipToWebPJobs.get(jobID)
	if !ok {
		return
	}
	startedAt := time.Now()
	adminLocalGIFZipToWebPJobs.update(jobID, func(job *adminLocalGIFZipToWebPJob) {
		job.Status = "processing"
		job.StartedAt = &startedAt
	})

	defer func() {
		if strings.TrimSpace(jobSnapshot.TempZipPath) != "" {
			_ = os.Remove(jobSnapshot.TempZipPath)
		}
	}()

	desktopDir, err := resolveAdminLocalDesktopDir()
	if err != nil {
		h.failAdminLocalGIFZipToWebPJob(jobID, err.Error())
		return
	}
	outputFileName := buildAdminLocalWebPZipName(jobSnapshot.InputFile)
	outputPath, err := buildAdminLocalUniqueOutputPath(desktopDir, outputFileName)
	if err != nil {
		h.failAdminLocalGIFZipToWebPJob(jobID, "创建输出文件路径失败")
		return
	}

	workDir, err := os.MkdirTemp("", "admin-gif-webp-job-*")
	if err != nil {
		h.failAdminLocalGIFZipToWebPJob(jobID, "创建工作目录失败")
		return
	}
	defer os.RemoveAll(workDir)

	zipReader, err := zip.OpenReader(jobSnapshot.TempZipPath)
	if err != nil {
		h.failAdminLocalGIFZipToWebPJob(jobID, "zip 文件损坏或格式不正确")
		return
	}
	defer zipReader.Close()

	outputFile, err := os.Create(outputPath)
	if err != nil {
		h.failAdminLocalGIFZipToWebPJob(jobID, "创建输出 zip 失败")
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

	seenOutputEntries := map[string]struct{}{}
	var totalUncompressed int64

	for _, file := range zipReader.File {
		if file == nil || file.FileInfo().IsDir() {
			continue
		}
		checked := checkAdminLocalGIFZipEntry(file)
		if !checked.Candidate {
			continue
		}

		inputGIFPath, copiedBytes, writeErr := writeAdminLocalGIFTempFile(file, workDir, adminLocalGIFZipToWebPMaxSingleEntryBytes)
		if writeErr != nil {
			adminLocalGIFZipToWebPJobs.update(jobID, func(job *adminLocalGIFZipToWebPJob) {
				job.FailedFiles++
				job.ProcessedTargetFiles++
				if len(job.Failures) < adminLocalGIFZipToWebPMaxFailureDetails {
					job.Failures = append(job.Failures, fmt.Sprintf("%s: %v", checked.NormalizedPath, writeErr))
				}
			})
			continue
		}

		totalUncompressed += copiedBytes
		if totalUncompressed > adminLocalGIFZipToWebPMaxUncompressedBytes {
			_ = os.Remove(inputGIFPath)
			h.failAdminLocalGIFZipToWebPJob(jobID, "zip 解压总大小超过限制（最大 2GB）")
			return
		}

		outputWebPPath := strings.TrimSuffix(inputGIFPath, filepath.Ext(inputGIFPath)) + ".webp"
		convertErr := convertAdminLocalGIFToWebP(context.Background(), inputGIFPath, outputWebPPath, jobSnapshot.WebPCodec)
		_ = os.Remove(inputGIFPath)
		if convertErr != nil {
			_ = os.Remove(outputWebPPath)
			adminLocalGIFZipToWebPJobs.update(jobID, func(job *adminLocalGIFZipToWebPJob) {
				job.FailedFiles++
				job.ProcessedTargetFiles++
				if len(job.Failures) < adminLocalGIFZipToWebPMaxFailureDetails {
					job.Failures = append(job.Failures, fmt.Sprintf("%s: %v", checked.NormalizedPath, convertErr))
				}
			})
			continue
		}

		outputEntry := strings.TrimSuffix(checked.NormalizedPath, path.Ext(checked.NormalizedPath)) + ".webp"
		outputEntry = ensureAdminLocalUniqueZipEntryName(outputEntry, seenOutputEntries)
		if err := copyAdminLocalFileToZip(zipWriter, outputWebPPath, outputEntry, file.Modified); err != nil {
			_ = os.Remove(outputWebPPath)
			adminLocalGIFZipToWebPJobs.update(jobID, func(job *adminLocalGIFZipToWebPJob) {
				job.FailedFiles++
				job.ProcessedTargetFiles++
				if len(job.Failures) < adminLocalGIFZipToWebPMaxFailureDetails {
					job.Failures = append(job.Failures, fmt.Sprintf("%s: 写入输出 zip 失败", checked.NormalizedPath))
				}
			})
			continue
		}
		_ = os.Remove(outputWebPPath)
		adminLocalGIFZipToWebPJobs.update(jobID, func(job *adminLocalGIFZipToWebPJob) {
			job.ConvertedFiles++
			job.ProcessedTargetFiles++
		})
	}

	if err := zipWriter.Close(); err != nil {
		h.failAdminLocalGIFZipToWebPJob(jobID, "关闭输出 zip 失败")
		return
	}
	if err := outputFile.Close(); err != nil {
		h.failAdminLocalGIFZipToWebPJob(jobID, "写入输出 zip 失败")
		return
	}
	outputClosed = true

	jobSnapshot, ok = adminLocalGIFZipToWebPJobs.get(jobID)
	if !ok {
		_ = os.Remove(outputPath)
		return
	}
	if jobSnapshot.ConvertedFiles <= 0 {
		_ = os.Remove(outputPath)
		h.failAdminLocalGIFZipToWebPJob(jobID, "所有 GIF 文件转换失败，请检查文件内容或 ffmpeg 编码能力")
		return
	}

	warnings := append([]string{}, jobSnapshot.Warnings...)
	if jobSnapshot.WebPCodec != "libwebp_anim" {
		warnings = append(warnings, "当前 ffmpeg 未启用 libwebp_anim，部分动图可能降级为静态 WebP")
	}
	if jobSnapshot.SkippedFiles > 0 {
		warnings = append(warnings, fmt.Sprintf("已跳过 %d 个非 GIF 文件", jobSnapshot.SkippedFiles))
	}
	if jobSnapshot.FailedFiles > len(jobSnapshot.Failures) {
		warnings = append(warnings, fmt.Sprintf("另有 %d 个失败文件未在列表中展示", jobSnapshot.FailedFiles-len(jobSnapshot.Failures)))
	}
	if jobSnapshot.FailedFiles > 0 {
		warnings = append(warnings, "部分文件转换失败，详情见 failures")
	}

	completedAt := time.Now()
	adminLocalGIFZipToWebPJobs.update(jobID, func(job *adminLocalGIFZipToWebPJob) {
		job.Status = "completed"
		job.OutputPath = outputPath
		job.OutputFile = filepath.Base(outputPath)
		job.DesktopDir = desktopDir
		job.Warnings = warnings
		job.ElapsedMS = completedAt.Sub(job.CreatedAt).Milliseconds()
		job.FinishedAt = &completedAt
	})
	outputCommitted = true
}

// StartConvertLocalGIFZipToWebPJob godoc
// @Summary Start local GIF zip to WebP conversion job (admin)
// @Tags admin
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "zip file containing gif files"
// @Success 202 {object} AdminLocalGIFZipToWebPJobStartResponse
// @Router /api/admin/system/local/gif-zip-to-webp/jobs [post]
func (h *Handler) StartConvertLocalGIFZipToWebPJob(c *gin.Context) {
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

	tempZipFile, err := os.CreateTemp("", "admin-gif-job-*.zip")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建临时文件失败"})
		return
	}
	tempZipPath := tempZipFile.Name()

	copied, err := io.Copy(tempZipFile, io.LimitReader(uploaded, adminLocalGIFZipToWebPMaxUploadBytes+1))
	if err != nil {
		_ = tempZipFile.Close()
		_ = os.Remove(tempZipPath)
		c.JSON(http.StatusBadRequest, gin.H{"error": "保存上传文件失败"})
		return
	}
	if copied > adminLocalGIFZipToWebPMaxUploadBytes {
		_ = tempZipFile.Close()
		_ = os.Remove(tempZipPath)
		c.JSON(http.StatusBadRequest, gin.H{"error": "zip 文件过大，最大支持 512MB"})
		return
	}
	if err := tempZipFile.Close(); err != nil {
		_ = os.Remove(tempZipPath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存上传文件失败"})
		return
	}

	totalEntries, totalTargets, skipped, failed, failures, scanErr := preScanAdminLocalGIFZip(tempZipPath)
	if scanErr != nil {
		_ = os.Remove(tempZipPath)
		c.JSON(http.StatusBadRequest, gin.H{"error": scanErr.Error()})
		return
	}
	if totalTargets == 0 {
		_ = os.Remove(tempZipPath)
		message := "zip 中没有可转换的 GIF 文件"
		if failed > 0 {
			message = "所有 GIF 文件预检失败，请检查文件内容"
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"error":              message,
			"total_entries":      totalEntries,
			"total_target_files": totalTargets,
			"skipped_files":      skipped,
			"failed_files":       failed,
			"failures":           failures,
		})
		return
	}

	jobID := newAdminLocalGIFZipToWebPJobID()
	now := time.Now()
	job := &adminLocalGIFZipToWebPJob{
		ID:                   jobID,
		Status:               "queued",
		InputFile:            fileHeader.Filename,
		TempZipPath:          tempZipPath,
		WebPCodec:            webpCodec,
		TotalEntries:         totalEntries,
		TotalTargetFiles:     totalTargets,
		ProcessedTargetFiles: 0,
		ConvertedFiles:       0,
		SkippedFiles:         skipped,
		FailedFiles:          failed,
		Failures:             append([]string{}, failures...),
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	adminLocalGIFZipToWebPJobs.create(job)
	go h.runAdminLocalGIFZipToWebPJob(jobID)

	warnings := make([]string, 0, 1)
	if webpCodec != "libwebp_anim" {
		warnings = append(warnings, "当前 ffmpeg 未启用 libwebp_anim，部分动图可能降级为静态 WebP")
	}

	c.JSON(http.StatusAccepted, AdminLocalGIFZipToWebPJobStartResponse{
		JobID:                jobID,
		Status:               "queued",
		InputFile:            fileHeader.Filename,
		TotalEntries:         totalEntries,
		TotalTargetFiles:     totalTargets,
		ProcessedTargetFiles: 0,
		ConvertedFiles:       0,
		SkippedFiles:         skipped,
		FailedFiles:          failed,
		Failures:             append([]string{}, failures...),
		Warnings:             warnings,
		CreatedAt:            now.Format(time.RFC3339),
	})
}

// GetConvertLocalGIFZipToWebPJob godoc
// @Summary Get local GIF zip to WebP conversion job status (admin)
// @Tags admin
// @Produce json
// @Param job_id path string true "job id"
// @Success 200 {object} AdminLocalGIFZipToWebPJobStatusResponse
// @Router /api/admin/system/local/gif-zip-to-webp/jobs/{job_id} [get]
func (h *Handler) GetConvertLocalGIFZipToWebPJob(c *gin.Context) {
	jobID := strings.TrimSpace(c.Param("job_id"))
	if jobID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "job_id required"})
		return
	}
	job, ok := adminLocalGIFZipToWebPJobs.get(jobID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	c.JSON(http.StatusOK, buildAdminLocalGIFZipToWebPJobStatus(job))
}
