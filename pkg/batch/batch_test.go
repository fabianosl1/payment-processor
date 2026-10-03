package batch_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/huncoding/payment-processor-course/pkg/batch"
)

func  TestSubmitAfterClose(t *testing.T) {
	echo := func(_ context.Context, items[]int) ([]int, error) { return items, nil}
	b := batch.New(22, time.Millisecond, echo)
	b.Close()

	_, err := b.Submit(context.Background(), 1)
	if !errors.Is(err, batch.ErrClosed) {
		t.Fatalf("expected errClosed, got %v", err)
	}
}

func TestBatchBySize(t *testing.T) {
	var calls atomic.Int32
	double := func(_ context.Context, items[]int) ([]int, error) {
		calls.Add(1)
		out := make([]int, len(items))
		for i, v := range items {
			out[i] = v * 2
		}
		return out, nil
	}

	b := batch.New(4, time.Second, double)
	defer b.Close()

	results := make([]int, 8)
	var wg sync.WaitGroup

	for i := range 8 {
		wg.Go(func() {
			res, err := b.Submit(t.Context(), i)
			if err != nil {
				t.Errorf("Submit failed for %d: %v", i, err)
				return
			}
			results[i] = res
		})
	}

	wg.Wait()

	for i := range 8 {
		if results[i] != i * 2 {
			t.Errorf("results[%d] = %d expected %d", i, results[i], i * 2)
		}
	}

	if count := calls.Load(); count > 4 {
		t.Errorf("expected at most 4 flusher calls, got %d", count)
	}
}

func TestFlushesByTimeWindow(t *testing.T) {
	echo := func(_ context.Context, items[]string) ([]string, error) {
		return items, nil
	}

	b := batch.New(100, 20 * time.Millisecond, echo)
	defer b.Close()

	start := time.Now()
	res, err := b.Submit(t.Context(), "lonely")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	if res != "lonely" {
		t.Errorf("expected 'lonely', got %q", res)
	}
	if elapsed >= 500 * time.Millisecond {
		t.Errorf("expected elpased time < 500ms, got %v", elapsed)
	}
}

func TestContextCancelUnblocksCaller(t *testing.T) {
	slow := func(_ context.Context, items []int) ([]int, error) {
		time.Sleep(200 * time.Millisecond)
		return items, nil
	}
	b := batch.New(10, 50 * time.Millisecond, slow)
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10 * time.Millisecond)
	defer cancel()

	_, err := b.Submit(ctx, 1)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline error, got: %v", err)
	}

}
