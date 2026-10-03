// Package memcache is a simple memory store.
package memcache

import (
	"errors"
	"hash/fnv"
	"sync"
	"sync/atomic"
	"time"
)

var ErrInvalidKey = errors.New("Invalid key")
var ErrClosed = errors.New("closed")

const shardCount = 16

type entry[V any] struct {
	value     V
	expiresAt time.Time
}

type shard[V any] struct {
	mu    sync.RWMutex
	items map[string]entry[V]
}

type Cache[V any] struct {
	shards [shardCount]*shard[V] // slice nao é um array
	ttl    time.Duration
	now    func() time.Time
	stop   chan struct{}
	once   sync.Once
	closed atomic.Bool
}

func New[V any](ttl, sweepEvery time.Duration) *Cache[V] {
	c := &Cache[V]{
		ttl:  ttl,
		now:  time.Now,
		stop: make(chan struct{}),
	}
	for i := range c.shards {
		c.shards[i] = &shard[V]{items: make(map[string]entry[V])}
	}
	go c.janitor(sweepEvery)
	return c
}

func (c *Cache[V]) shardFor(key string) *shard[V] {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return c.shards[h.Sum32()%shardCount]
}

func (c *Cache[V]) SetTTL(key string, value V, ttl time.Duration) error {
	if key == "" {
		return ErrInvalidKey
	}
	if c.closed.Load() {
		return ErrClosed
	}

	s := c.shardFor(key)
	s.mu.Lock()
	defer s.mu.Unlock()

	s.items[key] = entry[V]{value: value, expiresAt: time.Now().Add(ttl)}

	return nil
}

func (c *Cache[V]) Set(key string, value V) error {
	if key == "" {
		return ErrInvalidKey
	}
	if c.closed.Load() {
		return  ErrClosed
	}

	c.SetTTL(key, value, c.ttl)
	return nil
}

func (c *Cache[V]) Get(key string) (V, bool, error) {
	if key == "" {
		var zero V
		return zero, false, ErrInvalidKey
	}
	if c.closed.Load() {
		var zero V
		return zero, false, ErrClosed
	}

	s := c.shardFor(key)
	s.mu.RLock()
	entry, ok := s.items[key]
	s.mu.RUnlock()

	if !ok || c.now().After(entry.expiresAt) {
		var zero V
		return zero, false, nil
	}

	return entry.value, true, nil
}

// Increment adds delta to an integer counter stored at key and returns
// the new total. Counters share the default TTL, refreshed on write.
// Its is the primitive behind velociy checks
func (c *Cache[V]) Increment(key string, counter func(current V, exists bool) V) (V, error) {
	if key == "" {
		var zero V
		return zero, ErrInvalidKey
	}
	if c.closed.Load() {
		var zero V
		return zero, ErrClosed
	}
	s := c.shardFor(key)
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.items[key]
	valid := ok && !c.now().After(e.expiresAt)

	next := counter(e.value, valid)
	s.items[key] = entry[V]{value: next, expiresAt: c.now().Add(c.ttl)}
	return next, nil
}

func (c *Cache[V]) Len() int {
	var total int
	for _, s := range c.shards {
		s.mu.RLock()
		total += len(s.items)
		s.mu.RUnlock()
	}
	return total
}

func (c *Cache[V]) Close() error {
	if c.closed.Load() {
		return ErrClosed
	}
	c.once.Do(func() {
		c.closed.Store(true)
		close(c.stop)
	})
	return nil
}

func (c *Cache[V]) janitor(every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()

	for {
		select {
		case <-c.stop:
			return
		case <-ticker.C:
			now := c.now()
			for _, s := range c.shards {
				s.mu.Lock()
				for k, v := range s.items {
					if now.After(v.expiresAt) {
						delete(s.items, k)
					}
				}
				s.mu.Unlock()
			}
		}
	}
}
