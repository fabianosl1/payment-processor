package infra

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type geoRoundTripper func(*http.Request) (*http.Response, error)

func (f geoRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestStaticGeoResolver(t *testing.T) {
	code, err := NewStaticGeoResolver().CountryCode(context.Background(), "127.0.0.1")
	if err != nil || code != "BR" {
		t.Fatalf("CountryCode = %q, %v; want BR, nil", code, err)
	}
}

func TestHTTPGeoResolverCachesSingleRequest(t *testing.T) {
	r := NewHTTPGeoResolver()
	t.Cleanup(func() { r.Close() })
	type contextKey struct{}
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), contextKey{}, "caller"))
	defer cancel()
	calls := 0
	r.client.Transport = geoRoundTripper(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.Context().Value(contextKey{}) != "caller" {
			t.Error("request did not preserve caller context")
		}
		if req.URL.String() != "https://ipapi.co/8.8.8.8/json" {
			t.Errorf("unexpected URL: %s", req.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"country_code":"US"}`))}, nil
	})
	for i := 0; i < 2; i++ {
		code, err := r.CountryCode(ctx, "8.8.8.8")
		if err != nil || code != "US" {
			t.Fatalf("CountryCode = %q, %v; want US, nil", code, err)
		}
	}
	if calls != 1 {
		t.Fatalf("HTTP requests = %d; want 1", calls)
	}
}

func TestHTTPGeoResolverErrorsAreNotCached(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		err    error
	}{
		{name: "transport", err: context.Canceled},
		{name: "status", status: http.StatusTooManyRequests, body: `{}`},
		{name: "invalid JSON", status: http.StatusOK, body: `invalid`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := NewHTTPGeoResolver()
			t.Cleanup(func() { r.Close() })
			calls := 0
			r.client.Transport = geoRoundTripper(func(*http.Request) (*http.Response, error) {
				calls++
				if tc.err != nil {
					return nil, tc.err
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			for i := 0; i < 2; i++ {
				code, err := r.CountryCode(context.Background(), "8.8.8.8")
				if err == nil || code != "" {
					t.Fatalf("CountryCode = %q, %v; want empty code and error", code, err)
				}
				if tc.err != nil && !errors.Is(err, tc.err) {
					t.Fatalf("error = %v; want wrapped %v", err, tc.err)
				}
			}
			if calls != 2 {
				t.Fatalf("HTTP requests = %d; want 2", calls)
			}
		})
	}
}
