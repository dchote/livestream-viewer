package output

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/Zyko0/go-sdl3/sdl"
)

// DisplayInfo is one SDL-enumerated monitor.
type DisplayInfo struct {
	ID     uint32 `json:"id"`
	Name   string `json:"name"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// Output is an SDL3 window + renderer owned by the render thread.
type Output struct {
	Window   *sdl.Window
	Renderer *sdl.Renderer
	Width    int
	Height   int
	Driver   string
	// VSync reports that the renderer accepted vsync. Some drivers refuse it,
	// and without it presentation tears; the loop surfaces that rather than
	// letting it pass as an unexplained visual fault.
	VSync  bool
	cancel func()
	gone   bool
	closed bool
}

// Config selects the video driver and initial window size.
type Config struct {
	Driver string
	Device int
	Width  int
	Height int
	Title  string
	Cancel func() // invoked on window close
}

func loadLibrary() error {
	candidates := []string{
		os.Getenv("SDL3_LIBRARY"),
		sdl.Path(),
		"/opt/homebrew/lib/libSDL3.dylib",
		"/usr/local/lib/libSDL3.dylib",
		"/opt/homebrew/opt/sdl3/lib/libSDL3.dylib",
		"/usr/lib/libSDL3.so.0",
		"/usr/local/lib/libSDL3.so.0",
		"/opt/sdl3/lib/libSDL3.so.0",
		"/usr/lib/livestream-viewer/libSDL3.so.0",
	}
	var last error
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if err := sdl.LoadLibrary(p); err != nil {
			last = err
			continue
		}
		return nil
	}
	if last != nil {
		return last
	}
	return fmt.Errorf("SDL3 library not found")
}

// Init loads system SDL3 (never binsdl), verifies ≥ 3.4.0, and opens a windowed surface.
func Init(cfg Config) (*Output, error) {
	if err := loadLibrary(); err != nil {
		return nil, err
	}
	ver := sdl.GetVersion()
	if ver.Major() < 3 || (ver.Major() == 3 && ver.Minor() < 4) {
		return nil, fmt.Errorf("SDL %s is older than 3.4.0", ver.String())
	}

	driver := cfg.Driver
	if driver == "" {
		switch runtime.GOOS {
		case "darwin":
			driver = "cocoa"
		case "linux":
			if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
				driver = "kmsdrm"
			}
		}
	}
	if driver != "" {
		_ = sdl.SetHint(sdl.HINT_VIDEO_DRIVER, driver)
	}
	if driver == "kmsdrm" {
		_ = sdl.SetHint(sdl.HINT_RENDER_DRIVER, "opengles2")
		_ = sdl.SetHint("SDL_KMSDRM_DEVICE_INDEX", strconv.Itoa(cfg.Device))
		_ = sdl.SetHint("SDL_VIDEODRIVER", "kmsdrm") // ignored by SDL3; logged for operators grepping env
	}

	if err := sdl.Init(sdl.INIT_VIDEO); err != nil {
		return nil, fmt.Errorf("sdl init: %w", err)
	}

	w, h := cfg.Width, cfg.Height
	if w <= 0 {
		w = 1920
	}
	if h <= 0 {
		h = 1080
	}
	title := cfg.Title
	if title == "" {
		title = "livestream-viewer"
	}

	flags := sdl.WINDOW_RESIZABLE
	if driver == "kmsdrm" {
		flags |= sdl.WINDOW_FULLSCREEN
	}
	window, renderer, err := sdl.CreateWindowAndRenderer(title, w, h, flags)
	if err != nil {
		sdl.Quit()
		return nil, fmt.Errorf("window: %w", err)
	}
	vsync := renderer.SetVSync(1) == nil

	pw, ph, err := window.SizeInPixels()
	if err == nil && pw > 0 && ph > 0 {
		w, h = int(pw), int(ph)
	}

	return &Output{
		Window:   window,
		Renderer: renderer,
		Width:    w,
		Height:   h,
		Driver:   sdl.GetCurrentVideoDriver(),
		VSync:    vsync,
		cancel:   cfg.Cancel,
	}, nil
}

// Refresh is the interval between vblanks on the display currently showing the
// window, or 0 when SDL cannot report a mode.
//
// This is the authoritative figure. Timing the render loop instead measures our
// own overruns as well as the display, so an engine that falls behind would
// conclude the panel had slowed down and schedule against a refresh that does
// not exist.
func (o *Output) Refresh() time.Duration {
	if o == nil || o.gone || o.Window == nil {
		return 0
	}
	mode, err := sdl.GetDisplayForWindow(o.Window).CurrentDisplayMode()
	if err != nil || mode == nil {
		return 0
	}
	// The exact rational form is present for modes like 59.94Hz, where the
	// rounded float would accumulate a frame of error every few minutes.
	if mode.RefreshRateNumerator > 0 && mode.RefreshRateDenominator > 0 {
		return time.Duration(float64(time.Second) * float64(mode.RefreshRateDenominator) / float64(mode.RefreshRateNumerator))
	}
	if mode.RefreshRate > 0 {
		return time.Duration(float64(time.Second) / float64(mode.RefreshRate))
	}
	return 0
}

// Gone reports that the native window is already destroyed (do not Present or Destroy).
func (o *Output) Gone() bool {
	return o == nil || o.gone
}

// Pump drains the SDL event queue. Returns false when the user closed the window.
func (o *Output) Pump() bool {
	if o == nil || o.gone {
		return false
	}
	if o.Window == nil {
		return true
	}
	var ev sdl.Event
	for sdl.PollEvent(&ev) {
		switch ev.Type {
		case sdl.EVENT_QUIT, sdl.EVENT_WINDOW_CLOSE_REQUESTED:
			o.requestQuit()
			return false
		case sdl.EVENT_WINDOW_DESTROYED:
			// Cocoa may destroy the surface before we call Destroy.
			o.gone = true
			o.Renderer = nil
			o.Window = nil
			o.requestQuit()
			return false
		case sdl.EVENT_WINDOW_PIXEL_SIZE_CHANGED, sdl.EVENT_WINDOW_RESIZED:
			if pw, ph, err := o.Window.SizeInPixels(); err == nil && pw > 0 && ph > 0 {
				o.Width, o.Height = int(pw), int(ph)
			}
		}
	}
	return !o.gone
}

func (o *Output) requestQuit() {
	if o.cancel != nil {
		o.cancel()
		o.cancel = nil
	}
}

// Displays lists connected monitors. Valid after Init.
func Displays() []DisplayInfo {
	ids, err := sdl.GetDisplays()
	if err != nil {
		return []DisplayInfo{}
	}
	out := make([]DisplayInfo, 0, len(ids))
	for _, id := range ids {
		info := DisplayInfo{ID: uint32(id)}
		if n, err := id.Name(); err == nil {
			info.Name = n
		}
		if b, err := id.Bounds(); err == nil && b != nil {
			info.Width, info.Height = int(b.W), int(b.H)
		}
		out = append(out, info)
	}
	return out
}

// Close destroys the window and quits SDL.
func (o *Output) Close() {
	if o == nil || o.closed {
		return
	}
	o.closed = true
	if !o.gone {
		if o.Renderer != nil {
			o.Renderer.Destroy()
		}
		if o.Window != nil {
			o.Window.Destroy()
		}
	}
	o.Renderer = nil
	o.Window = nil
	sdl.Quit()
}
