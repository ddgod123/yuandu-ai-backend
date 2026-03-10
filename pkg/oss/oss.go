package oss

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	"emoji/internal/config"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

type Client struct {
	bucket  *oss.Bucket
	baseURL string
}

func NewClient(cfg config.Config) (*Client, error) {
	endpoint := strings.TrimSpace(cfg.OSSEndpoint)
	ak := strings.TrimSpace(cfg.OSSAccessKeyID)
	sk := strings.TrimSpace(cfg.OSSAccessKeySecret)
	bucketName := strings.TrimSpace(cfg.OSSBucket)

	if endpoint == "" || ak == "" || sk == "" || bucketName == "" {
		return nil, fmt.Errorf("OSS config incomplete: endpoint=%q ak=%q bucket=%q", endpoint, ak, bucketName)
	}

	client, err := oss.New(endpoint, ak, sk)
	if err != nil {
		return nil, fmt.Errorf("oss.New: %w", err)
	}

	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return nil, fmt.Errorf("oss.Bucket: %w", err)
	}

	baseURL := strings.TrimSpace(cfg.OSSBaseURL)
	if baseURL == "" {
		baseURL = fmt.Sprintf("https://%s.%s", bucketName, endpoint)
	}
	baseURL = strings.TrimRight(baseURL, "/")

	return &Client{bucket: bucket, baseURL: baseURL}, nil
}

func (c *Client) Upload(objectKey string, data []byte) (string, error) {
	reader := bytes.NewReader(data)
	if err := c.bucket.PutObject(objectKey, reader); err != nil {
		return "", fmt.Errorf("oss upload %s: %w", objectKey, err)
	}
	return c.URL(objectKey), nil
}

func (c *Client) Download(objectKey string) ([]byte, error) {
	rc, err := c.bucket.GetObject(objectKey)
	if err != nil {
		return nil, fmt.Errorf("oss download %s: %w", objectKey, err)
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func (c *Client) URL(objectKey string) string {
	return c.baseURL + "/" + strings.TrimLeft(objectKey, "/")
}

func (c *Client) SignedURL(objectKey string, expiry time.Duration) (string, error) {
	return c.bucket.SignURL(objectKey, oss.HTTPGet, int64(expiry.Seconds()))
}

func TemplateKey(id uint64) string {
	return fmt.Sprintf("templates/%d.png", id)
}

func MemeKey(userID uint64) string {
	return fmt.Sprintf("memes/%d/%d.png", userID, time.Now().UnixNano())
}
