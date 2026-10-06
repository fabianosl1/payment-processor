package memcache_test

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/huncoding/payment-processor-course/pkg/memcache"
)

func TestSetGet(t *testing.T) {
	c := memcache.New[string](time.Minute, time.Minute)
	defer c.Close()

	c.Set("k", "v")
	v, ok, err := c.Get("k")
	if err != nil {
		t.Errorf("unexpected error on get: %v", err)
	}
	if !ok {
		t.Fatal("expected key not exist")
	}

	if v != "v" {
		t.Errorf("expected v, got %s", v)
	}
}

func TestExpiration(t *testing.T) {
	c := memcache.New[int](10 * time.Millisecond, time.Hour)
	defer c.Close()

	c.Set("k", 42)
	time.Sleep(30 * time.Millisecond)
	_, ok, err := c.Get("k")
	if err != nil {
		t.Errorf("unexpected error on get: %v", err)
	}
	if ok {
		t.Error("expected key to be expired")
	}
}

func TestIncrementIsAtomic(t *testing.T) {
	c := memcache.New[int](time.Minute, time.Minute)
	defer c.Close()

	add := func(current int, exists bool) int {
		if !exists {
			return 1
		}
		return current + 1
	}

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			c.Increment("card:123", add)
		}()
	}
	wg.Wait()

	v, ok, err := c.Get("card:123")
	if err != nil {
		t.Errorf("unexpected error on get: %v", err)
	}
	if !ok {
		t.Fatal("expected key to exist")
	}
	if v != goroutines {
		t.Errorf("expected %d, got %d", goroutines, v)
	}
}


func TestConcurrentAccessAcrrosShards(t *testing.T) {
	c := memcache.New[int](time.Minute, time.Minute)
	defer c.Close()

	const goroutines = 200
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(val int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", val%50)
			c.Set(key, val)
			_, ok, err := c.Get(key)
			if err != nil {
				t.Errorf("unexpected error on get: %v", err)
			}
			if !ok {
				t.Errorf("expected key %s to exist", key)
			}
		}(i)
	}
	wg.Wait()
}

func TestCloseError(t *testing.T) {
	c := memcache.New[int](time.Minute, time.Minute)
	if err := c.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := c.Close(); !errors.Is(err, memcache.ErrClosed) {
		t.Errorf("expected errClosed, got %v", err)
	}
}

func TestInvalidKeyError(t *testing.T) {
	c := memcache.New[int](time.Minute, time.Minute)
	c.Close()

	if err := c.Set("", 1); !errors.Is(err, memcache.ErrInvalidKey) {
		t.Errorf("expected %v on set, got %v", memcache.ErrInvalidKey, err)
	}

	if _, _, err := c.Get(""); !errors.Is(err, memcache.ErrInvalidKey) {
		t.Errorf("expected %v on get, got %v", memcache.ErrInvalidKey, err)
	}

	if _, err := c.Increment("", func(c int, e bool) int { return 1 }); !errors.Is(err, memcache.ErrInvalidKey) {
		t.Errorf("expected %v on increment, got %v", memcache.ErrInvalidKey, err)
	}
}

func TestJanitorEvictsExpiered(t *testing.T) {
	c := memcache.New[int](10 * time.Millisecond, 1 * time.Millisecond)
	defer c.Close()

	for i := range 100 {
		key := fmt.Sprintf("key-%d", i)
		if err := c.Set(key, i); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if c.Len() == 0 {
			return
		}
		runtime.Gosched()
	}

	t.Errorf("janitor didi not evict expired items within deadline, got %d", c.Len())
}
