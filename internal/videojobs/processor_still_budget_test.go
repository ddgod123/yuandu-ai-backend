package videojobs

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

func TestConvertImageToJPG_SizeProfileShrinksOutput(t *testing.T) {
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.jpg")
	if err := writeSyntheticJPEG(inputPath, 2200, 1240, 96); err != nil {
		t.Fatalf("write synthetic jpeg: %v", err)
	}

	clarityPath := filepath.Join(tmpDir, "clarity.jpg")
	sizePath := filepath.Join(tmpDir, "size.jpg")
	if err := convertImageToJPG(inputPath, clarityPath, QualityProfileClarity, 512); err != nil {
		t.Fatalf("convert clarity jpg: %v", err)
	}
	if err := convertImageToJPG(inputPath, sizePath, QualityProfileSize, 320); err != nil {
		t.Fatalf("convert size jpg: %v", err)
	}

	clarityBytes, clarityW, _ := readImageInfo(clarityPath)
	sizeBytes, sizeW, _ := readImageInfo(sizePath)
	if sizeBytes <= 0 || clarityBytes <= 0 {
		t.Fatalf("unexpected output sizes clarity=%d size=%d", clarityBytes, sizeBytes)
	}
	if sizeBytes >= clarityBytes {
		t.Fatalf("expected size-profile jpg smaller than clarity, got size=%d clarity=%d", sizeBytes, clarityBytes)
	}
	if sizeW >= clarityW {
		t.Fatalf("expected size-profile jpg width lower, got size=%d clarity=%d", sizeW, clarityW)
	}
}

func TestConvertImageToPNG_SizeProfileShrinksOutput(t *testing.T) {
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.jpg")
	if err := writeSyntheticJPEG(inputPath, 2200, 1240, 96); err != nil {
		t.Fatalf("write synthetic jpeg: %v", err)
	}

	clarityPath := filepath.Join(tmpDir, "clarity.png")
	sizePath := filepath.Join(tmpDir, "size.png")
	if err := convertImageToPNG(inputPath, clarityPath, QualityProfileClarity, 1024); err != nil {
		t.Fatalf("convert clarity png: %v", err)
	}
	if err := convertImageToPNG(inputPath, sizePath, QualityProfileSize, 900); err != nil {
		t.Fatalf("convert size png: %v", err)
	}

	clarityBytes, clarityW, _ := readImageInfo(clarityPath)
	sizeBytes, sizeW, _ := readImageInfo(sizePath)
	if sizeBytes <= 0 || clarityBytes <= 0 {
		t.Fatalf("unexpected output sizes clarity=%d size=%d", clarityBytes, sizeBytes)
	}
	if sizeBytes >= clarityBytes {
		t.Fatalf("expected size-profile png smaller than clarity, got size=%d clarity=%d", sizeBytes, clarityBytes)
	}
	if sizeW >= clarityW {
		t.Fatalf("expected size-profile png width lower, got size=%d clarity=%d", sizeW, clarityW)
	}
}

func writeSyntheticJPEG(path string, width, height, quality int) error {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r := uint8((x * 255) / maxInt(width-1, 1))
			g := uint8((y * 255) / maxInt(height-1, 1))
			b := uint8(((x + y) * 255) / maxInt(width+height-2, 1))
			img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return jpeg.Encode(f, img, &jpeg.Options{Quality: quality})
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
