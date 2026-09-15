package capability

import (
	"testing"

	"github.com/asticode/go-astiav"
)

func TestChooseH264RequiresM2MNode(t *testing.T) {
	t.Parallel()
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
	old := readV4L2Name
	readV4L2Name = func(string) string { return "bcm2835-codec-decode" }
	t.Cleanup(func() { readV4L2Name = old })

	p := chooseH264(false, false, []string{"/dev/video10"})
	if p.Method != MethodV4L2M2M || p.Decoder != "h264_v4l2m2m" || p.UseHWCtx {
		t.Fatalf("v4l2m2m path: %#v", p)
	}
}

func TestChooseH264RejectsHEVCOnlyM2M(t *testing.T) {
	t.Parallel()
	if astiav.FindDecoderByName("h264_v4l2m2m") == nil {
		t.Skip("h264_v4l2m2m not in this libav build")
	}
	old := readV4L2Name
	readV4L2Name = func(string) string { return "rpivid" }
	t.Cleanup(func() { readV4L2Name = old })

	p := chooseH264(false, false, []string{"/dev/video19"})
	if p.Method != MethodNone {
		t.Fatalf("Pi 5 HEVC-only M2M must not claim H.264: %#v", p)
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

func TestChooseHEVCRequiresNamedHEVCNode(t *testing.T) {
	t.Parallel()
	old := readV4L2Name
	readV4L2Name = func(string) string { return "unicam-image" }
	t.Cleanup(func() { readV4L2Name = old })

	p := chooseHEVC(false, false, true, []string{"/dev/video0"})
	if p.Method != MethodNone {
		t.Fatalf("camera/ISP node must not claim HEVC: %#v", p)
	}

	readV4L2Name = func(string) string { return "rpivid" }
	if astiav.FindDecoderByName("hevc") == nil {
		t.Skip("hevc decoder missing")
	}
	p = chooseHEVC(false, false, true, []string{"/dev/video19"})
	if p.Method != MethodDRM || !p.UseHWCtx {
		t.Fatalf("rpivid + drm: %#v", p)
	}
}

func TestChooseHEVCRejectsV4L2M2MName(t *testing.T) {
	t.Parallel()
	p := chooseHEVC(false, false, false, []string{"/dev/video19"})
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

func TestIsH264DecodeName(t *testing.T) {
	t.Parallel()
	if !isH264DecodeName("bcm2835-codec-decode") {
		t.Fatal("pi4 decode")
	}
	if isH264DecodeName("bcm2835-codec-encode") {
		t.Fatal("encode")
	}
	if isH264DecodeName("rpivid") {
		t.Fatal("hevc")
	}
	if isH264DecodeName("bcm2835-codec-image") {
		t.Fatal("image fx")
	}
}

func TestHasDecodeM2MSkipsEncodeOnly(t *testing.T) {
	t.Parallel()
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
	if looksLikeM2MName("something-h264-capture") {
		t.Fatal("bare h264 marker removed")
	}
}
