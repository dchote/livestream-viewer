package capability

import (
	"testing"

	"github.com/asticode/go-astiav"
)

func TestChooseH264RequiresM2MNode(t *testing.T) {
	t.Parallel()
	// Decoder name alone must not claim H.264 HW (Pi 5 case).
	p := chooseH264(false, false, nil)
	if p.Method != MethodNone {
		t.Fatalf("no M2M nodes: got %#v", p)
	}
}

func TestChooseH264V4L2M2M(t *testing.T) {
	t.Parallel()
	if astiav.FindDecoderByName("h264_v4l2m2m") == nil {
		t.Skip("h264_v4l2m2m not in this libav build")
	}
	p := chooseH264(false, false, []string{"/dev/video10"})
	if p.Method != MethodV4L2M2M || p.Decoder != "h264_v4l2m2m" || p.UseHWCtx {
		t.Fatalf("v4l2m2m path: %#v", p)
	}
}

func TestChooseH264PrefersVideoToolbox(t *testing.T) {
	t.Parallel()
	if astiav.FindDecoderByName("h264") == nil {
		t.Skip("h264 decoder missing")
	}
	p := chooseH264(true, false, []string{"/dev/video10"})
	if p.Method != MethodVideoToolbox || !p.UseHWCtx {
		t.Fatalf("videotoolbox: %#v", p)
	}
}

func TestChooseHEVCRejectsV4L2M2MName(t *testing.T) {
	t.Parallel()
	// hevc_v4l2m2m is the wrong Pi path; drm + media/hevc node is required.
	p := chooseHEVC(false, false, false, []string{"/dev/video19"}, nil)
	if p.Method != MethodNone {
		t.Fatalf("without drm: %#v", p)
	}
}

func TestPathForAndSupports(t *testing.T) {
	t.Parallel()
	info := WithPaths(
		Path{Method: MethodV4L2M2M, Decoder: "h264_v4l2m2m"},
		Path{Method: MethodDRM, UseHWCtx: true, HWType: astiav.HardwareDeviceTypeDRM},
	)

	p, ok := info.PathFor("avc1")
	if !ok || p.Decoder != "h264_v4l2m2m" {
		t.Fatalf("h264 path: %#v ok=%v", p, ok)
	}
	if !info.Supports("h265") {
		t.Fatal("hevc should be supported")
	}
	if info.Supports("vp9") {
		t.Fatal("vp9 must not claim HW")
	}
	if !info.HasAnyHW() {
		t.Fatal("HasAnyHW")
	}
}

func TestHasDecodeM2MSkipsEncodeOnly(t *testing.T) {
	t.Parallel()
	// Without sysfs (temp paths), encode-only detection falls through to
	// accepting any node — exercise isEncodeOnlyName directly.
	if !isEncodeOnlyName("bcm2835-codec-encode") {
		t.Fatal("encode-only")
	}
	if isEncodeOnlyName("bcm2835-codec-decode") {
		t.Fatal("decode must not be encode-only")
	}
	if isEncodeOnlyName("rpi-hevc-dec") {
		t.Fatal("hevc dec")
	}
}

func TestIsNoisyHWAccelLog(t *testing.T) {
	t.Parallel()
	if !isNoisyHWAccelLog("[h264 @ 0x1] hardware accelerator failed to decode picture") {
		t.Fatal("vt picture failure")
	}
	if !isNoisyHWAccelLog("vt decoder cb: output image buffer is null: -12909") {
		t.Fatal("vt null buffer")
	}
	if isNoisyHWAccelLog("open: Connection refused") {
		t.Fatal("real errors must not be filtered")
	}
}

func TestLooksLikeM2MName(t *testing.T) {
	t.Parallel()
	if !looksLikeM2MName("bcm2835-codec-decode") {
		t.Fatal("codec-decode")
	}
	if looksLikeM2MName("unicam-image") {
		t.Fatal("camera capture is not M2M")
	}
}
