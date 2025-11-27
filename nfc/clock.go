package nfc

import (
	"sync"
	"time"
)

// Clock provides an abstraction over time operations to enable testing
// without real time delays.
type Clock interface {
	// Now returns the current time
	Now() time.Time

	// Sleep pauses execution for the given duration
	Sleep(d time.Duration)

	// NewTicker creates a new ticker that will send on its channel
	// at intervals specified by the duration
	NewTicker(d time.Duration) Ticker

	// NewTimer creates a new timer that will send on its channel
	// after the specified duration
	NewTimer(d time.Duration) Timer

	// After returns a channel that will receive a value after the duration
	After(d time.Duration) <-chan time.Time
}

// Ticker is an interface for time.Ticker to enable testing
type Ticker interface {
	// C returns the channel on which ticks are delivered
	C() <-chan time.Time

	// Stop turns off the ticker
	Stop()

	// Reset stops a ticker and resets its period to the specified duration
	Reset(d time.Duration)
}

// Timer is an interface for time.Timer to enable testing
type Timer interface {
	// C returns the channel on which the timer value will be sent
	C() <-chan time.Time

	// Stop prevents the timer from firing
	Stop() bool

	// Reset changes the timer to expire after duration d
	Reset(d time.Duration) bool
}

// RealClock implements Clock using actual time operations
type RealClock struct{}

// NewRealClock creates a new RealClock
func NewRealClock() Clock {
	return &RealClock{}
}

func (rc *RealClock) Now() time.Time {
	return time.Now()
}

func (rc *RealClock) Sleep(d time.Duration) {
	time.Sleep(d)
}

func (rc *RealClock) NewTicker(d time.Duration) Ticker {
	return &realTicker{ticker: time.NewTicker(d)}
}

func (rc *RealClock) NewTimer(d time.Duration) Timer {
	return &realTimer{timer: time.NewTimer(d)}
}

func (rc *RealClock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}

// realTicker wraps time.Ticker to implement Ticker interface
type realTicker struct {
	ticker *time.Ticker
}

func (rt *realTicker) C() <-chan time.Time {
	return rt.ticker.C
}

func (rt *realTicker) Stop() {
	rt.ticker.Stop()
}

func (rt *realTicker) Reset(d time.Duration) {
	rt.ticker.Reset(d)
}

// realTimer wraps time.Timer to implement Timer interface
type realTimer struct {
	timer *time.Timer
}

func (rt *realTimer) C() <-chan time.Time {
	return rt.timer.C
}

func (rt *realTimer) Stop() bool {
	return rt.timer.Stop()
}

func (rt *realTimer) Reset(d time.Duration) bool {
	return rt.timer.Reset(d)
}

// FakeClock implements Clock for testing with controllable time
type FakeClock struct {
	mu      sync.RWMutex
	now     time.Time
	tickers []*fakeTicker
	timers  []*fakeTimer
}

// NewFakeClock creates a new FakeClock starting at the given time
func NewFakeClock(startTime time.Time) *FakeClock {
	return &FakeClock{
		now:     startTime,
		tickers: make([]*fakeTicker, 0),
		timers:  make([]*fakeTimer, 0),
	}
}

func (fc *FakeClock) Now() time.Time {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	return fc.now
}

func (fc *FakeClock) Sleep(d time.Duration) {
	// In fake clock, sleep advances time immediately
	fc.Advance(d)
}

func (fc *FakeClock) NewTicker(d time.Duration) Ticker {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	ft := &fakeTicker{
		interval: d,
		c:        make(chan time.Time, 1),
		stopped:  false,
	}
	fc.tickers = append(fc.tickers, ft)
	return ft
}

func (fc *FakeClock) NewTimer(d time.Duration) Timer {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	ft := &fakeTimer{
		deadline: fc.now.Add(d),
		c:        make(chan time.Time, 1),
		stopped:  false,
	}
	fc.timers = append(fc.timers, ft)
	return ft
}

func (fc *FakeClock) After(d time.Duration) <-chan time.Time {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	ch := make(chan time.Time, 1)
	ft := &fakeTimer{
		deadline: fc.now.Add(d),
		c:        ch,
		stopped:  false,
	}
	fc.timers = append(fc.timers, ft)
	return ch
}

// Advance moves the fake clock forward by the given duration
// and fires any tickers/timers that should fire
func (fc *FakeClock) Advance(d time.Duration) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.now = fc.now.Add(d)

	// Fire tickers
	for _, ticker := range fc.tickers {
		if !ticker.stopped {
			select {
			case ticker.c <- fc.now:
			default:
				// Channel full, skip
			}
		}
	}

	// Fire timers that have reached their deadline
	for _, timer := range fc.timers {
		if !timer.stopped && !fc.now.Before(timer.deadline) {
			select {
			case timer.c <- fc.now:
				timer.stopped = true // Timers only fire once
			default:
				// Channel full, skip
			}
		}
	}
}

// fakeTicker implements Ticker for testing
type fakeTicker struct {
	interval time.Duration
	c        chan time.Time
	stopped  bool
}

func (ft *fakeTicker) C() <-chan time.Time {
	return ft.c
}

func (ft *fakeTicker) Stop() {
	ft.stopped = true
}

func (ft *fakeTicker) Reset(d time.Duration) {
	ft.interval = d
	ft.stopped = false
}

// fakeTimer implements Timer for testing
type fakeTimer struct {
	deadline time.Time
	c        chan time.Time
	stopped  bool
}

func (ft *fakeTimer) C() <-chan time.Time {
	return ft.c
}

func (ft *fakeTimer) Stop() bool {
	if ft.stopped {
		return false
	}
	ft.stopped = true
	return true
}

func (ft *fakeTimer) Reset(d time.Duration) bool {
	active := !ft.stopped
	ft.stopped = false
	// Note: In a real implementation, we'd need to update the deadline
	// based on the FakeClock's current time, but that requires a reference
	// back to the clock. For now, this is a simplified implementation.
	return active
}
