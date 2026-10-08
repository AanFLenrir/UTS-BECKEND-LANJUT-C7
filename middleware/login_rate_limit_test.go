package middleware

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

func TestLoginRateLimitBlocksAfterFiveFailedAttemptsAndExpires(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	limiter := newLoginLimiter(func() time.Time { return now })
	app := fiber.New()
	app.Post("/login", limiter.handle, func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"success": false})
	})

	for i := 0; i < loginAttemptLimit; i++ {
		if status, _ := postTest(app); status != fiber.StatusUnauthorized {
			t.Fatalf("failed attempt %d status = %d, want 401", i+1, status)
		}
	}
	status, retryAfter := postTest(app)
	if status != fiber.StatusTooManyRequests || retryAfter == "" {
		t.Fatalf("blocked attempt status=%d Retry-After=%q, want 429 with Retry-After", status, retryAfter)
	}

	now = now.Add(loginAttemptWindow)
	if status, _ := postTest(app); status != fiber.StatusUnauthorized {
		t.Fatalf("attempt after window status = %d, want 401", status)
	}
}

func TestLoginRateLimitSuccessfulLoginResetsCounter(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	limiter := newLoginLimiter(func() time.Time { return now })
	succeed := false
	app := fiber.New()
	app.Post("/login", limiter.handle, func(c *fiber.Ctx) error {
		if succeed {
			return c.SendStatus(fiber.StatusOK)
		}
		return c.SendStatus(fiber.StatusUnauthorized)
	})
	for i := 0; i < 2; i++ {
		if status, _ := postTest(app); status != fiber.StatusUnauthorized {
			t.Fatalf("failed attempt status = %d, want 401", status)
		}
	}
	succeed = true
	if status, _ := postTest(app); status != fiber.StatusOK {
		t.Fatalf("successful login status = %d, want 200", status)
	}
	succeed = false
	for i := 0; i < loginAttemptLimit; i++ {
		if status, _ := postTest(app); status != fiber.StatusUnauthorized {
			t.Fatalf("post-success failed attempt %d status = %d, want 401", i+1, status)
		}
	}
	if status, _ := postTest(app); status != fiber.StatusTooManyRequests {
		t.Fatalf("attempt after five post-success failures status = %d, want 429", status)
	}
}

func TestLoginRateLimitSerializesConcurrentAttemptsPerIP(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	limiter := newLoginLimiter(func() time.Time { return now })
	app := fiber.New()
	app.Post("/login", limiter.handle, func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusUnauthorized)
	})

	const total = 20
	statuses := make(chan int, total)
	var wg sync.WaitGroup
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest(http.MethodPost, "/login", nil)
			response, err := app.Test(req)
			if err != nil {
				statuses <- 0
				return
			}
			defer response.Body.Close()
			statuses <- response.StatusCode
		}()
	}
	wg.Wait()
	close(statuses)
	unauthorized, limited := 0, 0
	for status := range statuses {
		switch status {
		case fiber.StatusUnauthorized:
			unauthorized++
		case fiber.StatusTooManyRequests:
			limited++
		default:
			t.Fatalf("concurrent request status = %d", status)
		}
	}
	if unauthorized != loginAttemptLimit || limited != total-loginAttemptLimit {
		t.Fatalf("concurrent responses: 401=%d 429=%d; want 401=%d 429=%d", unauthorized, limited, loginAttemptLimit, total-loginAttemptLimit)
	}
}

func postTest(app *fiber.App) (int, string) {
	req, _ := http.NewRequest(http.MethodPost, "/login", nil)
	response, err := app.Test(req)
	if err != nil {
		panic(err)
	}
	defer response.Body.Close()
	return response.StatusCode, response.Header.Get(fiber.HeaderRetryAfter)
}
