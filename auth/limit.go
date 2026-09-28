package auth

import (
	"sync"
	"time"
)

// Guessing a password is slowed down per login: loginAttempts tries in a
// window, after that the login is refused until the window ends, without the
// store being asked at all. Every login counts, existing or not, so a refusal
// tells nothing about which ones exist. Signing in successfully clears the
// login's count: a few typos before it do not pile up. A browser that has
// signed in at the login before has a count of its own, see Auth.attemptKey.
const (
	loginAttempts = 5
	loginWindow   = 15 * time.Minute
	// the table is kept within bounds whatever is typed into the form
	loginTracked = 100_000
	loginKeyMax  = 64
)

// limiter counts attempts in a fixed window shared by all logins: the whole
// table is cleared once the window is over, so nothing needs sweeping one entry
// at a time.
//
// ponytail: in-process and per login only. Several replicas need a shared
// store; a table filled with loginTracked junk logins lets new logins through
// uncounted until the window ends, and spraying one password over many logins
// is not caught at all — both are for a per-address limit in front.
type limiter struct {
	mu       sync.Mutex
	attempts map[string]int
	reset    time.Time
}

// try counts an attempt under key and tells whether it may go ahead; when not,
// wait is how long until it may, and first says this is the first refusal in
// the window — the one worth a line in the log.
func (l *limiter) try(key string) (ok bool, wait time.Duration, first bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	if now.After(l.reset) {
		l.attempts, l.reset = map[string]int{}, now.Add(loginWindow)
	}
	if _, seen := l.attempts[key]; !seen && len(l.attempts) >= loginTracked {
		return true, 0, false
	}
	// counted before the password is checked: parallel requests cannot all slip
	// in while none of them has failed yet
	l.attempts[key]++
	if l.attempts[key] > loginAttempts {
		return false, l.reset.Sub(now), l.attempts[key] == loginAttempts+1
	}
	return true, 0, false
}

// forget clears the count of a key that has signed in.
func (l *limiter) forget(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}
