package ingest

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/dchote/livestream-viewer/internal/frame"
	"github.com/dchote/livestream-viewer/internal/ingest/capability"
	"github.com/dchote/livestream-viewer/internal/model"
)

// Session is an open demux+decode pipeline. Close releases libav resources.
type Session struct {
	Format    *astiav.FormatContext
	Codec     *astiav.CodecContext
	Stream    *astiav.Stream
	UsingHW   bool
	interrupt *astiav.IOInterrupter
	hdc       *astiav.HardwareDeviceContext
	cancel    func() bool
}

// OpenVideo opens a URL and a video decoder. The caller must Close the session.
func OpenVideo(ctx context.Context, opts OpenOptions, caps capability.Info, wantHW, allowSW bool) (*Session, error) {
	fc := astiav.AllocFormatContext()
	if fc == nil {
		return nil, fmt.Errorf("alloc format context")
	}
	ii := astiav.NewIOInterrupter()
	fc.SetIOInterrupter(ii)
	stop := context.AfterFunc(ctx, ii.Interrupt)
	if ctx.Err() != nil {
		ii.Interrupt()
	}

	dict := opts.Dictionary()
	if err := fc.OpenInput(opts.URL, nil, dict); err != nil {
		dict.Free()
		stop()
		ii.Free()
		fc.Free()
		return nil, fmt.Errorf("open: %w", err)
	}
	dict.Free()

	if err := fc.FindStreamInfo(nil); err != nil {
		stop()
		fc.CloseInput()
		fc.Free()
		ii.Free()
		return nil, fmt.Errorf("stream info: %w", err)
	}

	var in *astiav.Stream
	for _, st := range fc.Streams() {
		if st.CodecParameters().MediaType() == astiav.MediaTypeVideo {
			in = st
			break
		}
	}
	if in == nil {
		stop()
		fc.CloseInput()
		fc.Free()
		ii.Free()
		return nil, fmt.Errorf("no video stream")
	}
	videoIdx := in.Index()
	for _, st := range fc.Streams() {
		if st.Index() != videoIdx {
			st.SetDiscard(astiav.DiscardAll)
		}
	}

	codecID := in.CodecParameters().CodecID()
	usingHW := wantHW && caps.HWType != astiav.HardwareDeviceTypeNone
	dec := pickDecoder(codecID, caps, usingHW)
	if dec == nil {
		stop()
		fc.CloseInput()
		fc.Free()
		ii.Free()
		return nil, fmt.Errorf("no decoder for %s", codecID.Name())
	}

	cc, hdc, err := openCodec(in, dec, caps, usingHW, opts.BufferMS <= 0)
	if err != nil && usingHW && allowSW {
		usingHW = false
		sw := astiav.FindDecoder(codecID)
		if sw != nil {
			cc, hdc, err = openCodec(in, sw, caps, false, opts.BufferMS <= 0)
		}
	}
	if err != nil {
		stop()
		fc.CloseInput()
		fc.Free()
		ii.Free()
		return nil, err
	}

	return &Session{
		Format:    fc,
		Codec:     cc,
		Stream:    in,
		UsingHW:   usingHW,
		interrupt: ii,
		hdc:       hdc,
		cancel:    stop,
	}, nil
}

// pickDecoder prefers a named hwaccel decoder when one actually exists.
// VideoToolbox has no h264_videotoolbox decoder (that name is an encoder);
// decode uses the generic H.264 decoder plus a hardware device context.
func pickDecoder(id astiav.CodecID, caps capability.Info, hw bool) *astiav.Codec {
	if hw && caps.HWTypeName != "" && caps.HWTypeName != "videotoolbox" {
		if d := astiav.FindDecoderByName(id.Name() + "_" + caps.HWTypeName); d != nil {
			return d
		}
	}
	return astiav.FindDecoder(id)
}

