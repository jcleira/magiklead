package redisx

import (
	"testing"

	"github.com/hibiken/asynq"
)

func TestConnOpt(t *testing.T) {
	// Bare host:port — the form the CI e2e workflow sets (localhost:6379).
	t.Run("bare host:port", func(t *testing.T) {
		opt, err := ConnOpt("localhost:6379")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		c, ok := opt.(asynq.RedisClientOpt)
		if !ok {
			t.Fatalf("want RedisClientOpt, got %T", opt)
		}
		if c.Addr != "localhost:6379" {
			t.Fatalf("Addr = %q, want localhost:6379", c.Addr)
		}
	})

	// redis:// URL — the form devpods generates (redis://redis:6379/0).
	t.Run("redis:// URL", func(t *testing.T) {
		opt, err := ConnOpt("redis://redis:6379/0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		c, ok := opt.(asynq.RedisClientOpt)
		if !ok {
			t.Fatalf("want RedisClientOpt, got %T", opt)
		}
		if c.Addr != "redis:6379" {
			t.Fatalf("Addr = %q, want redis:6379", c.Addr)
		}
		if c.DB != 0 {
			t.Fatalf("DB = %d, want 0", c.DB)
		}
	})

	t.Run("empty is an error", func(t *testing.T) {
		if _, err := ConnOpt("  "); err == nil {
			t.Fatal("want error for empty REDIS_URL")
		}
	})
}
