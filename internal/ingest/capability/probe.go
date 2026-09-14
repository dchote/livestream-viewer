package capability

import (
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/asticode/go-astiav"
)

// DecodeMethod names the hardware path chosen for a codec on this host.
type DecodeMethod string

const (
	MethodNone         DecodeMethod = ""
	MethodVideoToolbox DecodeMethod = "videotoolbox"
	MethodVAAPI        DecodeMethod = "vaapi"
	MethodV4L2M2M      DecodeMethod = "v4l2m2m"
	MethodDRM          DecodeMethod = "drm"
)

// Path is how OpenVideo should decode one codec on this host.
//
// V4L2 M2M (Pi 4/CM4 H.264) uses a named decoder and no FFmpeg hwdevice
// context — frames land in system memory as YUV420P. VideoToolbox, VA-API,
// and DRM attach a hardware device context to the generic decoder.
type Path struct {
	Method   DecodeMethod
	Decoder  string // named decoder, or empty for FindDecoder(codecID)
	UseHWCtx bool
	HWType   astiav.HardwareDeviceType
}

// Info is the once-probed hardware decode surface of this host.
type Info struct {
	BoardModel   string   `json:"board_model,omitempty"`
	VideoToolbox bool     `json:"videotoolbox"`
	VAAPI        bool     `json:"vaapi"`
	V4L2         []string `json:"v4l2"`
	V4L2M2M      []string `json:"v4l2_m2m"`
	Media        []string `json:"media,omitempty"`
	DRM          []string `json:"drm"`
	H264HW       bool     `json:"h264_hw"`
	HEVCHW       bool     `json:"hevc_hw"`
	// HWTypeName is the preferred path label for UI (h264 path, else hevc).
	HWTypeName string       `json:"hw_type,omitempty"`
	H264Path   DecodeMethod `json:"h264_path,omitempty"`
	HEVCPath   DecodeMethod `json:"hevc_path,omitempty"`

	h264 Path
	hevc Path
}

var (
	once   sync.Once
	cached Info
)

// Probe enumerates libav hardware device types and Linux nodes. Cached after the first call.
func Probe() Info {
	once.Do(func() {
		astiav.SetLogLevel(astiav.LogLevelError)
		astiav.SetLogCallback(quietHWAccelLog)
		cached = probe()
		slog.Info("hardware decode probe",
			"board", cached.BoardModel,
			"h264_hw", cached.H264HW,
			"h264_path", cached.H264Path,
			"hevc_hw", cached.HEVCHW,
			"hevc_path", cached.HEVCPath,
			"v4l2_m2m", cached.V4L2M2M,
			"drm", cached.DRM,
		)
	})
	return cached
}

func probe() Info {
	info := Info{
		V4L2:    []string{},
		V4L2M2M: []string{},
		Media:   []string{},
		DRM:     []string{},
	}

	if runtime.GOOS == "linux" {
		info.BoardModel = readBoardModel()
		info.V4L2 = globFiles("/dev/video*")
		info.Media = globFiles("/dev/media*")
		info.DRM = globFiles("/dev/dri/card*", "/dev/dri/renderD*")
		info.V4L2M2M = classifyV4L2M2M(info.V4L2)
	}

	vtOK := tryHWDevice("videotoolbox", astiav.HardwareDeviceTypeVideoToolbox)
	vaOK := tryHWDevice("vaapi", astiav.HardwareDeviceTypeVAAPI)
	drmOK := tryHWDevice("drm", astiav.HardwareDeviceTypeDRM)
	info.VideoToolbox = vtOK
	info.VAAPI = vaOK

	info.h264 = chooseH264(vtOK, vaOK, info.V4L2M2M)
	info.hevc = chooseHEVC(vtOK, vaOK, drmOK, info.V4L2M2M, info.Media)
	info.H264HW = info.h264.Method != MethodNone
	info.HEVCHW = info.hevc.Method != MethodNone
	info.H264Path = info.h264.Method
	info.HEVCPath = info.hevc.Method
	if info.h264.Method != MethodNone {
		info.HWTypeName = string(info.h264.Method)
	} else if info.hevc.Method != MethodNone {
		info.HWTypeName = string(info.hevc.Method)
	}
	return info
}

func tryHWDevice(name string, t astiav.HardwareDeviceType) bool {
	if astiav.FindHardwareDeviceTypeByName(name) == astiav.HardwareDeviceTypeNone {
		return false
	}
	hdc, err := astiav.CreateHardwareDeviceContext(t, "", nil, 0)
	if err != nil {
		slog.Debug("hwdevice unavailable", "type", name, "err", err)
		return false
	}
	hdc.Free()
	return true
}

func chooseH264(vtOK, vaOK bool, m2m []string) Path {
	if vtOK && astiav.FindDecoderByName("h264") != nil {
		return Path{Method: MethodVideoToolbox, UseHWCtx: true, HWType: astiav.HardwareDeviceTypeVideoToolbox}
	}
	if vaOK && astiav.FindDecoderByName("h264") != nil {
		return Path{Method: MethodVAAPI, UseHWCtx: true, HWType: astiav.HardwareDeviceTypeVAAPI}
	}
	// Stateful V4L2 M2M. Require both the named decoder and a real M2M node so
	// a Pi 5 (no H.264 block) with an FFmpeg that ships h264_v4l2m2m is not
	// claimed as hardware-capable.
	if len(m2m) > 0 && hasDecodeM2M(m2m) && astiav.FindDecoderByName("h264_v4l2m2m") != nil {
		return Path{Method: MethodV4L2M2M, Decoder: "h264_v4l2m2m"}
	}
	return Path{}
}

