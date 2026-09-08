package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRateLimiterAllowsBurstThenThrottles(t *testing.T) {
	t.Parallel()
	l := newRateLimiter(3, time.Second)
	now := time.Now()
	for i := 0; i < 3; i++ {
		if !l.allow("a", now) {
			t.Fatalf("attempt %d should be allowed within the burst", i)
		}
	}
	if l.allow("a", now) {
		t.Fatal("fourth attempt should be throttled")
	}
}

func TestRateLimiterRefillsOverTime(t *testing.T) {
	t.Parallel()
	l := newRateLimiter(1, time.Second)
	now := time.Now()
	if !l.allow("a", now) {
		t.Fatal("first attempt should be allowed")
	}
	if l.allow("a", now.Add(500*time.Millisecond)) {
		t.Fatal("half a refill period should not earn a token")
	}
	if !l.allow("a", now.Add(1100*time.Millisecond)) {
		t.Fatal("a full refill period should earn a token")
	}
}

func TestRateLimiterKeysAreIndependent(t *testing.T) {
	t.Parallel()
	l := newRateLimiter(1, time.Minute)
	now := time.Now()
	if !l.allow("a", now) || !l.allow("b", now) {
		t.Fatal("each key has its own bucket")
	}
	if l.allow("a", now) {
		t.Fatal("key a is exhausted")
	}
}

// The bucket map would otherwise keep an entry for every address that ever
// hit the login endpoint, for the life of the process.
func TestRateLimiterDiscardsIdleBuckets(t *testing.T) {
	t.Parallel()
	l := newRateLimiter(1, time.Second)
	now := time.Now()
	for i := 0; i < 100; i++ {
		l.allow(strings.Repeat("x", i%7)+string(rune('a'+i%26)), now)
	}
	before := len(l.buckets)
	if before == 0 {
		t.Fatal("expected buckets to be recorded")
	}
	// Past both the sweep interval and the idle TTL.
	l.allow("fresh", now.Add(l.idleTTL+2*time.Minute))
	if len(l.buckets) != 1 {
		t.Fatalf("expected only the fresh bucket to survive, have %d of %d", len(l.buckets), before)
	}
}

func TestRateLimiterIsConcurrencySafe(t *testing.T) {
	t.Parallel()
	l := newRateLimiter(50, time.Second)
	var allowed atomic.Int32
	var wg sync.WaitGroup
	now := time.Now()
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if l.allow("shared", now) {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := allowed.Load(); got != 50 {
		t.Fatalf("burst of 50 admitted %d of 100 concurrent attempts", got)
	}
}

func TestTryAcquireProbeBoundsConcurrency(t *testing.T) {
	t.Parallel()
	h := &Handlers{probeSlots: make(chan struct{}, maxConcurrentProbes)}
	releases := make([]func(), 0, maxConcurrentProbes)
	for i := 0; i < maxConcurrentProbes; i++ {
		release, ok := h.tryAcquireProbe()
		if !ok {
			t.Fatalf("slot %d should be available", i)
		}
		releases = append(releases, release)
	}
	if _, ok := h.tryAcquireProbe(); ok {
		t.Fatal("probe slots should be exhausted")
	}
	releases[0]()
	release, ok := h.tryAcquireProbe()
	if !ok {
		t.Fatal("releasing a slot should free it")
	}
	// Releasing twice must not free someone else's slot.
	release()
	release()
	for _, r := range releases[1:] {
		r()
	}
	if len(h.probeSlots) != 0 {
		t.Fatalf("%d slots still held", len(h.probeSlots))
	}
}

// A Handlers built without New (as some tests do) must not reject probes.
func TestTryAcquireProbeWithoutSlotsAllows(t *testing.T) {
	t.Parallel()
	h := &Handlers{}
	if _, ok := h.tryAcquireProbe(); !ok {
		t.Fatal("an unconfigured guard should not block probes")
	}
}

func TestTryAcquireStreamBoundsClients(t *testing.T) {
	t.Parallel()
	var counter atomic.Int32
	first, ok := tryAcquireStream(&counter, 1)
	if !ok {
		t.Fatal("first client should be admitted")
	}
	if _, ok := tryAcquireStream(&counter, 1); ok {
		t.Fatal("second client should be rejected at limit 1")
	}
	// A rejected attempt must not leave the counter incremented, or the
	// endpoint would refuse every later client.
	if counter.Load() != 1 {
		t.Fatalf("counter is %d after a rejection, want 1", counter.Load())
	}
	first()
	if counter.Load() != 0 {
		t.Fatalf("counter is %d after release, want 0", counter.Load())
	}
}

func TestLimitRequestBodyCapsJSON(t *testing.T) {
	t.Parallel()
	var readErr error
	h := LimitRequestBody(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 1024)
		for {
			if _, err := r.Body.Read(buf); err != nil {
				readErr = err
				return
			}
		}
	}))
	body := strings.NewReader(strings.Repeat("a", maxJSONBodyBytes+1024))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/config", body)
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(httptest.NewRecorder(), req)

	var maxErr *http.MaxBytesError
	if readErr == nil || !errors.As(readErr, &maxErr) {
		t.Fatalf("expected MaxBytesError, got %v", readErr)
	}
}

// Multipart must stay exempt: the upload handler applies its own, far larger
// limit, and wrapping here first would make the smaller one win.
func TestLimitRequestBodyExemptsMultipart(t *testing.T) {
	t.Parallel()
	read := 0
	h := LimitRequestBody(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		for {
			n, err := r.Body.Read(buf)
			read += n
			if err != nil {
				return
			}
		}
	}))
	size := maxJSONBodyBytes + 4096
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", strings.NewReader(strings.Repeat("a", size)))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if read != size {
		t.Fatalf("read %d of %d bytes; multipart should not be capped here", read, size)
	}
}

func TestClientKeyStripsPort(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.5:54321"
	if got := clientKey(req); got != "10.0.0.5" {
		t.Fatalf("clientKey = %q, want 10.0.0.5", got)
	}
	// Ports vary per connection, so an address without one must still yield a
	// stable key rather than an empty one.
	req.RemoteAddr = "10.0.0.5"
	if got := clientKey(req); got != "10.0.0.5" {
		t.Fatalf("clientKey = %q, want 10.0.0.5", got)
	}
}
