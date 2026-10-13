package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/huncoding/payment-processor-course/internal/fraud/adapter"
	"github.com/huncoding/payment-processor-course/internal/fraud/infra"
	"github.com/huncoding/payment-processor-course/internal/fraud/usecase"
	"github.com/huncoding/payment-processor-course/internal/platform/config"
	"github.com/huncoding/payment-processor-course/internal/platform/logging"
	"github.com/huncoding/payment-processor-course/internal/platform/metrics"
	fraudv1 "github.com/huncoding/payment-processor-course/proto/gen/go/fraud/v1"
	"google.golang.org/grpc"
)

func main() {
	logger := logging.New("fraud")

	if err := run(logger); err != nil {
		logger.Error("service stopped", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	grpcAddr := config.String("GRPC_ADDR", ":9002")
	metricsAddr := config.String("METRICS_ADDR", ":9102")
	modelPath := config.String("MODEL_PATH", "")

	go metrics.Server(ctx, metricsAddr, logger)

	batchSize := config.Int("BATCH_SIZE", 32)
	batchWait := config.Duration("BATCH_WAIT", 5*time.Millisecond)
	model, err := infra.LoadModel(modelPath)
	if err != nil {
		return err
	}
	logger.Info("model loadaded", slog.String("version", model.Version))
	geo := infra.NewHTTPGeoResolver()
	defer geo.Close()

	pipeline := usecase.NewPipeline(
		model,
		geo,
		infra.NewHashingEmbedder(),
		infra.NewSemanticCache(1024, 10*time.Minute, 0.95),
		logger,
		batchSize,
		batchWait,
	)

	defer pipeline.Close()

	listener, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return err
	}

	server := grpc.NewServer(grpc.UnaryInterceptor(metrics.UnaryServerInterceptor()))
	fraudv1.RegisterFraudServiceServer(server, adapter.NewServer(pipeline, logger))

	go func() {
		<-ctx.Done()
		server.GracefulStop()
	}()

	logger.Info("fraud listening", slog.String("addr", grpcAddr))
	return server.Serve(listener)
}
