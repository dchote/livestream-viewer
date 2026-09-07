package events

import (
	"strings"
	"testing"
	"time"
)

func TestHubPublish(t *testing.T) {
	t.Parallel()
	h := NewHub()
	ch, unsub := h.Subscribe()
	defer unsub()
	h.PublishDisplayState(map[string]any{"display_running": false})
	select {
	case chunk := <-ch:
		if !strings.Contains(string(chunk), "event: display.state") {
			t.Fatalf("chunk %s", chunk)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestHubCloseUnblocksSubscribers(t *testing.T) {
	t.Parallel()
	h := NewHub()
	ch, unsub := h.Subscribe()
	defer unsub()
	h.Close()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected closed channel")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for close")
	}
	// Idempotent, and a late publish must not panic.
	h.Close()
	h.PublishDisplayState(map[string]any{"paused": true})
}
