package display

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Zyko0/go-sdl3/sdl"
	"github.com/dchote/livestream-viewer/internal/display/compositor"
	"github.com/dchote/livestream-viewer/internal/display/layout"
	"github.com/dchote/livestream-viewer/internal/display/output"
	"github.com/dchote/livestream-viewer/internal/display/texture"
	"github.com/dchote/livestream-viewer/internal/display/transition"
	"github.com/dchote/livestream-viewer/internal/frame"
	"github.com/dchote/livestream-viewer/internal/ingest"
	"github.com/dchote/livestream-viewer/internal/preview"
	"github.com/dchote/livestream-viewer/internal/schedule"
)

const maxCommandsPerFrame = 32

type commandKind int

const (
	cmdBind commandKind = iota
	cmdUnbind
)

type command struct {
	kind commandKind
	id   uint
	slot *frame.Slot
	done chan struct{}
}

// Config is process-level display setup.
type Config struct {
	Driver string
	Device int
	Width  int
	Height int
	Cancel func()
}

// Engine is the vsync render loop. SDL is only used from Run.
type Engine struct {
	cfg     Config
	rt      *schedule.Runtime
	mgr     *ingest.Manager
	preview *preview.Service

	cmds         chan command
	stopCh       chan struct{}
	stopOnce     sync.Once
	stopped      atomic.Bool
	mu           sync.Mutex
	slots        map[uint]*frame.Slot
	seen         map[uint]time.Time
	sizes        map[uint]compositor.VideoSize
	lastSeq      map[uint]uint64
	clocks       map[uint]*presentClock
	vsync        time.Duration
	refreshKnown bool
	phase        int

	displays []output.DisplayInfo
	cache    *texture.Cache
}

// NewEngine constructs an engine. Run must be called on the locked main thread.
func NewEngine(cfg Config, rt *schedule.Runtime, mgr *ingest.Manager, prev *preview.Service) *Engine {
	return &Engine{
		cfg:     cfg,
		rt:      rt,
		mgr:     mgr,
		preview: prev,
		cmds:    make(chan command, 64),
		stopCh:  make(chan struct{}),
		slots:   map[uint]*frame.Slot{},
		seen:    map[uint]time.Time{},
		sizes:   map[uint]compositor.VideoSize{},
		lastSeq: map[uint]uint64{},
		clocks:  map[uint]*presentClock{},
		vsync:   defaultVsync,
	}
}

// SetManager attaches the ingest manager after construction.
func (e *Engine) SetManager(m *ingest.Manager) {
	e.mgr = m
}

// Bind queues a slot for the render thread. It does not wait for the bind to
// be applied; a false return means the command was not queued and the worker
// must not start.
func (e *Engine) Bind(id uint, slot *frame.Slot) bool {
	return e.send(command{kind: cmdBind, id: id, slot: slot}, false)
}

// Unbind drops a slot before the worker is cancelled. It waits until the
// render thread has released the texture.
func (e *Engine) Unbind(id uint) {
	e.send(command{kind: cmdUnbind, id: id, done: make(chan struct{})}, true)
}

func (e *Engine) send(c command, waitApply bool) bool {
	if e.stopped.Load() {
		if c.done != nil {
			close(c.done)
		}
		return false
	}
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case e.cmds <- c:
	case <-e.stopCh:
		if c.done != nil {
			close(c.done)
		}
		return false
	case <-timer.C:
		if c.done != nil {
			close(c.done)
		}
		return false
	}
	if !waitApply {
		return true
	}
	select {
	case <-c.done:
		return true
	case <-e.stopCh:
		return false
	case <-timer.C:
		return false
	}
}

func (e *Engine) stop() {
	e.stopOnce.Do(func() {
		e.stopped.Store(true)
		close(e.stopCh)
	})
	e.drainAll()
}

// Displays is the last SDL enumeration.
func (e *Engine) Displays() []output.DisplayInfo {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]output.DisplayInfo(nil), e.displays...)
}

