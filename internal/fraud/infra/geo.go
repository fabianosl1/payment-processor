package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/huncoding/payment-processor-course/pkg/memcache"
)

type GeoResolver interface {
	CountryCode(ctx context.Context, ip string) (string, error)
}

type StaticGeoResolver struct{}

func NewStaticGeoResolver() *StaticGeoResolver {
	return &StaticGeoResolver{}
}

type HTTPGeoResolver struct {
	client *http.Client
	cache *memcache.Cache[string]
}

func NewHTTPGeoResolver() *HTTPGeoResolver {
	return &HTTPGeoResolver{
		client: &http.Client{
			Timeout: 3 * time.Second,
		},
		cache: memcache.New[string](6 * time.Hour, time.Hour),
	}
}

func (r *HTTPGeoResolver) CountryCode(ctx context.Context, ip string) (string, error) {
	if code, ok, _ := r.cache.Get(ip); ok {
		return code, nil
	}

	url := fmt.Sprintf("https://ipapi.co/%s/json", ip)
	response, err := r.client.Get(url)
	if err != nil {
		return "", err
	}

	defer response.Body.Close()

	resp, err := r.client.Do(response.Request)
	if  err != nil {
		return "", fmt.Errorf("geo: failed to get response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("geo: unexpected status codfe: %d", resp.StatusCode)
	}

	var body struct {
		CountryCode string `json:"country_code"`
	}

	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("geo: failed to decode response: %w", err)
	}

	r.cache.Set(ip, body.CountryCode)
	return body.CountryCode, nil
}
