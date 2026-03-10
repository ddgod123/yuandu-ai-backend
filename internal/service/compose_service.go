package service

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"

	"emoji/internal/models"
	"emoji/pkg/oss"

	"github.com/fogleman/gg"
)

type ComposeService struct {
	ossClient *oss.Client
	fontPath  string
}

func NewComposeService(ossClient *oss.Client, fontPath string) *ComposeService {
	return &ComposeService{ossClient: ossClient, fontPath: fontPath}
}

// PLACEHOLDER_COMPOSE_1

// Compose renders memeText onto the template image and uploads to OSS.
func (s *ComposeService) Compose(tmpl models.MemeTemplate, memeText string, userID uint64) (string, error) {
	// Download template image from OSS
	imgData, err := s.ossClient.Download(templateObjectKey(tmpl.ImageURL))
	if err != nil {
		return "", fmt.Errorf("download template: %w", err)
	}

	img, _, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		return "", fmt.Errorf("decode template image: %w", err)
	}

	dc := gg.NewContextForImage(img)

	// Load font
	fontSize := float64(tmpl.FontSize)
	if fontSize <= 0 {
		fontSize = 32
	}
	if err := dc.LoadFontFace(s.fontPath, fontSize); err != nil {
		return "", fmt.Errorf("load font: %w", err)
	}

	// Parse text color
	textColor := parseHexColor(tmpl.TextColor)
	dc.SetColor(textColor)

	// Text area
	tx := float64(tmpl.TextX)
	ty := float64(tmpl.TextY)
	tw := float64(tmpl.TextWidth)
	th := float64(tmpl.TextHeight)
	if tw <= 0 {
		tw = float64(img.Bounds().Dx()) * 0.8
		tx = float64(img.Bounds().Dx()) * 0.1
	}
	if th <= 0 {
		th = float64(img.Bounds().Dy()) * 0.3
		ty = float64(img.Bounds().Dy()) * 0.65
	}

	// Draw text with word wrap
	dc.DrawStringWrapped(memeText, tx+tw/2, ty+th/2, 0.5, 0.5, tw, 1.5, gg.AlignCenter)

	// Encode to PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, dc.Image()); err != nil {
		return "", fmt.Errorf("encode png: %w", err)
	}

	// Upload to OSS
	objectKey := oss.MemeKey(userID)
	url, err := s.ossClient.Upload(objectKey, buf.Bytes())
	if err != nil {
		return "", fmt.Errorf("upload meme: %w", err)
	}

	return url, nil
}

func templateObjectKey(imageURL string) string {
	// If it's a full URL, extract the path part
	if idx := strings.Index(imageURL, "templates/"); idx >= 0 {
		return imageURL[idx:]
	}
	return imageURL
}

func parseHexColor(hex string) color.Color {
	hex = strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(hex) != 6 {
		return color.White
	}
	var r, g, b uint8
	fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)
	return color.RGBA{R: r, G: g, B: b, A: 255}
}
