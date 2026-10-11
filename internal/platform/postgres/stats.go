package postgres

import (
	"github.com/huncoding/payment-processor-course/internal/platform/metrics"
	"github.com/jackc/pgx/v5/pgxpool"
)

func RegisterPoolMetricts(pool *pgxpool.Pool) {
	metrics.NewGaugeFunc(
		"pgx_pool_total_conns",
		"connections currently open in the pool",
		func() float64 {
		 return float64(pool.Stat().TotalConns() )
		},
	)

	metrics.NewGaugeFunc(
		"pgx_pool_idle_conns",
		"idle connections in the pool",
		func() float64 {
		 return float64(pool.Stat().IdleConns() )
		},
	)

	metrics.NewGaugeFunc(
		"pgx_pool_acquired_conns",
		"Acquired connecctions in the pool",
		func() float64 {
		 return float64(pool.Stat().AcquiredConns() )
		},
	)

	metrics.NewGaugeFunc(
		"pgx_pool_empty_acquire_conns",
		"acquired connections that have been released",
		func() float64 {
			return float64(pool.Stat().EmptyAcquireCount())
		},
	)
}