func chooseHEVC(vtOK, vaOK, drmOK bool, m2m, media []string) Path {
	if vtOK && astiav.FindDecoderByName("hevc") != nil {
		return Path{Method: MethodVideoToolbox, UseHWCtx: true, HWType: astiav.HardwareDeviceTypeVideoToolbox}
	}
	if vaOK && astiav.FindDecoderByName("hevc") != nil {
		return Path{Method: MethodVAAPI, UseHWCtx: true, HWType: astiav.HardwareDeviceTypeVAAPI}
	}
	// Pi HEVC is stateless V4L2-request via the drm hwaccel, not hevc_v4l2m2m.
	if drmOK && astiav.FindDecoderByName("hevc") != nil && (len(media) > 0 || hasHEVCNode(m2m)) {
		return Path{Method: MethodDRM, UseHWCtx: true, HWType: astiav.HardwareDeviceTypeDRM}
	}
	return Path{}
}

func hasDecodeM2M(nodes []string) bool {
	for _, n := range nodes {
		if isEncodeOnlyName(v4l2SysfsName(n)) {
			continue
		}
		return true
	}
	return false
}

func hasHEVCNode(nodes []string) bool {
	for _, n := range nodes {
		name := strings.ToLower(v4l2SysfsName(n))
		if strings.Contains(name, "hevc") || strings.Contains(name, "h265") || strings.Contains(name, "rpivid") {
			return true
		}
	}
	return false
}

// PathFor returns the decode path for a codec name (h264, hevc, …).
func (i Info) PathFor(codec string) (Path, bool) {
	switch normalizeCodec(codec) {
	case "h264":
		if i.h264.Method == MethodNone {
			return Path{}, false
		}
		return i.h264, true
	case "hevc":
		if i.hevc.Method == MethodNone {
			return Path{}, false
		}
		return i.hevc, true
	default:
		return Path{}, false
	}
}

// HWType is the FFmpeg hardware device type for paths that need a context.
// Empty / None for V4L2 M2M.
func (i Info) HWType() astiav.HardwareDeviceType {
	if p, ok := i.PathFor("h264"); ok && p.UseHWCtx {
		return p.HWType
	}
	if p, ok := i.PathFor("hevc"); ok && p.UseHWCtx {
		return p.HWType
	}
	return astiav.HardwareDeviceTypeNone
}

// Supports reports whether this codec name can use hardware decode here.
func (i Info) Supports(codec string) bool {
	_, ok := i.PathFor(codec)
	return ok
}

// HasAnyHW is true when any supported hardware decode path exists.
func (i Info) HasAnyHW() bool {
	return i.H264HW || i.HEVCHW
}

// WithPaths builds an Info for tests and policy checks without running Probe.
func WithPaths(h264, hevc Path) Info {
	info := Info{
		h264:     h264,
		hevc:     hevc,
		H264HW:   h264.Method != MethodNone,
		HEVCHW:   hevc.Method != MethodNone,
		H264Path: h264.Method,
		HEVCPath: hevc.Method,
	}
	if h264.Method == MethodVideoToolbox || hevc.Method == MethodVideoToolbox {
		info.VideoToolbox = true
	}
	if h264.Method == MethodVAAPI || hevc.Method == MethodVAAPI {
		info.VAAPI = true
	}
	if info.H264HW {
		info.HWTypeName = string(h264.Method)
	} else if info.HEVCHW {
		info.HWTypeName = string(hevc.Method)
	}
	return info
}

func normalizeCodec(codec string) string {
	c := strings.ToLower(strings.TrimSpace(codec))
	switch c {
	case "h264", "avc1", "avc":
		return "h264"
	case "hevc", "h265", "hev1", "hvc1":
		return "hevc"
	default:
		return c
	}
}

func quietHWAccelLog(_ astiav.Classer, l astiav.LogLevel, _, msg string) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return
	}
	if isNoisyHWAccelLog(msg) {
		return
	}
	if l <= astiav.LogLevelError {
		slog.Warn("libav", "msg", msg)
	}
}

func isNoisyHWAccelLog(msg string) bool {
	return strings.Contains(msg, "hardware accelerator failed to decode picture") ||
		strings.Contains(msg, "vt decoder cb:") ||
		strings.Contains(msg, "output image buffer is null")
}

func globFiles(patterns ...string) []string {
	var out []string
	for _, p := range patterns {
		matches, _ := filepath.Glob(p)
		for _, m := range matches {
			if st, err := os.Stat(m); err == nil && !st.IsDir() {
				out = append(out, m)
			}
		}
	}
	if out == nil {
		return []string{}
	}
	return out
}

func readBoardModel() string {
	b, err := os.ReadFile("/proc/device-tree/model")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.TrimRight(string(b), "\x00"))
}

func v4l2SysfsName(dev string) string {
	base := filepath.Base(dev)
	b, err := os.ReadFile(filepath.Join("/sys/class/video4linux", base, "name"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func isEncodeOnlyName(name string) bool {
	n := strings.ToLower(name)
	if n == "" {
		return false
	}
	hasEncode := strings.Contains(n, "encode")
	hasDecode := strings.Contains(n, "decode") ||
		strings.HasSuffix(n, "-dec") ||
		strings.Contains(n, "-dec-") ||
		strings.Contains(n, "decoder")
	return hasEncode && !hasDecode
}

func looksLikeM2MName(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return false
	}
	markers := []string{
		"codec-decode", "codec-encode", "codec-image",
		"mem2mem", "m2m", "hevc-dec", "h264", "rpivid",
	}
	for _, m := range markers {
		if strings.Contains(n, m) {
			return true
		}
	}
	return false
}
