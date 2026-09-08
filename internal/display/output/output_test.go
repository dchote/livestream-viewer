package output

import (
	"testing"
)

func TestVersionGateLogic(t *testing.T) {
	// Default tests must not initialise SDL. Version parsing is covered by
	// go-sdl3; this package's Init is exercised with -tags sdl.
	t.Log("sdl init is behind the sdl build tag")
}
