// Package batch implements a dynamic batcher.
package batch

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrClosed = errors.New("batch: batcher is closed")

type Flusher[In, Out any] func(ctx context.Context, items []In) ([]Out, error)

type Batcher[In, Out any] struct {
	maxSize int
	maxWait time.Duration
	flush   Flusher[In, Out]
	mu      sync.Mutex

	queue  chan request[In, Out]
	closed bool
	wg     sync.WaitGroup
}

type request[In, Out any] struct {
	item In
	resp chan result[Out]
}

type result[Out any] struct {
	err   error
	value Out
}

func New[In, Out any](maxSize int, maxWait time.Duration, flush Flusher[In, Out]) *Batcher[In, Out] {
	if maxSize < 1 {
		maxSize = 1
	}

	b := &Batcher[In, Out]{
		maxSize: maxSize,
		maxWait: maxWait,
		flush:   flush,
		queue:   make(chan request[In, Out], maxSize),
	}

	b.wg.Add(1)
	go b.loop()
	return b
}

func (b *Batcher[In, Out]) Submit(ctx context.Context, in In) (Out, error) {
	var zero Out


	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return zero, ErrClosed
	}

	// Allow dispatch to deliver the result even if the caller's context expires.
	req := request[In, Out]{item: in, resp: make(chan result[Out], 1)}
	b.queue <- req
	b.mu.Unlock()

	select {
	case res := <-req.resp:
		return res.value, res.err
	case <-ctx.Done():
		return zero, ctx.Err()
	}
}

func (b *Batcher[In, Out]) Close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}

	b.closed = true
	close(b.queue)
	b.mu.Unlock()

	b.wg.Wait()
}

func (b *Batcher[In, Out]) dispatch(batch []request[In, Out]) {
	items := make([]In, len(batch))

	for i, req := range batch {
		items[i] = req.item
	}

	outs, err := b.flush(context.Background(), items)
	if err != nil || len(outs) != len(batch) {
		err = errors.New("batch: flusher returned wrong number of results")
	}

	for i, req := range batch {
		if err != nil {
			var zero Out
			req.resp <- result[Out]{err: err, value: zero}
			continue
		}
		req.resp <- result[Out]{err: nil, value: outs[i]}
	}
}

func (b *Batcher[In, Out]) loop() {
	defer b.wg.Done()
	pending := make([]request[In, Out], 0, b.maxSize)
	timer := time.NewTimer(b.maxWait)
	defer timer.Stop()

	flushPending := func() {
		if len(pending) == 0 {
			return
		}
		batch := pending
		pending = make([]request[In, Out], 0, b.maxSize)
		b.dispatch(batch)
	}

	for {
		select {
		case req, ok := <-b.queue:
			if !ok {
				flushPending()
				return
			}

			pending = append(pending, req)
			if len(pending) == 1 {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(b.maxWait)
			}

			if len(pending) >= b.maxSize {
				flushPending()
			}
		case <-timer.C:
			flushPending()
		}
	}
}
