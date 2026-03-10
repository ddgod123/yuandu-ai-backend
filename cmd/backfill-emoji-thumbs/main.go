package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/color/palette"
	stddraw "image/draw"
	"image/gif"
	"io"
	"log"
	"math"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"emoji/internal/config"
	"emoji/internal/db"
	"emoji/internal/models"
	"emoji/internal/storage"

	"github.com/joho/godotenv"
	"github.com/qiniu/go-sdk/v7/client"
	qiniustorage "github.com/qiniu/go-sdk/v7/storage"
	"gorm.io/gorm"
)

const (
	listPreviewGIFWidth      = 320
	listPreviewGIFFPS        = 10
	listPreviewGIFMaxFrames  = 48
	listPreviewGIFMaxSrcSize = 20 << 20 // 20MB
	listPreviewMinGainRatio  = 0.95
	listPreviewDefaultDelay  = 10
)

type emojiCandidate struct {
	ID       uint64
	FileURL  string
	Format   string
	ThumbURL string
}

type backfillReport struct {
	Apply              bool      `json:"apply"`
	TotalScanned       int       `json:"total_scanned"`
	UpdatedDBRows      int       `json:"updated_db_rows"`
	UploadedThumbs     int       `json:"uploaded_thumbs"`
	ReusedExisting     int       `json:"reused_existing"`
	RebuiltExisting    int       `json:"rebuilt_existing"`
	SkippedHasThumb    int       `json:"skipped_has_thumb"`
	SkippedNoKey       int       `json:"skipped_no_key"`
	SkippedNonGIF      int       `json:"skipped_non_gif"`
	SkippedNotFound    int       `json:"skipped_not_found"`
	SkippedTooLarge    int       `json:"skipped_too_large"`
	SkippedLowGain     int       `json:"skipped_low_gain"`
	SkippedDecodeError int       `json:"skipped_decode_error"`
	Failed             int       `json:"failed"`
	LastID             uint64    `json:"last_id"`
	GeneratedAt        time.Time `json:"generated_at"`
}

func main() {
	apply := flag.Bool("apply", false, "apply updates to qiniu + database")
	force := flag.Bool("force", false, "rebuild even when thumb_url already exists")
	rebuildExisting := flag.Bool("rebuild-existing", false, "overwrite existing list gif object key when it already exists")
	collectionID := flag.Uint64("collection-id", 0, "only process one collection id")
	startID := flag.Uint64("start-id", 0, "start from emoji id (exclusive)")
	limit := flag.Int("limit", 0, "max emojis to process (0 means no limit)")
	batchSize := flag.Int("batch-size", 200, "batch size when scanning db")
	reportPath := flag.String("report", "", "write json report to file")
	flag.Parse()

	if *batchSize <= 0 || *batchSize > 2000 {
		*batchSize = 200
	}

	loadEnv()
	cfg := config.Load()

	dbConn, err := db.Connect(cfg)
	if err != nil {
		log.Fatalf("db connect failed: %v", err)
	}
	qiniuClient, err := storage.NewQiniuClient(cfg)
	if err != nil {
		log.Fatalf("qiniu connect failed: %v", err)
	}

	report := backfillThumbs(
		dbConn,
		qiniuClient,
		*apply,
		*force,
		*rebuildExisting,
		*collectionID,
		*startID,
		*limit,
		*batchSize,
	)
	report.GeneratedAt = time.Now()

	raw, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(raw))
	if strings.TrimSpace(*reportPath) != "" {
		if err := os.WriteFile(*reportPath, raw, 0644); err != nil {
			log.Fatalf("write report failed: %v", err)
		}
	}
}

