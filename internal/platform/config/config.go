package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

func String(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return def
}

func Int(key string) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}

	return 0
}

func Duration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}

	return def
}

func MustString(key, def string) string {
	v := os.Getenv(key)

	if v == "" {
		fmt.Fprintf(os.Stderr, "Missing required config: %s", key)
		os.Exit(1)
	}

	return v
}