func openCodec(in *astiav.Stream, codec *astiav.Codec, caps capability.Info, hw, lowDelay bool) (*astiav.CodecContext, *astiav.HardwareDeviceContext, error) {
	cc := astiav.AllocCodecContext(codec)
	if cc == nil {
		return nil, nil, fmt.Errorf("alloc codec context")
	}
	if err := in.CodecParameters().ToCodecContext(cc); err != nil {
		cc.Free()
		return nil, nil, err
	}
	if lowDelay && !hw {
		// LOW_DELAY is for software live decode. VideoToolbox has its own
		// reorder buffer; combining the two produces
		// "vt decoder cb: output image buffer is null" and
		// "hardware accelerator failed to decode picture".
		cc.SetFlags(cc.Flags().Add(astiav.CodecContextFlagLowDelay))
	}
	var hdc *astiav.HardwareDeviceContext
	if hw && caps.HWType != astiav.HardwareDeviceTypeNone {
		var err error
		hdc, err = astiav.CreateHardwareDeviceContext(caps.HWType, "", nil, 0)
		if err != nil {
			cc.Free()
			return nil, nil, err
		}
		hwPix := hardwarePixel(hdc, caps)
		cc.SetHardwareDeviceContext(hdc)
		cc.SetExtraHardwareFrames(16)
		cc.SetThreadCount(1)
		cc.SetPixelFormatCallback(func(pfs []astiav.PixelFormat) astiav.PixelFormat {
			for _, pf := range pfs {
				if pf == hwPix || isHWPixel(pf) {
					return pf
				}
			}
			return astiav.PixelFormatNone
		})
	} else {
		hw = false
		if lowDelay {
			cc.SetThreadCount(1)
		}
	}
	var copts *astiav.Dictionary
	if hw {
		copts = astiav.NewDictionary()
		_ = copts.Set("hwaccel_flags", "+allow_profile_mismatch+ignore_level", astiav.NewDictionaryFlags())
	}
	err := cc.Open(codec, copts)
	if copts != nil {
		copts.Free()
	}
	if err != nil {
		cc.Free()
		if hdc != nil {
			hdc.Free()
		}
		return nil, nil, err
	}
	return cc, hdc, nil
}

func hardwarePixel(hdc *astiav.HardwareDeviceContext, caps capability.Info) astiav.PixelFormat {
	fallback := astiav.PixelFormatVideotoolbox
	switch caps.HWType {
	case astiav.HardwareDeviceTypeVAAPI:
		fallback = astiav.PixelFormatVaapi
	case astiav.HardwareDeviceTypeCUDA:
		fallback = astiav.PixelFormatCuda
	case astiav.HardwareDeviceTypeDRM:
		fallback = astiav.PixelFormatDrmPrime
	}
	if hdc == nil {
		return fallback
	}
	cons := hdc.HardwareFramesConstraints()
	if cons == nil {
		return fallback
	}
	defer cons.Free()
	if fmts := cons.ValidHardwarePixelFormats(); len(fmts) > 0 {
		return fmts[0]
	}
	return fallback
}

// Close frees the session.
func (s *Session) Close() {
	if s == nil {
		return
	}
	if s.interrupt != nil {
		s.interrupt.Interrupt()
	}
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.Codec != nil {
		s.Codec.Free()
		s.Codec = nil
	}
	if s.hdc != nil {
		s.hdc.Free()
		s.hdc = nil
	}
	if s.Format != nil {
		s.Format.CloseInput()
		s.Format.Free()
		s.Format = nil
	}
	if s.interrupt != nil {
		s.interrupt.Free()
		s.interrupt = nil
	}
}

func isHWPixel(pf astiav.PixelFormat) bool {
	switch pf {
	case astiav.PixelFormatVideotoolbox, astiav.PixelFormatVaapi, astiav.PixelFormatDrmPrime, astiav.PixelFormatCuda:
		return true
	default:
		return false
	}
}

// ShouldLoop reports whether a source should rewind on EOF.
func ShouldLoop(s model.Source) bool {
	if s.Kind == model.KindFile {
		if s.Options.Loop == nil {
			return true
		}
		return *s.Options.Loop
	}
	u := strings.ToLower(s.URL)
	return u != "" && !strings.Contains(u, "://")
}

func nativeYUV(pf astiav.PixelFormat) (frame.Format, bool) {
	switch pf {
	case astiav.PixelFormatNv12:
		return frame.FormatNV12, true
	case astiav.PixelFormatYuv420P, astiav.PixelFormatYuvj420P:
		return frame.FormatI420, true
	default:
		return 0, false
	}
}

