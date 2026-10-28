package web

import (
	"sync"
	"time"
)

const (
	maxLoginFehler    = 10
	loginFensterDauer = time.Minute
	demoResetAbstand  = 10 * time.Second
)

// loginLimit throttles password guessing and the demo reset. The app is only
// reached through Home Assistant Ingress, so all requests share one source
// address and the limit is global, not per client.
//
// shortcut: in-memory, resets on restart. While locked, the real Vermieter is
// locked out too (at most loginFensterDauer); upgrade to per-client limits if
// the app is ever exposed directly.
type loginLimit struct {
	mu          sync.Mutex
	now         func() time.Time
	fenster     time.Time // start of the current failure window
	fehler      int
	letzterDemo time.Time
}

func newLoginLimit(now func() time.Time) *loginLimit {
	return &loginLimit{now: now}
}

// gesperrt reports whether password logins are refused right now and, if so,
// how long until the window passes.
func (l *loginLimit) gesperrt() (bool, time.Duration) {
	if l == nil {
		return false, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	rest := l.fenster.Add(loginFensterDauer).Sub(l.now())
	if l.fehler >= maxLoginFehler && rest > 0 {
		return true, rest
	}
	return false, 0
}

// fehlversuch records one failed password login.
func (l *loginLimit) fehlversuch() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if now := l.now(); l.fehler == 0 || !now.Before(l.fenster.Add(loginFensterDauer)) {
		l.fenster, l.fehler = now, 0
	}
	l.fehler++
}

// demoResetErlaubt reports whether the demo database may be reset now, and
// counts the reset when it does.
func (l *loginLimit) demoResetErlaubt() bool {
	if l == nil {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if !l.letzterDemo.IsZero() && now.Sub(l.letzterDemo) < demoResetAbstand {
		return false
	}
	l.letzterDemo = now
	return true
}
