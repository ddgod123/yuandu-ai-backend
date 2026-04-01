package handlers

import (
	"strings"
	"testing"
	"time"

	appstorage "emoji/internal/storage"

	"github.com/qiniu/go-sdk/v7/auth/qbox"
)

func TestExtractQiniuObjectKey_LegacyAbsoluteURLFallback(t *testing.T) {
	q := &appstorage.QiniuClient{
		Domain:   "cdn.emoji.icu",
		UseHTTPS: false,
	}

	key, ok := extractQiniuObjectKey("https://legacy.example.com/emoji/demo/a.gif?x=1", q)
	if !ok {
		t.Fatalf("expected legacy absolute url to be parsed as key")
	}
	if key != "emoji/demo/a.gif" {
		t.Fatalf("unexpected key: %s", key)
	}

	if _, ok := extractQiniuObjectKey("https://legacy.example.com/not-emoji/a.gif", q); ok {
		t.Fatalf("non-emoji path should not be treated as qiniu key")
	}
}

func TestExtractQiniuObjectKey_ConfiguredRootPrefixFallback(t *testing.T) {
	q := &appstorage.QiniuClient{
		Domain:     "cdn.emoji.icu",
		UseHTTPS:   false,
		RootPrefix: "emoji-prod/",
	}
	key, ok := extractQiniuObjectKey("https://legacy.example.com/emoji-prod/demo/a.gif?x=1", q)
	if !ok {
		t.Fatalf("expected configured-root absolute url to be parsed as key")
	}
	if key != "emoji-prod/demo/a.gif" {
		t.Fatalf("unexpected key: %s", key)
	}
}

func TestResolveDownloadURL_RewriteLegacyAbsoluteURLToCurrentDomain(t *testing.T) {
	q := &appstorage.QiniuClient{
		Mac:      qbox.NewMac("test-ak", "test-sk"),
		Domain:   "cdn.emoji.icu",
		UseHTTPS: false,
		Private:  true,
		SignTTL:  3600,
	}

	now := time.Now().Unix()
	gotURL, exp := resolveDownloadURL("https://old.example.com/emoji/demo/a.gif", q, "60")
	if gotURL == "" {
		t.Fatalf("expected signed url")
	}
	if !strings.HasPrefix(gotURL, "http://cdn.emoji.icu/emoji/demo/a.gif?") {
		t.Fatalf("unexpected signed url domain/path: %s", gotURL)
	}
	if exp <= now {
		t.Fatalf("expected future expiration, got=%d now=%d", exp, now)
	}
}

func TestResolveDownloadURL_KeepUnknownAbsoluteURL(t *testing.T) {
	q := &appstorage.QiniuClient{
		Mac:      qbox.NewMac("test-ak", "test-sk"),
		Domain:   "cdn.emoji.icu",
		UseHTTPS: false,
		Private:  true,
		SignTTL:  3600,
	}

	raw := "https://example.com/files/a.gif"
	gotURL, exp := resolveDownloadURL(raw, q, "60")
	if gotURL != raw {
		t.Fatalf("unexpected rewritten url: %s", gotURL)
	}
	if exp != 0 {
		t.Fatalf("expected exp=0 for passthrough absolute url, got=%d", exp)
	}
}
