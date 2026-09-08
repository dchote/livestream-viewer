package frame

import "time"

// Format is the pixel layout of a Frame.
type Format int

const (
	// FormatNV12 is Y + interleaved UV. Hardware download and remaining
	// conversions land here.
	FormatNV12 Format = iota
	// FormatI420 is planar YUV 4:2:0 (Y, U, V). Software H.264 usually
	// produces this; uploading it as IYUV avoids a sysmem NV12 conversion.
	FormatI420
	// FormatDRMPrime is reserved for a later zero-copy path.
	FormatDRMPrime
)

// Color describes the YUV colorspace of a frame.
type Color struct {
	Space string // "bt709", "bt601", …
	Range string // "limited" (mpeg/tv) or "full" (jpeg)
}

// Frame is a system-memory video frame ready for the renderer.
type Frame struct {
	Width, Height int
	Format        Format
	Planes        [][]byte // Y, UV for NV12; Y, U, V for I420
	Strides       []int
	PTS           time.Duration
	Received      time.Time
	Color         Color
	Seq           uint64
}
