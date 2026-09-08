package capability

import (
	"testing"

	"github.com/asticode/go-astiav"
)

func TestCanHWH264OnDRMRequiresV4L2M2M(t *testing.T) {
	t.Parallel()
	info := Info{HWType: astiav.HardwareDeviceTypeDRM, HWTypeName: "drm"}
	if info.canHW("h264") {
		t.Fatal("DRM must not claim H.264 without h264_v4l2m2m")
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
