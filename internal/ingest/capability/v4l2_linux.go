//go:build linux

package capability

import (
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// V4L2 capability bits / ioctls for memory-to-memory devices (videodev2.h).
// VIDIOC_QUERYCAP is _IOR('V', 0, struct v4l2_capability); x/sys/unix does not export it.
const (
	v4l2CapVideoM2M       = 0x00004000
	v4l2CapVideoM2MMplane = 0x00008000
	v4l2CapDeviceCaps     = 0x80000000
	vidIOCQueryCap        = 0x80685600
)

// v4l2Capability matches struct v4l2_capability on Linux.
type v4l2Capability struct {
	Driver       [16]byte
	Card         [32]byte
	BusInfo      [32]byte
	Version      uint32
	Capabilities uint32
	DeviceCaps   uint32
	Reserved     [3]uint32
}

func classifyV4L2M2M(nodes []string) []string {
	var out []string
	for _, n := range nodes {
		if isV4L2M2M(n) {
			out = append(out, n)
		}
	}
	if out == nil {
		return []string{}
	}
	return out
}

func isV4L2M2M(dev string) bool {
	if caps, ok := queryV4L2Caps(dev); ok {
		c := caps.Capabilities
		if c&v4l2CapDeviceCaps != 0 {
			c = caps.DeviceCaps
		}
		if c&(v4l2CapVideoM2M|v4l2CapVideoM2MMplane) != 0 {
			return true
		}
		// QUERYCAP succeeded but no M2M flag — trust the ioctl over sysfs.
		return false
	}
	return looksLikeM2MName(v4l2SysfsName(dev))
}

func queryV4L2Caps(dev string) (v4l2Capability, bool) {
	var zero v4l2Capability
	f, err := os.OpenFile(dev, os.O_RDWR|unix.O_NONBLOCK, 0)
	if err != nil {
		// Some nodes are write-only encode endpoints; try read-only.
		f, err = os.OpenFile(dev, os.O_RDONLY|unix.O_NONBLOCK, 0)
		if err != nil {
			return zero, false
		}
	}
	defer f.Close()

	var caps v4l2Capability
	_, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		f.Fd(),
		uintptr(vidIOCQueryCap),
		uintptr(unsafe.Pointer(&caps)),
	)
	if errno != 0 {
		return zero, false
	}
	return caps, true
}