// Run initialises SDL and loops until ctx is cancelled. On init failure it
// records the error, leaves the API up, and returns after ctx is done.
//
// Run owns the main OS thread, so a panic here would kill the process before
// anything could be flushed. It is recovered into an ordinary shutdown: the
// error is published to the control plane and the process exits deliberately.
func (e *Engine) Run(ctx context.Context) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			msg := fmt.Sprintf("render loop panicked: %v", rec)
			slog.Error("render loop panicked", "panic", rec, "stack", string(debug.Stack()))
			if e.rt != nil {
				e.rt.SetDisplayMetrics(false, 0, 0, msg, []string{"render_loop_panic"})
			}
			e.stop()
			if e.preview != nil {
				e.preview.Close()
			}
			if e.cfg.Cancel != nil {
				e.cfg.Cancel()
			}
			err = errors.New(msg)
		}
	}()
	out, err := output.Init(output.Config{
		Driver: e.cfg.Driver,
		Device: e.cfg.Device,
		Width:  e.cfg.Width,
		Height: e.cfg.Height,
		Cancel: e.cfg.Cancel,
	})
	if err != nil {
		if e.rt != nil {
			e.rt.SetDisplayMetrics(false, 0, 0, err.Error(), []string{"sdl_init_failed"})
		}
		return err
	}
	defer out.Close()

	e.mu.Lock()
	e.displays = output.Displays()
	e.mu.Unlock()

	if e.rt != nil {
		e.rt.SetDisplayMetrics(true, 0, 0, "", nil)
	}

	cache := texture.NewCache(out.Renderer)
	e.cache = cache
	defer cache.Close()

	e.setRefresh(out.Refresh())
	if !out.VSync {
		slog.Warn("renderer refused vsync; presentation may tear")
	}

	var (
		layerTex [2]*sdl.Texture
		back     *sdl.Texture
		prevTex  *sdl.Texture
		prevW    int
		prevH    int
		dropped  uint64
		frames   uint64
		lastFPS  = time.Now()
		fps      float64
		lastPrev time.Time
		lastLoop time.Time
		degrade  []string
	)
	// Degradations describe the frame being drawn, so they are rebuilt each
	// pass. Accumulating them meant one masked wipe left the display reporting
	// itself degraded for the rest of the process's life.
	degradedThisFrame := func(active bool, reason string) {
		degrade = degrade[:0]
		if !out.VSync {
			// Without vsync the flip is not tied to vblank and the wall
			// tears. Say so rather than leaving it as a mystery.
			degrade = append(degrade, "vsync_unavailable")
		}
		if active {
			degrade = append(degrade, reason)
		}
	}

	ensureTarget := func(tex **sdl.Texture, w, h int, access sdl.TextureAccess) {
		if *tex != nil && int((*tex).W) == w && int((*tex).H) == h {
			return
		}
		t, err := out.Renderer.CreateTexture(sdl.PIXELFORMAT_RGBA32, access, w, h)
		if err == nil {
			if *tex != nil {
				(*tex).Destroy()
			}
			*tex = t
		}
	}

	tw, th := out.Width, out.Height
	ensureTarget(&layerTex[0], tw, th, sdl.TEXTUREACCESS_TARGET)
	ensureTarget(&layerTex[1], tw, th, sdl.TEXTUREACCESS_TARGET)
	ensureTarget(&back, tw, th, sdl.TEXTUREACCESS_TARGET)
	defer func() {
		if out.Gone() {
			return
		}
		for _, t := range layerTex {
			if t != nil {
				t.Destroy()
			}
		}
		if back != nil {
			back.Destroy()
		}
		if prevTex != nil {
			prevTex.Destroy()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			e.finish(out, cache, fps, dropped, degrade)
			return nil
		default:
		}

		if !out.Pump() {
			e.finish(out, cache, fps, dropped, degrade)
			return nil
		}
		e.drain()

		if e.rt != nil {
			e.rt.Tick()
		}

		view := schedule.ComposeView{}
		if e.rt != nil {
			view = e.rt.ComposeView()
		}
		if e.mgr != nil {
			e.mgr.RequestSync(view.Needed)
		}

		placeholder := "#1a1a1a"
		gutter := 4
		previewFPS := 2
		previewWidth := 640
		if e.rt != nil {
			if s := e.rt.Snapshot(); s != nil {
				if s.PlaceholderColor != "" {
					placeholder = s.PlaceholderColor
				}
				if s.GutterPx >= 0 {
					gutter = s.GutterPx
				}
				if s.PreviewFPS > 0 {
					previewFPS = s.PreviewFPS
				}
				if s.PreviewWidth > 0 {
					previewWidth = s.PreviewWidth
				}
			}
		}

		if out.Width != tw || out.Height != th {
			tw, th = out.Width, out.Height
			ensureTarget(&layerTex[0], tw, th, sdl.TEXTUREACCESS_TARGET)
			ensureTarget(&layerTex[1], tw, th, sdl.TEXTUREACCESS_TARGET)
			ensureTarget(&back, tw, th, sdl.TEXTUREACCESS_TARGET)
		}

		now := time.Now()
		if !lastLoop.IsZero() {
			period := now.Sub(lastLoop)
			if e.missedRefresh(period) {
				dropped++
			}
			e.observeVsync(period)
		}
		lastLoop = now
		e.sample(now, cache)

		outScene := compositor.Build(toLayer(view.Outgoing), tw, th, gutter, e.sizes, placeholder)
		var inScene compositor.Scene
		if view.Incoming != nil {
			inScene = compositor.Build(toLayer(*view.Incoming), tw, th, gutter, e.sizes, placeholder)
		}

		r := out.Renderer
		wantPreview := e.preview != nil && e.preview.Wanted() &&
			(previewFPS <= 0 || now.Sub(lastPrev) >= time.Second/time.Duration(previewFPS))

		// Steady state is one screen and no blend, so draw straight to the
		// backbuffer. Routing through two offscreen targets costs three
		// full-screen writes per frame instead of one, and that headroom is
		// what decides whether a wall of 1080p tiles holds the refresh or
		// misses it — which hitches every tile at once. The offscreen path
		// is still needed to blend a transition, and to give the preview
		// something it can downscale from.
		if view.Incoming == nil && !wantPreview {
			degradedThisFrame(false, "")
			_ = r.SetRenderTarget(nil)
			paintScene(r, cache, outScene, placeholder)
		} else {
			drawScene(r, cache, layerTex[0], outScene, placeholder)
			if view.Incoming != nil {
				drawScene(r, cache, layerTex[1], inScene, placeholder)
			}

			spec := view.Transition
			if spec.Type == "" || view.Incoming == nil {
				spec = transition.Spec{Type: "cut"}
			}
			draw := transition.Eval(spec, view.Progress, tw, th)
			degradedThisFrame(draw.Degraded, "masked_wipe_fade")

			_ = r.SetRenderTarget(back)
			pr, pg, pb, _ := parseColor(placeholder)
			_ = r.SetDrawColor(pr, pg, pb, 255)
			_ = r.Clear()
			composite(r, layerTex[0], layerTex[1], draw, tw, th)
			_ = r.SetRenderTarget(nil)
			_ = r.SetDrawColor(0, 0, 0, 255)
			_ = r.Clear()
			_ = r.RenderTexture(back, nil, nil)
		}

		if wantPreview {
			lastPrev = now
			pw := previewWidth
			if pw <= 0 {
				pw = 640
			}
			ph := pw * th / tw
			if ph < 1 {
				ph = 1
			}
			if prevTex == nil || prevW != pw || prevH != ph {
				if prevTex != nil {
					prevTex.Destroy()
				}
				prevTex, _ = r.CreateTexture(sdl.PIXELFORMAT_RGBA32, sdl.TEXTUREACCESS_TARGET, pw, ph)
				prevW, prevH = pw, ph
			}
			if prevTex != nil {
				_ = r.SetRenderTarget(prevTex)
				_ = r.RenderTexture(back, nil, nil)
				surf, err := r.ReadPixels(nil)
				_ = r.SetRenderTarget(nil)
				if err == nil && surf != nil {
					pix := surf.Pixels()
					e.preview.PushRGBA(pix, int(surf.W), int(surf.H), int(surf.Pitch))
					surf.Destroy()
				}
			}
		}

		if err := r.Present(); err != nil {
			dropped++
			e.finish(out, cache, fps, dropped, degrade)
			if e.cfg.Cancel != nil {
				e.cfg.Cancel()
			}
			return nil
		}
		frames++
		if now.Sub(lastFPS) >= time.Second {
			fps = float64(frames) / now.Sub(lastFPS).Seconds()
			frames = 0
			lastFPS = now
			// The window can be dragged to a panel with a different refresh
			// rate, which changes what every presentation clock is aiming at.
			e.setRefresh(out.Refresh())
			if e.rt != nil {
				e.rt.SetDisplayMetrics(true, fps, dropped, "", degrade)
			}
		}
	}
}

