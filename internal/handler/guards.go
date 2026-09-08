package handler

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Resource guards for an unattended service on a LAN.
//
// None of these protect against a determined attacker with valid admin
// credentials; they exist so that a buggy script, a stuck browser tab, or an
// unauthenticated brute-force attempt cannot exhaust a Raspberry Pi.
const (
	// loginBurst attempts may be made back to back per client address before
	// throttling begins.
	loginBurst = 5
	// loginRefill is how long it takes to earn one more attempt.
	loginRefill = 20 * time.Second

	// maxConcurrentProbes bounds probing, which opens a network stream and
	// runs a decoder. Two at a time keeps the UI responsive without letting a
	// loop of probe calls saturate the host.
	maxConcurrentProbes = 2

	// Long-lived streaming endpoints each hold a connection and a goroutine
	// for as long as the client stays. These are generous for a single-screen
	// appliance whose UI opens one of each.
	maxSSEClients     = 16
	maxPreviewClients = 4

	// maxJSONBodyBytes bounds non-multipart request bodies. The largest
	// legitimate payload is a screen with its tiles and sequences.
	maxJSONBodyBytes = 1 << 20
)

// rateLimiter is a per-key token bucket.
type rateLimiter struct {
	burst  int
	refill time.Duration

	mu         sync.Mutex
	buckets    map[string]*bucket
	lastSweep  time.Time
	sweepEvery time.Duration
	idleTTL    time.Duration
}

type bucket struct {
	tokens float64
	seen   time.Time
}

func newRateLimiter(burst int, refill time.Duration) *rateLimiter {
	return &rateLimiter{
		burst:      burst,
		refill:     refill,
		buckets:    map[string]*bucket{},
		sweepEvery: time.Minute,
		idleTTL:    10 * time.Minute,
	}
}

// allow consumes a token for key, reporting whether the request may proceed.
func (l *rateLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sweepLocked(now)

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: float64(l.burst), seen: now}
		l.buckets[key] = b
	} else {
		b.tokens += now.Sub(b.seen).Seconds() / l.refill.Seconds()
		if b.tokens > float64(l.burst) {
			b.tokens = float64(l.burst)
		}
		b.seen = now
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// sweepLocked discards idle buckets. Without it the map would keep an entry
// for every address that ever attempted a login, for the life of the process.
func (l *rateLimiter) sweepLocked(now time.Time) {
	if now.Sub(l.lastSweep) < l.sweepEvery {
		return
	}
	l.lastSweep = now
	for key, b := range l.buckets {
		if now.Sub(b.seen) > l.idleTTL {
			delete(l.buckets, key)
		}
	}
}

// clientKey identifies a caller for rate-limiting. RealIP has already applied
// any proxy headers by the time handlers run.
func clientKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// tryAcquireProbe reserves one of the probe slots, returning a release func.
func (h *Handlers) tryAcquireProbe() (release func(), ok bool) {
	if h.probeSlots == nil {
		return func() {}, true
	}
	select {
	case h.probeSlots <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-h.probeSlots }) }, true
	default:
		return nil, false
	}
}

// tryAcquireStream reserves a slot on a streaming endpoint.
func tryAcquireStream(counter *atomic.Int32, limit int) (release func(), ok bool) {
	if counter.Add(1) > int32(limit) {
		counter.Add(-1)
		return nil, false
	}
	var once sync.Once
	return func() { once.Do(func() { counter.Add(-1) }) }, true
}

// LimitRequestBody caps non-multipart request bodies.
//
// Multipart is exempt because the upload handler applies its own, much larger
// limit; wrapping it here first would make the smaller limit win and break
// uploads.
func LimitRequestBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil && !isMultipart(r) {
			r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
		}
		next.ServeHTTP(w, r)
	})
}

func isMultipart(r *http.Request) bool {
	return strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/")
}
