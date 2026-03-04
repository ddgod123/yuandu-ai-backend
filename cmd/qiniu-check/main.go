package main

import (
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/qiniu/go-sdk/v7/auth/qbox"
	"github.com/qiniu/go-sdk/v7/storage"
)

func main() {
	_ = godotenv.Overload()
	_ = godotenv.Overload("backend/.env")

	ak := strings.TrimSpace(os.Getenv("QINIU_ACCESS_KEY"))
	sk := strings.TrimSpace(os.Getenv("QINIU_SECRET_KEY"))
	bucket := strings.TrimSpace(os.Getenv("QINIU_BUCKET"))
	if ak == "" || sk == "" || bucket == "" {
		log.Fatal("missing QINIU_ACCESS_KEY/QINIU_SECRET_KEY/QINIU_BUCKET")
	}

	zoneName := strings.ToLower(strings.TrimSpace(os.Getenv("QINIU_ZONE")))
	useHTTPS := parseBool(os.Getenv("QINIU_USE_HTTPS"), true)
	useCDN := parseBool(os.Getenv("QINIU_USE_CDN"), false)

	cfg := storage.NewConfig()
	cfg.UseHTTPS = useHTTPS
	cfg.UseCdnDomains = useCDN

	if zone := zoneFromString(zoneName); zone != nil {
		cfg.Zone = zone
	} else {
		z, err := storage.GetZone(ak, bucket)
		if err != nil {
			log.Fatalf("get zone failed: %v", err)
		}
		cfg.Zone = z
	}

	mac := qbox.NewMac(ak, sk)
	bm := storage.NewBucketManager(mac, cfg)

	prefix := "emoji/"
	entries, _, _, hasNext, err := bm.ListFiles(bucket, prefix, "", "", 1)
	if err != nil {
		log.Fatalf("qiniu list failed: %v", err)
	}

	log.Printf("qiniu ok: bucket=%s prefix=%s items=%d hasNext=%v", bucket, prefix, len(entries), hasNext)
}

func parseBool(val string, def bool) bool {
	val = strings.TrimSpace(strings.ToLower(val))
	switch val {
	case "1", "true", "yes", "y":
		return true
	case "0", "false", "no", "n":
		return false
	default:
		return def
	}
}

func zoneFromString(val string) *storage.Zone {
	switch val {
	case "z0", "huadong", "cn-east-1":
		return &storage.ZoneHuadong
	case "z1", "huabei", "cn-north-1":
		return &storage.ZoneHuabei
	case "z2", "huanan", "cn-south-1":
		return &storage.ZoneHuanan
	case "na0", "beimei", "north-america", "us":
		return &storage.ZoneBeimei
	case "as0", "xinjiapo", "singapore", "ap-southeast-1":
		return &storage.ZoneXinjiapo
	case "huadong-zhejiang2", "zhejiang2", "z0zj2":
		return &storage.ZoneHuadongZheJiang2
	default:
		return nil
	}
}