func (e *Engine) finish(out *output.Output, cache *texture.Cache, fps float64, dropped uint64, degrade []string) {
	e.stop()
	if out.Gone() && cache != nil {
		cache.Abandon()
	}
	if e.rt != nil {
		e.rt.SetDisplayMetrics(false, fps, dropped, "", degrade)
	}
	if e.preview != nil {
		e.preview.Close()
	}
}

func (e *Engine) drain() {
	e.drainLimited(maxCommandsPerFrame)
}

func (e *Engine) drainAll() {
	e.drainLimited(-1)
}

func (e *Engine) drainLimited(max int) {
	for i := 0; max < 0 || i < max; i++ {
		select {
		case c := <-e.cmds:
			e.apply(c)
		default:
			return
		}
	}
}

func (e *Engine) apply(c command) {
	switch c.kind {
	case cmdBind:
		e.slots[c.id] = c.slot
	case cmdUnbind:
		delete(e.slots, c.id)
		delete(e.seen, c.id)
		delete(e.sizes, c.id)
		delete(e.lastSeq, c.id)
		delete(e.clocks, c.id)
		if e.cache != nil {
			e.cache.Drop(c.id)
		}
	}
	if c.done != nil {
		close(c.done)
	}
}

// setRefresh adopts the interval SDL reports for the display showing the
// window. It is what tells the presentation clocks how far ahead the frame
// being drawn will actually appear.
func (e *Engine) setRefresh(d time.Duration) {
	if d < time.Millisecond || d > 100*time.Millisecond {
		return
	}
	e.vsync = d
	e.refreshKnown = true
}

