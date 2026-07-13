// Package redisx normalizes a REDIS_URL into an asynq connection option.
// Environments disagree on the format: devpods emits a redis:// URL
// (redis://redis:6379/0) while the CI workflow sets a bare host:port
// (localhost:6379). asynq.ParseRedisURI accepts only the URL form and
// RedisClientOpt.Addr only the bare form, so a caller that hardcodes one
// breaks the other. ConnOpt accepts both.
package redisx

import (
	"fmt"
	"strings"

	"github.com/hibiken/asynq"
)

// ConnOpt turns a REDIS_URL into an asynq.RedisConnOpt. A value carrying a
// scheme (redis:// or rediss://) is parsed as a URL; a bare host:port is
// used directly as the dial address.
func ConnOpt(raw string) (asynq.RedisConnOpt, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("REDIS_URL is empty")
	}
	if strings.Contains(raw, "://") {
		opt, err := asynq.ParseRedisURI(raw)
		if err != nil {
			return nil, fmt.Errorf("parse REDIS_URL %q: %w", raw, err)
		}
		return opt, nil
	}
	return asynq.RedisClientOpt{Addr: raw}, nil
}
