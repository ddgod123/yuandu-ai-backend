package handlers

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/color/palette"
	stddraw "image/draw"
	"image/gif"
	"math"
	"path"
	"strings"

	"emoji/internal/storage"

	qiniustorage "github.com/qiniu/go-sdk/v7/storage"
)

const (
	listPreviewGIFWidth      = 320
	listPreviewGIFFPS        = 10
	listPreviewGIFMaxFrames  = 48
	listPreviewGIFMaxSrcSize = 20 << 20 // 20MB
	listPreviewMinGainRatio  = 0.95
	listPreviewDefaultDelay  = 10
)

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

func tryUploadListPreviewGIF(
	uploader *qiniustorage.FormUploader,
	q *storage.QiniuClient,
	rawKey string,
	source []byte,
) string {
	if uploader == nil || q == nil || len(source) == 0 {
		return ""
	}
	dstKey := buildListPreviewGIFKey(rawKey)
	if dstKey == "" {
		return ""
	}
	previewBytes, err := buildCompressedGIF(source)
	if err != nil || len(previewBytes) == 0 {
		return ""
	}
	// Skip preview upload when compression gain is too small.
	if float64(len(previewBytes)) >= float64(len(source))*listPreviewMinGainRatio {
		return ""
	}
	if err := uploadReaderToQiniu(uploader, q, dstKey, bytes.NewReader(previewBytes), int64(len(previewBytes))); err != nil {
		return ""
	}
	return dstKey
}

func buildCompressedGIF(source []byte) ([]byte, error) {
	if len(source) == 0 {
		return nil, errors.New("empty gif source")
	}
	if len(source) > listPreviewGIFMaxSrcSize {
		return nil, errors.New("gif source too large")
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
		delay := 10
		if i < len(delays) && delays[i] > 0 {
			delay = delays[i]
		}
		totalCentis += delay
	}
	if totalCentis <= 0 {
		totalCentis = frameCount * 10
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
