package middleware

import (
	"sync"
	"time"
)

type loginAttempt struct {
	failures     int
	blockedUntil time.Time
	lastSeen     time.Time
}

// LoginAttemptGuard 对 IP+用户名组合进行短期封禁。
type LoginAttemptGuard struct {
	mu            sync.Mutex
	attempts      map[string]*loginAttempt
	failureLimit  int
	blockDuration time.Duration
	now           func() time.Time
	nextSweep     time.Time
}

func NewLoginAttemptGuard(failureLimit int, blockDuration time.Duration) *LoginAttemptGuard {
	return &LoginAttemptGuard{
		attempts: make(map[string]*loginAttempt), failureLimit: failureLimit,
		blockDuration: blockDuration, now: time.Now,
	}
}

func (g *LoginAttemptGuard) Allow(key string) (bool, time.Duration) {
	now := g.now()
	g.mu.Lock()
	defer g.mu.Unlock()
	g.sweep(now)
	attempt := g.attempts[key]
	if attempt == nil || !now.Before(attempt.blockedUntil) {
		if attempt != nil && !attempt.blockedUntil.IsZero() {
			delete(g.attempts, key)
		}
		return true, 0
	}
	attempt.lastSeen = now
	return false, attempt.blockedUntil.Sub(now)
}

func (g *LoginAttemptGuard) Failure(key string) {
	now := g.now()
	g.mu.Lock()
	defer g.mu.Unlock()
	g.sweep(now)
	attempt := g.attempts[key]
	if attempt == nil {
		attempt = &loginAttempt{}
		g.attempts[key] = attempt
	}
	attempt.failures++
	attempt.lastSeen = now
	if attempt.failures >= g.failureLimit {
		attempt.blockedUntil = now.Add(g.blockDuration)
	}
}

func (g *LoginAttemptGuard) Success(key string) {
	g.mu.Lock()
	delete(g.attempts, key)
	g.mu.Unlock()
}

func (g *LoginAttemptGuard) sweep(now time.Time) {
	if !g.nextSweep.IsZero() && now.Before(g.nextSweep) {
		return
	}
	for key, attempt := range g.attempts {
		if now.Sub(attempt.lastSeen) > 2*g.blockDuration {
			delete(g.attempts, key)
		}
	}
	g.nextSweep = now.Add(time.Minute)
}
