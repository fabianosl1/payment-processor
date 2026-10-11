package metrics

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var LatencyBuckets = prometheus.ExponentialBuckets(0.001, 2, 10)

func Handler() http.Handler {
	return promhttp.Handler()
}

func Server(ctx context.Context, addr string, logger *slog.Logger) {
	mux := http.NewServeMux()

	mux.Handle("/metrics", Handler())

	server := http.Server{
		Addr: addr,
		Handler: mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func () {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 3 * time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()

	logger.Info("starting metrics server", "addr", addr)
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		logger.Error("metrics server failed", "err", err)
	}
}

func NewHistogram(name string, help string, labels ...string) *prometheus.HistogramVec {
	return prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: name,
		Help: help,
		Buckets: LatencyBuckets,
	}, labels)
}

func NewCounter(name string, help string, labels ...string) *prometheus.CounterVec {
	return prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: name,
		Help: help,
	}, labels)
}

func NewGauge(name string, help string, labels ... string) *prometheus.GaugeVec {
	return prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: name,
		Help: help,
	}, labels)
}

func NewGaugeFunc(name string, help string, fn func() float64) {
	prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: name,
		Help: help,
	}, fn)
}
