package handlers

import (
	"net"
	"strings"
	"sync"

	"emoji/internal/config"

	"github.com/oschwald/geoip2-golang"
)

type geoIPLookupResult struct {
	Country string
	Region  string
	City    string
	Source  string
}

var (
	geoIPReaderOnce sync.Once
	geoIPReader     *geoip2.Reader
	geoIPReaderErr  error
)

func lookupGeoByIP(cfg config.Config, rawIP string) geoIPLookupResult {
	if !cfg.GeoIPEnabled {
		return geoIPLookupResult{}
	}
	ip := parseClientIP(rawIP)
	if ip == nil || shouldSkipGeoIP(ip) {
		return geoIPLookupResult{}
	}

	reader, err := loadGeoIPReader(cfg)
	if err != nil || reader == nil {
		return geoIPLookupResult{}
	}

	record, err := reader.City(ip)
	if err != nil || record == nil {
		return geoIPLookupResult{}
	}

	result := geoIPLookupResult{
		Country: geoIPName(record.Country.Names, record.Country.IsoCode),
		Source:  "geoip_mmdb",
	}
	if len(record.Subdivisions) > 0 {
		result.Region = geoIPName(record.Subdivisions[0].Names, record.Subdivisions[0].IsoCode)
	}
	result.City = geoIPName(record.City.Names, "")

	if result.Country == "" && result.Region == "" && result.City == "" {
		return geoIPLookupResult{}
	}
	return result
}

func loadGeoIPReader(cfg config.Config) (*geoip2.Reader, error) {
	geoIPReaderOnce.Do(func() {
		path := strings.TrimSpace(cfg.GeoIPMMDBPath)
		if path == "" {
			return
		}
		reader, err := geoip2.Open(path)
		if err != nil {
			geoIPReaderErr = err
			return
		}
		geoIPReader = reader
	})
	return geoIPReader, geoIPReaderErr
}

func parseClientIP(raw string) net.IP {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}
	if idx := strings.Index(value, ","); idx >= 0 {
		value = strings.TrimSpace(value[:idx])
	}
	ip := net.ParseIP(value)
	if ip != nil {
		return ip
	}

	host, _, err := net.SplitHostPort(value)
	if err == nil {
		return net.ParseIP(strings.TrimSpace(host))
	}
	return nil
}

func shouldSkipGeoIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if ip.IsPrivate() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 169 && ip4[1] == 254 {
			return true
		}
	}
	return false
}

func geoIPName(names map[string]string, fallback string) string {
	if len(names) > 0 {
		for _, key := range []string{"zh-CN", "zh", "en"} {
			if value := strings.TrimSpace(names[key]); value != "" {
				return value
			}
		}
		for _, value := range names {
			if v := strings.TrimSpace(value); v != "" {
				return v
			}
		}
	}
	return strings.TrimSpace(fallback)
}
