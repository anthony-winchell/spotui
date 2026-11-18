package ui

import "time"

// pending tracks a local change Spotify hasn't confirmed yet.
// While active, our value wins over the fetched one
type pending struct {
	seq   int       // bumped on every change; flush messages carry it to debounce
	until time.Time // stop overriding after this; zero = nothing pending
}

// start records a new change that stays active for d, and returns its sequence number
func (p *pending) start(d time.Duration) int {
	p.seq++
	p.until = time.Now().Add(d)
	return p.seq
}

// active reports whether the change is still waiting for confirmation
func (p pending) active(now time.Time) bool {
	return now.Before(p.until)
}

// confirm ends the pending change: Spotify caught up, or it no longer applies
func (p *pending) confirm() {
	p.until = time.Time{}
}
