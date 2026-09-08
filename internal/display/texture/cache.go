package texture

import (
	"github.com/Zyko0/go-sdl3/sdl"
	"github.com/dchote/livestream-viewer/internal/frame"
)

// texKey is the texture identity for one source. A change in any field means
// the existing texture cannot be reused and must be recreated.
type texKey struct {
	w, h   int
	format frame.Format
	space  string
	rng    string
}

type entry struct {
	key texKey
	tex *sdl.Texture
}

// Cache holds one streaming texture per source. Adaptive streams change
// resolution mid-session, so the entry is replaced (and the old texture
// destroyed) rather than accumulating a texture per geometry ever seen.
type Cache struct {
	r     *sdl.Renderer
	items map[uint]*entry
}

// NewCache binds to a renderer.
func NewCache(r *sdl.Renderer) *Cache {
	return &Cache{r: r, items: map[uint]*entry{}}
}

func keyOf(f *frame.Frame) texKey {
	return texKey{w: f.Width, h: f.Height, format: f.Format, space: f.Color.Space, rng: f.Color.Range}
}

// Upload writes an NV12 or I420 frame. Must run on the render thread.
func (c *Cache) Upload(sourceID uint, f *frame.Frame) *sdl.Texture {
	if c == nil || c.r == nil || f == nil || !uploadable(f) {
		return nil
	}
	k := keyOf(f)
	e, ok := c.items[sourceID]
	if ok && e.key != k {
		if e.tex != nil {
			e.tex.Destroy()
		}
		delete(c.items, sourceID)
		ok = false
	}
	if !ok {
		tex, err := createYUV(c.r, f)
		if err != nil || tex == nil {
			return nil
		}
		e = &entry{key: k, tex: tex}
		c.items[sourceID] = e
	}
	switch f.Format {
	case frame.FormatI420:
		_ = e.tex.UpdateYUV(nil, f.Planes[0], int32(f.Strides[0]), f.Planes[1], int32(f.Strides[1]), f.Planes[2], int32(f.Strides[2]))
	default:
		_ = e.tex.UpdateNV(nil, f.Planes[0], int32(f.Strides[0]), f.Planes[1], int32(f.Strides[1]))
	}
	return e.tex
}

func uploadable(f *frame.Frame) bool {
	switch f.Format {
	case frame.FormatI420:
		return len(f.Planes) >= 3 && len(f.Strides) >= 3
	case frame.FormatNV12:
		return len(f.Planes) >= 2 && len(f.Strides) >= 2
	default:
		return false
	}
}

// Get returns the current texture for the source, if any.
func (c *Cache) Get(sourceID uint) *sdl.Texture {
	if c == nil {
		return nil
	}
	if e, ok := c.items[sourceID]; ok {
		return e.tex
	}
	return nil
}

// Drop removes the texture for a source.
func (c *Cache) Drop(sourceID uint) {
	if c == nil {
		return
	}
	e, ok := c.items[sourceID]
	if !ok {
		return
	}
	if c.r != nil && e.tex != nil {
		e.tex.Destroy()
	}
	delete(c.items, sourceID)
}

// Len is the number of live textures. Used by tests to assert no growth.
func (c *Cache) Len() int {
	if c == nil {
		return 0
	}
	return len(c.items)
}

// Abandon drops texture pointers without Destroy. Used when the window is already gone.
func (c *Cache) Abandon() {
	if c == nil {
		return
	}
	c.items = map[uint]*entry{}
	c.r = nil
}

// Close destroys every texture.
func (c *Cache) Close() {
	if c == nil {
		return
	}
	if c.r == nil {
		c.items = map[uint]*entry{}
		return
	}
	for id, e := range c.items {
		if e.tex != nil {
			e.tex.Destroy()
		}
		delete(c.items, id)
	}
}

func createYUV(r *sdl.Renderer, f *frame.Frame) (*sdl.Texture, error) {
	pf := sdl.PIXELFORMAT_NV12
	if f.Format == frame.FormatI420 {
		pf = sdl.PIXELFORMAT_IYUV
	}
	props, err := sdl.CreateProperties()
	if err != nil {
		return r.CreateTexture(pf, sdl.TEXTUREACCESS_STREAMING, f.Width, f.Height)
	}
	defer props.Destroy()
	cs := sdlColorspace(f)
	_ = props.SetNumberProperty("SDL.texture.create.colorspace", cs)
	_ = props.SetNumberProperty("SDL.texture.create.format", int64(pf))
	_ = props.SetNumberProperty("SDL.texture.create.access", int64(sdl.TEXTUREACCESS_STREAMING))
	_ = props.SetNumberProperty("SDL.texture.create.width", int64(f.Width))
	_ = props.SetNumberProperty("SDL.texture.create.height", int64(f.Height))
	tex, err := r.CreateTextureWithProperties(props)
	if err != nil {
		return r.CreateTexture(pf, sdl.TEXTUREACCESS_STREAMING, f.Width, f.Height)
	}
	return tex, nil
}

func sdlColorspace(f *frame.Frame) int64 {
	full := f.Color.Range == "full"
	switch f.Color.Space {
	case "bt601":
		if full {
			return int64(sdl.COLORSPACE_BT601_FULL)
		}
		return int64(sdl.COLORSPACE_BT601_LIMITED)
	case "bt2020":
		if full {
			return int64(sdl.COLORSPACE_BT2020_FULL)
		}
		return int64(sdl.COLORSPACE_BT2020_LIMITED)
	default:
		if full {
			return int64(sdl.COLORSPACE_BT709_FULL)
		}
		return int64(sdl.COLORSPACE_BT709_LIMITED)
	}
}