// observeVsync estimates the refresh interval from the loop period, for
// drivers that cannot report a mode. It is a fallback, not a source of truth:
// the loop period includes our own overruns, so an engine timing itself would
// mistake its own slowness for a slower panel and drift further behind.
func (e *Engine) observeVsync(d time.Duration) {
	if e.refreshKnown || d < time.Millisecond || d > 100*time.Millisecond {
		return
	}
	e.vsync += (d - e.vsync) / 8
}

// stagger is the anchor offset for a clock about to be set, cycling through
// whole refresh intervals so that consecutive sources land on different
// refreshes. Clocks that are already anchored keep the offset they were given.
func (e *Engine) stagger(c *presentClock) time.Duration {
	if c.set {
		return 0
	}
	d := time.Duration(e.phase%staggerSpread) * e.vsync
	e.phase++
	return d
}

// missedRefresh reports that the loop failed to hold the display's cadence.
// This matters more than any single stream's timing: a missed vblank hitches
// every tile on the wall at once.
func (e *Engine) missedRefresh(period time.Duration) bool {
	return e.vsync > 0 && period > e.vsync*3/2
}

func (e *Engine) sample(now time.Time, cache *texture.Cache) {
	// Pixels drawn this pass reach the panel at the next refresh, so frames
	// are chosen for that moment rather than for this one.
	present := now.Add(e.vsync)
	for id, slot := range e.slots {
		c, ok := e.clocks[id]
		if !ok {
			c = &presentClock{}
			e.clocks[id] = c
		}
		f := c.due(slot, present, e.stagger(c))
		if f == nil {
			// Nothing new is due. Keep the last uploaded texture: this is
			// the ordinary repeat for a source slower than the refresh rate,
			// and it also covers reconnects and GOP waits without flashing
			// a black tile.
			continue
		}
		// The untimed path re-returns the frame already on screen, and
		// uploading it again would re-send megabytes to the GPU to arrive
		// at the pixels it is already holding.
		if e.lastSeq[id] == f.Seq {
			continue
		}
		if _, ok := e.seen[id]; !ok {
			slog.Info("bound frame", "source_id", id, "w", f.Width, "h", f.Height)
		}
		e.lastSeq[id] = f.Seq
		e.seen[id] = now
		e.sizes[id] = compositor.VideoSize{W: f.Width, H: f.Height}
		cache.Upload(id, f)
	}
}