// publishToSlot copies packed YUV from libav into a reserved slot buffer.
// There is no staging buffer: ImageCopyToBuffer writes the pool entry the
// renderer will read. AVFrame is unreferenced by the caller after this returns.
func publishToSlot(slot *frame.Slot, nv *astiav.Frame, tb astiav.Rational) error {
	if slot == nil || nv == nil {
		return fmt.Errorf("nil frame")
	}
	w, h := nv.Width(), nv.Height()
	if w <= 0 || h <= 0 {
		return fmt.Errorf("empty yuv")
	}
	format, ok := nativeYUV(nv.PixelFormat())
	if !ok {
		return fmt.Errorf("unsupported pixel format %s", nv.PixelFormat().Name())
	}
	size, err := nv.ImageBufferSize(1)
	if err != nil {
		return err
	}
	ySize := w * h
	if size <= 0 || size < ySize {
		return fmt.Errorf("yuv buffer size %d for %dx%d", size, w, h)
	}
	dst := slot.Prepare(w, h, format, size)
	if dst == nil {
		slot.Abort()
		return fmt.Errorf("slot prepare")
	}
	buf := dst.Packed()
	if len(buf) < size {
		slot.Abort()
		return fmt.Errorf("slot buffer short: %d < %d", len(buf), size)
	}
	n, err := nv.ImageCopyToBuffer(buf[:size], 1)
	if err != nil {
		slot.Abort()
		return err
	}
	need := packedYUVSize(w, h, format)
	if n < need {
		slot.Abort()
		return fmt.Errorf("yuv buffer short: %d < %d", n, need)
	}
	sliceYUV(dst, n)
	c := colorOf(nv)
	if nv.PixelFormat() == astiav.PixelFormatYuvj420P {
		c.Range = "full"
	}
	slot.Commit(ptsDuration(nv.Pts(), tb), c)
	return nil
}

func packedYUVSize(w, h int, format frame.Format) int {
	ySize := w * h
	if format == frame.FormatI420 {
		cw, ch := (w+1)/2, (h+1)/2
		return ySize + 2*cw*ch
	}
	return ySize + w*((h+1)/2)
}

func sliceYUV(f *frame.Frame, n int) {
	if f == nil || len(f.Planes) == 0 {
		return
	}
	buf := f.Packed()
	if n > len(buf) {
		n = len(buf)
	}
	ySize := f.Width * f.Height
	switch f.Format {
	case frame.FormatI420:
		cw, ch := (f.Width+1)/2, (f.Height+1)/2
		uSize := cw * ch
		end := ySize + 2*uSize
		if end > n {
			return
		}
		if len(f.Planes) != 3 {
			f.Planes = make([][]byte, 3)
			f.Strides = make([]int, 3)
		}
		f.Planes[0] = buf[:ySize]
		f.Planes[1] = buf[ySize : ySize+uSize]
		f.Planes[2] = buf[ySize+uSize : end]
		f.Strides[0], f.Strides[1], f.Strides[2] = f.Width, cw, cw
	default:
		uvSize := f.Width * ((f.Height + 1) / 2)
		end := ySize + uvSize
		if end > n {
			return
		}
		if len(f.Planes) != 2 {
			f.Planes = make([][]byte, 2)
			f.Strides = make([]int, 2)
		}
		f.Planes[0] = buf[:ySize]
		f.Planes[1] = buf[ySize:end]
		f.Strides[0], f.Strides[1] = f.Width, f.Width
	}
}

// ptsDuration converts a stream timestamp. Negative PTS is NOPTS and stays
// negative so the pacer and presentation clock can skip it. Zero is valid.
func ptsDuration(pts int64, tb astiav.Rational) time.Duration {
	if pts < 0 || tb.Den() == 0 {
		return -1
	}
	sec := tb.Float64() * float64(pts)
	return time.Duration(sec * float64(time.Second))
}

func colorOf(nv *astiav.Frame) frame.Color {
	c := frame.Color{Space: "bt709", Range: "limited"}
	switch nv.ColorSpace() {
	case astiav.ColorSpaceBt470Bg, astiav.ColorSpaceSmpte170M, astiav.ColorSpaceSmpte240M, astiav.ColorSpaceFcc:
		c.Space = "bt601"
	case astiav.ColorSpaceBt2020Ncl, astiav.ColorSpaceBt2020Cl:
		c.Space = "bt2020"
	}
	if nv.ColorRange() == astiav.ColorRangeJpeg {
		c.Range = "full"
	}
	return c
}
