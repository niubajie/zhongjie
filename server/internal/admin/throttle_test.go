package admin

import (
	"testing"
	"time"
)

func TestLoginThrottleLocksAfterMaxFailures(t *testing.T) {
	now := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	throttle := newLoginThrottle()
	throttle.now = func() time.Time { return now }

	for i := 0; i < loginMaxFailures-1; i++ {
		throttle.fail("admin")
	}
	if d := throttle.locked("admin"); d != 0 {
		t.Fatalf("锁定过早，剩余 %v", d)
	}

	throttle.fail("admin")
	if d := throttle.locked("admin"); d <= 0 {
		t.Fatal("达到失败上限后应锁定")
	}

	now = now.Add(loginLockout + time.Second)
	if d := throttle.locked("admin"); d != 0 {
		t.Fatalf("锁定到期后应解除，剩余 %v", d)
	}
}

func TestLoginThrottleResetsOnSuccess(t *testing.T) {
	throttle := newLoginThrottle()
	throttle.fail("admin")
	throttle.fail("admin")
	throttle.reset("admin")

	if d := throttle.locked("admin"); d != 0 {
		t.Fatalf("重置后不应锁定，剩余 %v", d)
	}
	if len(throttle.entries) != 0 {
		t.Fatalf("重置后应清除计数，剩余 %d 条", len(throttle.entries))
	}
}

func TestLoginThrottleExpiresStaleFailures(t *testing.T) {
	now := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	throttle := newLoginThrottle()
	throttle.now = func() time.Time { return now }

	for i := 0; i < loginMaxFailures-1; i++ {
		throttle.fail("admin")
	}
	now = now.Add(loginFailureTTL + time.Minute)
	throttle.fail("admin")

	if d := throttle.locked("admin"); d != 0 {
		t.Fatal("过期失败次数不应累计触发锁定")
	}
}

func TestLoginThrottleIsPerAccount(t *testing.T) {
	throttle := newLoginThrottle()
	for i := 0; i < loginMaxFailures; i++ {
		throttle.fail("admin")
	}
	if d := throttle.locked("reviewer"); d != 0 {
		t.Fatal("不同账号之间不应共享锁定")
	}
}
