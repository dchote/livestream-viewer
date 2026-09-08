package display

import (
	"testing"
	"time"
)

func TestUnbindAfterStopDoesNotBlock(t *testing.T) {
	e := NewEngine(Config{}, nil, nil, nil)
	e.stop()
	done := make(chan struct{})
	go func() {
		e.Unbind(1)
		if e.Bind(2, nil) {
			t.Error("Bind after stop must fail")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(400 * time.Millisecond):
		t.Fatal("bind/unbind blocked after engine stop")
	}
}
