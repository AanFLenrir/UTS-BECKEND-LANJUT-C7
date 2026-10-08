package middleware

import (
	"strconv"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

const (
	loginAttemptLimit  = 5
	loginAttemptWindow = time.Minute
)

type loginAttempt struct {
	count     int
	startedAt time.Time
}

type loginIPState struct {
	mu    sync.Mutex
	state loginAttempt
}

type loginIPEntry struct {
	limiter  *loginIPState
	refs     int
	lastSeen time.Time
}

type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string]*loginIPEntry
	now      func() time.Time
}

func newLoginLimiter(now func() time.Time) *loginLimiter {
	return &loginLimiter{attempts: make(map[string]*loginIPEntry), now: now}
}

// LoginRateLimit allows five failed attempts per IP in each one-minute window.
// Only 401 responses count; validation errors and successful logins do not.
func LoginRateLimit() fiber.Handler {
	return newLoginLimiter(time.Now).handle
}

func (l *loginLimiter) handle(c *fiber.Ctx) error {
	ip := c.IP()
	l.mu.Lock()
	now := l.now()
	for key, entry := range l.attempts {
		if entry.refs == 0 && now.Sub(entry.lastSeen) >= 2*loginAttemptWindow {
			delete(l.attempts, key)
		}
	}
	entry := l.attempts[ip]
	if entry == nil {
		entry = &loginIPEntry{limiter: &loginIPState{}}
		l.attempts[ip] = entry
	}
	entry.refs++
	l.mu.Unlock()

	entry.limiter.mu.Lock()
	current := entry.limiter.state
	if current.count > 0 && now.Sub(current.startedAt) >= loginAttemptWindow {
		current = loginAttempt{}
	}
	if current.count >= loginAttemptLimit {
		retryAfter := current.startedAt.Add(loginAttemptWindow).Sub(now)
		if retryAfter < time.Second {
			retryAfter = time.Second
		}
		entry.limiter.mu.Unlock()
		l.release(ip, entry, now)
		c.Set(fiber.HeaderRetryAfter, strconv.Itoa(int(retryAfter.Round(time.Second)/time.Second)))
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{"success": false, "message": "terlalu banyak percobaan login; coba lagi nanti"})
	}

	err := c.Next()
	status := c.Response().StatusCode()
	if status == fiber.StatusOK {
		current = loginAttempt{}
	} else if status == fiber.StatusUnauthorized {
		if current.count == 0 {
			current.startedAt = now
		}
		current.count++
	}
	entry.limiter.state = current
	entry.limiter.mu.Unlock()
	l.release(ip, entry, now)
	return err
}

func (l *loginLimiter) release(ip string, entry *loginIPEntry, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry.refs--
	entry.lastSeen = now
}
