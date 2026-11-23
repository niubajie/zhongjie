package admin

import (
	"sync"
	"time"
)

const (
	// 15 分钟内连续失败 5 次锁定 15 分钟，抵御密码与 6 位 TOTP 爆破。
	loginMaxFailures = 5
	loginLockout     = 15 * time.Minute
	loginFailureTTL  = 15 * time.Minute
)

type loginThrottleEntry struct {
	failures    int
	lockedUntil time.Time
	lastSeen    time.Time
}

// loginThrottle 是进程内的登录失败计数器。M1 单实例部署够用；
// 多实例部署时应替换为 Redis 实现，接口保持不变即可。
type loginThrottle struct {
	mu      sync.Mutex
	entries map[string]*loginThrottleEntry
	now     func() time.Time
}

func newLoginThrottle() *loginThrottle {
	return &loginThrottle{entries: make(map[string]*loginThrottleEntry), now: time.Now}
}

// locked 返回剩余锁定时间，0 表示未锁定。
func (t *loginThrottle) locked(key string) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.entries[key]
	if !ok {
		return 0
	}
	now := t.now()
	if e.lockedUntil.After(now) {
		return e.lockedUntil.Sub(now)
	}
	if now.Sub(e.lastSeen) > loginFailureTTL {
		delete(t.entries, key)
	}
	return 0
}

func (t *loginThrottle) fail(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	e, ok := t.entries[key]
	if !ok || now.Sub(e.lastSeen) > loginFailureTTL {
		e = &loginThrottleEntry{}
		t.entries[key] = e
	}
	e.failures++
	e.lastSeen = now
	if e.failures >= loginMaxFailures {
		e.failures = 0
		e.lockedUntil = now.Add(loginLockout)
	}
	t.evict(now)
}

func (t *loginThrottle) reset(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.entries, key)
}

func (t *loginThrottle) evict(now time.Time) {
	for key, e := range t.entries {
		if e.lockedUntil.After(now) {
			continue
		}
		if now.Sub(e.lastSeen) > loginFailureTTL {
			delete(t.entries, key)
		}
	}
}
