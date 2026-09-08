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

// Info is the once-probed hardware decode surface of this host.
type Info struct {
	VideoToolbox bool                      `json:"videotoolbox"`
	VAAPI        bool                      `json:"vaapi"`
	V4L2         []string                  `json:"v4l2"`
	DRM          []string                  `json:"drm"`
	H264HW       bool                      `json:"h264_hw"`
	HEVCHW       bool                      `json:"hevc_hw"`
	HWTypeName   string                    `json:"hw_type,omitempty"`
	HWType       astiav.HardwareDeviceType `json:"-"`
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
	})
	return cached
}

func probe() Info {
	info := Info{
		V4L2:   []string{},
		DRM:    []string{},
		HWType: astiav.HardwareDeviceTypeNone,
	}

	try := func(name string, t astiav.HardwareDeviceType) bool {
		if astiav.FindHardwareDeviceTypeByName(name) == astiav.HardwareDeviceTypeNone {
			return false
		}
		hdc, err := astiav.CreateHardwareDeviceContext(t, "", nil, 0)
		if err != nil {
			return false
		}
		hdc.Free()
		return true
	}

	if try("videotoolbox", astiav.HardwareDeviceTypeVideoToolbox) {
		info.VideoToolbox = true
		info.HWType = astiav.HardwareDeviceTypeVideoToolbox
		info.HWTypeName = "videotoolbox"
	}
	if try("vaapi", astiav.HardwareDeviceTypeVAAPI) {
		info.VAAPI = true
		if info.HWType == astiav.HardwareDeviceTypeNone {
			info.HWType = astiav.HardwareDeviceTypeVAAPI
			info.HWTypeName = "vaapi"
		}
	}
	if try("drm", astiav.HardwareDeviceTypeDRM) {
		if info.HWType == astiav.HardwareDeviceTypeNone {
			info.HWType = astiav.HardwareDeviceTypeDRM
			info.HWTypeName = "drm"
		}
	}

	if runtime.GOOS == "linux" {
		info.V4L2 = globNames("/dev/video*")
		info.DRM = globNames("/dev/dri/card*", "/dev/dri/renderD*")
	}

	info.H264HW = info.canHW("h264")
	info.HEVCHW = info.canHW("hevc")
	return info
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

func globNames(patterns ...string) []string {
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

func (i Info) canHW(codec string) bool {
	if i.HWType == astiav.HardwareDeviceTypeNone {
		return false
	}
	drm := i.HWType == astiav.HardwareDeviceTypeDRM || i.HWTypeName == "drm"
	if drm {
		// Pi 5 exposes a DRM device but has no H.264 block. Claiming H.264
		// here would open a hardware session that never produces a frame.
		if codec == "h264" {
			return astiav.FindDecoderByName("h264_v4l2m2m") != nil
		}
		if astiav.FindDecoderByName(codec+"_"+i.HWTypeName) != nil {
			return true
		}
		return astiav.FindDecoderByName(codec) != nil
	}
	name := codec + "_" + i.HWTypeName
	if astiav.FindDecoderByName(name) != nil {
		return true
	}
	// Generic decoder + hwaccel device is enough on VideoToolbox / VA-API.
	return astiav.FindDecoderByName(codec) != nil && (i.VideoToolbox || i.VAAPI)
}

// Supports reports whether this codec name can use hardware decode here.
func (i Info) Supports(codec string) bool {
	c := strings.ToLower(strings.TrimSpace(codec))
	switch c {
	case "h264", "avc1", "avc":
		return i.H264HW
	case "hevc", "h265", "hev1", "hvc1":
		return i.HEVCHW
	default:
		return i.canHW(c)
	}
}
