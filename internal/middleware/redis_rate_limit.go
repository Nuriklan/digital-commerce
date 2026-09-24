package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

// rateLimitScript increments counter and sets TTL only if key is new
var rateLimitScript = redis.NewScript(`
local current = redis.call("INCR", KEYS[1])
if current == 1 then
	redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
if current > tonumber(ARGV[2]) then
	return 0
end
return 1
`)

type RedisRateLimiter struct {
	client *redis.Client
	limit  int
	window time.Duration
}

func NewRedisRateLimiter(client *redis.Client, limit int, window time.Duration) *RedisRateLimiter {
	return &RedisRateLimiter{
		client: client,
		limit:  limit,
		window: window,
	}
}

func (rl *RedisRateLimiter) Allow(ctx context.Context, ip string) (bool, error) {
	key := fmt.Sprintf("ratelimit:%s", ip)
	windowMs := rl.window.Milliseconds()

	res, err := rateLimitScript.Run(ctx, rl.client, []string{key}, windowMs, rl.limit).Int()
	if err != nil {
		return true, err
	}

	return res == 1, nil
}

func RedisLimit(rl *RedisRateLimiter) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				ip = r.RemoteAddr
			}

			allowed, err := rl.Allow(r.Context(), ip)
			if err != nil {

			}

			if !allowed {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "too many requests, please slow down",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