func backfillThumbs(
	dbConn *gorm.DB,
	qiniuClient *storage.QiniuClient,
	apply bool,
	force bool,
	rebuildExisting bool,
	collectionID uint64,
	startID uint64,
	limit int,
	batchSize int,
) backfillReport {
	report := backfillReport{Apply: apply, LastID: startID}
	bm := qiniuClient.BucketManager()
	uploader := qiniustorage.NewFormUploader(qiniuClient.Cfg)

	for {
		if limit > 0 && report.TotalScanned >= limit {
			break
		}

		var rows []emojiCandidate
		query := dbConn.Model(&models.Emoji{}).
			Select("id, file_url, format, thumb_url").
			Where("deleted_at IS NULL").
			Where("id > ?", report.LastID).
			Where("LOWER(COALESCE(format, '')) LIKE ? OR LOWER(COALESCE(file_url, '')) LIKE ?", "%gif%", "%.gif%").
			Order("id ASC").
			Limit(batchSize)
		if collectionID > 0 {
			query = query.Where("collection_id = ?", collectionID)
		}
		if !force {
			query = query.Where("COALESCE(NULLIF(TRIM(thumb_url), ''), '') = ''")
		}
		if err := query.Find(&rows).Error; err != nil {
			report.Failed++
			break
		}
		if len(rows) == 0 {
			break
		}

		for _, row := range rows {
			report.LastID = row.ID
			if limit > 0 && report.TotalScanned >= limit {
				break
			}
			report.TotalScanned++
			backfillOneEmoji(dbConn, bm, uploader, qiniuClient, row, apply, force, rebuildExisting, &report)
		}
	}
	return report
}

func backfillOneEmoji(
	dbConn *gorm.DB,
	bm *qiniustorage.BucketManager,
	uploader *qiniustorage.FormUploader,
	qiniuClient *storage.QiniuClient,
	row emojiCandidate,
	apply bool,
	force bool,
	rebuildExisting bool,
	report *backfillReport,
) {
	if !force && strings.TrimSpace(row.ThumbURL) != "" {
		report.SkippedHasThumb++
		return
	}

	sourceKey := extractQiniuObjectKey(row.FileURL, qiniuClient)
	if sourceKey == "" {
		report.SkippedNoKey++
		return
	}
	if !isGIFObjectKey(sourceKey) {
		report.SkippedNonGIF++
		return
	}
	thumbKey := buildListPreviewGIFKey(sourceKey)
	if thumbKey == "" {
		report.SkippedNonGIF++
		return
	}

	exists, err := qiniuObjectExists(bm, qiniuClient.Bucket, thumbKey)
	if err != nil {
		report.Failed++
		return
	}
	if exists && !rebuildExisting {
		report.ReusedExisting++
		if apply {
			if err := updateEmojiThumbURL(dbConn, row.ID, thumbKey); err != nil {
				report.Failed++
				return
			}
			report.UpdatedDBRows++
		}
		return
	}

	sourceBytes, err := downloadQiniuObjectLimited(bm, qiniuClient.Bucket, sourceKey, listPreviewGIFMaxSrcSize)
	if err != nil {
		switch {
		case isQiniuNotFound(err):
			report.SkippedNotFound++
		case errors.Is(err, errSourceTooLarge):
			report.SkippedTooLarge++
		default:
			report.Failed++
		}
		return
	}

	previewBytes, err := buildCompressedGIF(sourceBytes)
	if err != nil {
		if errors.Is(err, errSourceTooLarge) {
			report.SkippedTooLarge++
		} else {
			report.SkippedDecodeError++
		}
		return
	}
	if float64(len(previewBytes)) >= float64(len(sourceBytes))*listPreviewMinGainRatio && !(rebuildExisting && exists) {
		report.SkippedLowGain++
		return
	}

	if !apply {
		return
	}
	if err := uploadReaderToQiniu(uploader, qiniuClient, thumbKey, bytes.NewReader(previewBytes), int64(len(previewBytes))); err != nil {
		report.Failed++
		return
	}
	report.UploadedThumbs++
	if exists && rebuildExisting {
		report.RebuiltExisting++
	}

	if err := updateEmojiThumbURL(dbConn, row.ID, thumbKey); err != nil {
		report.Failed++
		return
	}
	report.UpdatedDBRows++
}

func updateEmojiThumbURL(dbConn *gorm.DB, emojiID uint64, thumbKey string) error {
	return dbConn.Model(&models.Emoji{}).
		Where("id = ?", emojiID).
		Updates(map[string]interface{}{"thumb_url": thumbKey}).
		Error
}

