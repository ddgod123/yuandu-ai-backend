package storage

import (
	"errors"
	"strings"
	"time"

	"emoji/internal/config"

	"github.com/qiniu/go-sdk/v7/auth/qbox"
	qiniustorage "github.com/qiniu/go-sdk/v7/storage"
)

type QiniuClient struct {
	Mac      *qbox.Mac
	Cfg      *qiniustorage.Config
	Bucket   string
	Domain   string
	UseHTTPS bool
	Private  bool
	SignTTL  int
}

func NewQiniuClient(cfg config.Config) (*QiniuClient, error) {
	ak := strings.TrimSpace(cfg.QiniuAccessKey)
	sk := strings.TrimSpace(cfg.QiniuSecretKey)
	bucket := strings.TrimSpace(cfg.QiniuBucket)
	if ak == "" || sk == "" || bucket == "" {
		return nil, errors.New("missing qiniu access key/secret key/bucket")
	}

	qcfg := qiniustorage.NewConfig()
	qcfg.UseHTTPS = cfg.QiniuUseHTTPS
	qcfg.UseCdnDomains = cfg.QiniuUseCDN

	if zone := zoneFromString(strings.ToLower(strings.TrimSpace(cfg.QiniuZone))); zone != nil {
		qcfg.Zone = zone
	} else {
		zone, err := qiniustorage.GetZone(ak, bucket)
		if err != nil {
			return nil, err
		}
		qcfg.Zone = zone
	}

	return &QiniuClient{
		Mac:      qbox.NewMac(ak, sk),
		Cfg:      qcfg,
		Bucket:   bucket,
		Domain:   strings.TrimSpace(cfg.QiniuDomain),
		UseHTTPS: cfg.QiniuUseHTTPS,
		Private:  cfg.QiniuPrivate,
		SignTTL:  cfg.QiniuSignTTL,
	}, nil
}

func (q *QiniuClient) BucketManager() *qiniustorage.BucketManager {
	return qiniustorage.NewBucketManager(q.Mac, q.Cfg)
}

func (q *QiniuClient) PublicURL(key string) string {
	domain := strings.TrimSpace(q.Domain)
	if domain == "" {
		return key
	}
	if !strings.HasPrefix(domain, "http://") && !strings.HasPrefix(domain, "https://") {
		if q.UseHTTPS {
			domain = "https://" + domain
		} else {
			domain = "http://" + domain
		}
	}
	domain = strings.TrimRight(domain, "/")
	key = strings.TrimLeft(key, "/")
	return domain + "/" + key
}

func (q *QiniuClient) PublicURLWithQuery(key string, query string) string {
	base := q.PublicURL(strings.TrimLeft(strings.TrimSpace(key), "/"))
	query = strings.TrimPrefix(strings.TrimSpace(query), "?")
	if query == "" {
		return base
	}
	separator := "?"
	if strings.Contains(base, "?") {
		separator = "&"
	}
	return base + separator + query
}

func (q *QiniuClient) SignedURL(key string, ttl int64) (string, error) {
	domain := strings.TrimSpace(q.Domain)
	if domain == "" {
		return "", errors.New("qiniu domain is required for signed url")
	}
	if ttl <= 0 {
		ttl = int64(q.SignTTL)
	}
	if ttl <= 0 {
		ttl = 3600
	}
	deadline := ttl + nowUnix()
	return qiniustorage.MakePrivateURLv2(q.Mac, normalizeDomain(domain, q.UseHTTPS), key, deadline), nil
}

func (q *QiniuClient) SignedURLWithQuery(key string, query string, ttl int64) (string, error) {
	domain := strings.TrimSpace(q.Domain)
	if domain == "" {
		return "", errors.New("qiniu domain is required for signed url")
	}
	key = strings.TrimLeft(strings.TrimSpace(key), "/")
	if key == "" {
		return "", errors.New("qiniu key is required for signed url")
	}
	query = strings.TrimPrefix(strings.TrimSpace(query), "?")
	if ttl <= 0 {
		ttl = int64(q.SignTTL)
	}
	if ttl <= 0 {
		ttl = 3600
	}
	deadline := ttl + nowUnix()
	domain = normalizeDomain(domain, q.UseHTTPS)
	if query == "" {
		return qiniustorage.MakePrivateURLv2(q.Mac, domain, key, deadline), nil
	}
	return qiniustorage.MakePrivateURLv2WithQueryString(q.Mac, domain, key, query, deadline), nil
}

func nowUnix() int64 {
	return timeNow().Unix()
}

var timeNow = func() time.Time { return time.Now() }

func normalizeDomain(domain string, useHTTPS bool) string {
	domain = strings.TrimRight(domain, "/")
	if strings.HasPrefix(domain, "http://") || strings.HasPrefix(domain, "https://") {
		return domain
	}
	if useHTTPS {
		return "https://" + domain
	}
	return "http://" + domain
}

func zoneFromString(val string) *qiniustorage.Zone {
	switch val {
	case "z0", "huadong", "cn-east-1":
		return &qiniustorage.ZoneHuadong
	case "z1", "huabei", "cn-north-1":
		return &qiniustorage.ZoneHuabei
	case "z2", "huanan", "cn-south-1":
		return &qiniustorage.ZoneHuanan
	case "na0", "beimei", "north-america", "us":
		return &qiniustorage.ZoneBeimei
	case "as0", "xinjiapo", "singapore", "ap-southeast-1":
		return &qiniustorage.ZoneXinjiapo
	case "huadong-zhejiang2", "zhejiang2", "z0zj2":
		return &qiniustorage.ZoneHuadongZheJiang2
	default:
		return nil
	}
}