func toLayer(l schedule.Layer) compositor.Layer {
	tiles := make([]compositor.LayerTile, 0, len(l.Tiles))
	for _, t := range l.Tiles {
		tiles = append(tiles, compositor.LayerTile{Index: t.Index, SourceID: t.SourceID, Fit: t.Fit})
	}
	return compositor.Layer{Kind: l.Kind, Layout: l.Layout, Rects: l.Rects, Tiles: tiles}
}

func drawScene(r *sdl.Renderer, cache *texture.Cache, target *sdl.Texture, scene compositor.Scene, placeholder string) {
	if target == nil {
		return
	}
	_ = r.SetRenderTarget(target)
	paintScene(r, cache, scene, placeholder)
}

// paintScene draws the scene into whatever render target is currently bound.
func paintScene(r *sdl.Renderer, cache *texture.Cache, scene compositor.Scene, placeholder string) {
	pr, pg, pb, _ := parseColor(placeholder)
	_ = r.SetDrawColor(pr, pg, pb, 255)
	_ = r.Clear()
	for _, tile := range scene.Tiles {
		if tile.Placeholder || tile.SourceID == nil {
			cr, cg, cb, _ := parseColor(placeholder)
			_ = r.SetDrawColor(cr, cg, cb, 255)
			dst := frect(tile.Cell)
			_ = r.RenderFillRect(&dst)
			continue
		}
		tex := cache.Get(*tile.SourceID)
		if tex == nil {
			cr, cg, cb, _ := parseColor(placeholder)
			_ = r.SetDrawColor(cr, cg, cb, 255)
			dst := frect(tile.Cell)
			_ = r.RenderFillRect(&dst)
			continue
		}
		src := frect(tile.Src)
		dst := frect(tile.Dst)
		_ = tex.SetBlendMode(sdl.BLENDMODE_NONE)
		_ = r.RenderTexture(tex, &src, &dst)
	}
}

func composite(r *sdl.Renderer, outgoing, incoming *sdl.Texture, d transition.Draw, w, h int) {
	if d.Fill != nil {
		cr, cg, cb, _ := parseColor(*d.Fill)
		_ = r.SetDrawColor(cr, cg, cb, 255)
		full := sdl.FRect{X: 0, Y: 0, W: float32(w), H: float32(h)}
		_ = r.RenderFillRect(&full)
	}
	blit := func(tex *sdl.Texture, layer transition.Layer) {
		if tex == nil || layer.Alpha <= 0 || layer.Dst.W <= 0 || layer.Dst.H <= 0 {
			return
		}
		_ = tex.SetBlendMode(sdl.BLENDMODE_BLEND)
		_ = tex.SetAlphaModFloat(layer.Alpha)
		if layer.Clip != nil {
			c := layer.Clip
			_ = r.SetClipRect(&sdl.Rect{X: int32(c.X), Y: int32(c.Y), W: int32(c.W), H: int32(c.H)})
		} else {
			_ = r.SetClipRect(nil)
		}
		dst := sdl.FRect{X: float32(layer.Dst.X), Y: float32(layer.Dst.Y), W: float32(layer.Dst.W), H: float32(layer.Dst.H)}
		_ = r.RenderTexture(tex, nil, &dst)
		_ = r.SetClipRect(nil)
		_ = tex.SetAlphaModFloat(1)
	}
	blit(outgoing, d.Outgoing)
	blit(incoming, d.Incoming)
}

func frect(p layout.PixelRect) sdl.FRect {
	return sdl.FRect{X: float32(p.X), Y: float32(p.Y), W: float32(p.W), H: float32(p.H)}
}

func parseColor(s string) (uint8, uint8, uint8, uint8) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) < 6 {
		return 26, 26, 26, 255
	}
	b, err := hex.DecodeString(s[:6])
	if err != nil || len(b) < 3 {
		return 26, 26, 26, 255
	}
	return b[0], b[1], b[2], 255
}