func qiniuObjectExists(bm *qiniustorage.BucketManager, bucket string, key string) (bool, error) {
	_, err := bm.Stat(bucket, key)
	if err == nil {
		return true, nil
	}
	if isQiniuNotFound(err) {
		return false, nil
	}
	return false, err
}

var errSourceTooLarge = errors.New("source gif too large")

func downloadQiniuObjectLimited(bm *qiniustorage.BucketManager, bucket string, key string, maxSize int64) ([]byte, error) {
	rc, err := bm.Get(bucket, key, nil)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	limited := &io.LimitedReader{
		R: rc,
		N: maxSize + 1,
	}
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxSize {
		return nil, errSourceTooLarge
	}
	return body, nil
}

func uploadReaderToQiniu(uploader *qiniustorage.FormUploader, q *storage.QiniuClient, key string, reader io.Reader, size int64) error {
	putPolicy := qiniustorage.PutPolicy{
		Scope: q.Bucket + ":" + key,
	}
	upToken := putPolicy.UploadToken(q.Mac)
	var ret qiniustorage.PutRet
	return uploader.Put(context.Background(), &ret, upToken, key, reader, size, &qiniustorage.PutExtra{})
}

func isQiniuNotFound(err error) bool {
	info, ok := err.(*client.ErrorInfo)
	if !ok {
		return false
	}
	return info.Code == 404 || info.Code == 612
}

func buildListPreviewGIFKey(rawKey string) string {
	key := strings.TrimLeft(strings.TrimSpace(rawKey), "/")
	if key == "" || !strings.HasSuffix(strings.ToLower(key), ".gif") {
		return ""
	}
	if strings.Contains(key, "/raw/") {
		return strings.Replace(key, "/raw/", "/list/", 1)
	}
	ext := path.Ext(key)
	return strings.TrimSuffix(key, ext) + "_list.gif"
}

func isGIFObjectKey(raw string) bool {
	clean := strings.SplitN(raw, "?", 2)[0]
	clean = strings.SplitN(clean, "#", 2)[0]
	return strings.HasSuffix(strings.ToLower(clean), ".gif")
}

func buildCompressedGIF(source []byte) ([]byte, error) {
	if len(source) == 0 {
		return nil, errors.New("empty gif source")
	}
	if len(source) > listPreviewGIFMaxSrcSize {
		return nil, errSourceTooLarge
	}
	srcGIF, err := gif.DecodeAll(bytes.NewReader(source))
	if err != nil {
		return nil, err
	}
	if len(srcGIF.Image) == 0 {
		return nil, errors.New("gif has no frame")
	}

	bounds := srcGIF.Image[0].Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	if srcW <= 0 || srcH <= 0 {
		return nil, errors.New("invalid gif size")
	}

	dstW := srcW
	if dstW > listPreviewGIFWidth {
		dstW = listPreviewGIFWidth
	}
	dstH := int(math.Round(float64(srcH) * float64(dstW) / float64(srcW)))
	if dstH < 1 {
		dstH = 1
	}

	frameIndices := pickGIFFrameIndices(srcGIF.Delay, len(srcGIF.Image), listPreviewGIFFPS, listPreviewGIFMaxFrames)
	if len(frameIndices) == 0 {
		frameIndices = []int{0}
	}
	frameDelays := buildSelectedFrameDelays(frameIndices, srcGIF.Delay, len(srcGIF.Image))
	selectedPos := make(map[int]int, len(frameIndices))
	for pos, idx := range frameIndices {
		selectedPos[idx] = pos
	}

	outGIF := &gif.GIF{
		LoopCount: srcGIF.LoopCount,
		Image:     make([]*image.Paletted, 0, len(frameIndices)),
		Delay:     make([]int, 0, len(frameIndices)),
	}
	outPalette := make(color.Palette, 0, 256)
	outPalette = append(outPalette, color.RGBA{R: 0, G: 0, B: 0, A: 0})
	if len(palette.Plan9) > 0 {
		outPalette = append(outPalette, palette.Plan9[:len(palette.Plan9)-1]...)
	}
	canvas := image.NewNRGBA(image.Rect(0, 0, srcW, srcH))

	for idx, srcFrame := range srcGIF.Image {
		if srcFrame == nil {
			continue
		}
		disposal := byte(0)
		if idx < len(srcGIF.Disposal) {
			disposal = srcGIF.Disposal[idx]
		}

		var previous *image.NRGBA
		if disposal == gif.DisposalPrevious {
			previous = cloneNRGBA(canvas)
		}

		stddraw.Draw(canvas, srcFrame.Bounds(), srcFrame, srcFrame.Bounds().Min, stddraw.Over)

		if pos, ok := selectedPos[idx]; ok {
			scaled := resizeNRGBANearest(canvas, dstW, dstH)
			if scaled != nil {
				dstFrame := quantizeNRGBAToPaletted(scaled, outPalette)
				if dstFrame != nil {
					outGIF.Image = append(outGIF.Image, dstFrame)
					delay := listPreviewDefaultDelay
					if pos < len(frameDelays) && frameDelays[pos] > 0 {
						delay = frameDelays[pos]
					}
					if delay < 1 {
						delay = 1
					}
					outGIF.Delay = append(outGIF.Delay, delay)
				}
			}
		}

		switch disposal {
		case gif.DisposalBackground:
			clearNRGBARect(canvas, srcFrame.Bounds())
		case gif.DisposalPrevious:
			if previous != nil {
				copy(canvas.Pix, previous.Pix)
			}
		}
	}
	if len(outGIF.Image) == 0 {
		return nil, errors.New("no output gif frame")
	}

	var out bytes.Buffer
	if err := gif.EncodeAll(&out, outGIF); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func buildSelectedFrameDelays(indices []int, delays []int, frameCount int) []int {
	if len(indices) == 0 || frameCount <= 0 {
		return nil
	}
	out := make([]int, 0, len(indices))
	for pos, start := range indices {
		if start < 0 || start >= frameCount {
			out = append(out, listPreviewDefaultDelay)
			continue
		}
		end := frameCount
		if pos+1 < len(indices) {
			end = indices[pos+1]
		}
		if end <= start {
			end = start + 1
		}
		total := 0
		for idx := start; idx < end && idx < frameCount; idx++ {
			total += gifFrameDelay(delays, idx)
		}
		if total <= 0 {
			total = gifFrameDelay(delays, start)
		}
		out = append(out, total)
	}
	return out
}

func gifFrameDelay(delays []int, idx int) int {
	if idx >= 0 && idx < len(delays) && delays[idx] > 0 {
		return delays[idx]
	}
	return listPreviewDefaultDelay
}

func pickGIFFrameIndices(delays []int, frameCount int, targetFPS int, maxFrames int) []int {
	if frameCount <= 0 {
		return nil
	}
	if targetFPS <= 0 {
		targetFPS = listPreviewGIFFPS
	}
	if maxFrames <= 0 {
		maxFrames = listPreviewGIFMaxFrames
	}

	totalCentis := 0
	for i := 0; i < frameCount; i++ {
		totalCentis += gifFrameDelay(delays, i)
	}
	if totalCentis <= 0 {
		totalCentis = frameCount * listPreviewDefaultDelay
	}

	durationSec := float64(totalCentis) / 100.0
	srcFPS := float64(frameCount) / durationSec
	step := 1
	if srcFPS > float64(targetFPS) {
		step = int(math.Ceil(srcFPS / float64(targetFPS)))
	}
	if step < 1 {
		step = 1
	}

	indices := make([]int, 0, frameCount/step+1)
	for idx := 0; idx < frameCount; idx += step {
		indices = append(indices, idx)
		if len(indices) >= maxFrames {
			break
		}
	}
	if len(indices) == 0 || indices[0] != 0 {
		indices = append([]int{0}, indices...)
	}
	return indices
}

func resizeNRGBANearest(src *image.NRGBA, dstW int, dstH int) *image.NRGBA {
	if src == nil || dstW <= 0 || dstH <= 0 {
		return nil
	}
	sb := src.Bounds()
	srcW := sb.Dx()
	srcH := sb.Dy()
	if srcW <= 0 || srcH <= 0 {
		return nil
	}

	dst := image.NewNRGBA(image.Rect(0, 0, dstW, dstH))
	for y := 0; y < dstH; y++ {
		sy := sb.Min.Y + y*srcH/dstH
		for x := 0; x < dstW; x++ {
			sx := sb.Min.X + x*srcW/dstW
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

func quantizeNRGBAToPaletted(src *image.NRGBA, pal color.Palette) *image.Paletted {
	if src == nil || len(pal) == 0 {
		return nil
	}
	b := src.Bounds()
	dst := image.NewPaletted(image.Rect(0, 0, b.Dx(), b.Dy()), pal)
	stddraw.FloydSteinberg.Draw(dst, dst.Rect, src, b.Min)
	return dst
}

func cloneNRGBA(src *image.NRGBA) *image.NRGBA {
	if src == nil {
		return nil
	}
	out := image.NewNRGBA(src.Bounds())
	copy(out.Pix, src.Pix)
	return out
}

func clearNRGBARect(dst *image.NRGBA, rect image.Rectangle) {
	if dst == nil {
		return
	}
	b := dst.Bounds().Intersect(rect)
	if b.Empty() {
		return
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dst.SetNRGBA(x, y, color.NRGBA{})
		}
	}
}

func extractQiniuObjectKey(raw string, qiniuClient *storage.QiniuClient) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		key := strings.TrimLeft(strings.SplitN(raw, "?", 2)[0], "/")
		if decoded, err := url.PathUnescape(key); err == nil && strings.TrimSpace(decoded) != "" {
			key = strings.TrimSpace(decoded)
		}
		return key
	}

	parsedURL, err := url.Parse(raw)
	if err != nil || parsedURL.Host == "" {
		return ""
	}

	domainHost, domainPath, ok := qiniuDomainInfo(qiniuClient)
	if ok && strings.EqualFold(parsedURL.Hostname(), domainHost) {
		pathKey := strings.TrimLeft(parsedURL.EscapedPath(), "/")
		if domainPath != "" {
			if pathKey == domainPath {
				pathKey = ""
			} else if strings.HasPrefix(pathKey, domainPath+"/") {
				pathKey = strings.TrimPrefix(pathKey, domainPath+"/")
			} else {
				return ""
			}
		}
		if pathKey == "" {
			return ""
		}
		if decoded, err := url.PathUnescape(pathKey); err == nil {
			pathKey = decoded
		}
		return strings.TrimSpace(pathKey)
	}

	fallback := strings.TrimLeft(parsedURL.EscapedPath(), "/")
	if decoded, err := url.PathUnescape(fallback); err == nil {
		fallback = decoded
	}
	if strings.HasPrefix(fallback, "emoji/") {
		return strings.TrimSpace(fallback)
	}
	return ""
}

func qiniuDomainInfo(qiniuClient *storage.QiniuClient) (host string, pathPrefix string, ok bool) {
	if qiniuClient == nil {
		return "", "", false
	}
	domain := strings.TrimSpace(qiniuClient.Domain)
	if domain == "" {
		return "", "", false
	}
	if !strings.HasPrefix(domain, "http://") && !strings.HasPrefix(domain, "https://") {
		if qiniuClient.UseHTTPS {
			domain = "https://" + domain
		} else {
			domain = "http://" + domain
		}
	}
	parsedDomain, err := url.Parse(domain)
	if err != nil || parsedDomain.Host == "" {
		return "", "", false
	}
	return strings.ToLower(parsedDomain.Hostname()), strings.Trim(parsedDomain.EscapedPath(), "/"), true
}

func loadEnv() {
	seen := map[string]struct{}{}
	candidates := []string{".env", "backend/.env"}

	if wd, err := os.Getwd(); err == nil {
		dir := wd
		for i := 0; i < 5; i++ {
			candidates = append(candidates,
				filepath.Join(dir, ".env"),
				filepath.Join(dir, "backend", ".env"),
			)
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	for _, p := range candidates {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		if _, err := os.Stat(p); err == nil {
			_ = godotenv.Overload(p)
		}
	}
}
