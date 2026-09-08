package ingest

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/asticode/go-astiav"
	"github.com/dchote/livestream-viewer/internal/model"
)

// OpenOptions are libavformat input options derived from a source.
type OpenOptions struct {
	URL       string
	Transport string
	TLSVerify bool
	Kind      string
	BufferMS  int
}

// OptionsFromSource builds libav open options. Passwords stay on the Source row,
// not in the strategy snapshot.
func OptionsFromSource(s *model.Source, resolvedURL string) OpenOptions {
	u := resolvedURL
	if u == "" && s != nil {
		u = s.URL
	}
	opt := OpenOptions{URL: u, Transport: "tcp", TLSVerify: false}
	if s == nil {
		return opt
	}
	opt.Kind = s.Kind
	opt.BufferMS = s.EffectiveBufferMS()
	if s.Kind == model.KindRTSP {
		t := strings.ToLower(strings.TrimSpace(s.Options.Transport))
		if t == "udp" {
			opt.Transport = "udp"
		}
		u = InjectRTSPCredentials(u, s.Username, s.Password)
		opt.URL = u
		if s.Options.TLSVerify != nil {
			opt.TLSVerify = *s.Options.TLSVerify
		}
	}
	return opt
}

// InjectRTSPCredentials puts username/password into the URL userinfo.
func InjectRTSPCredentials(raw, username, password string) string {
	if username == "" && password == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.User = url.UserPassword(username, password)
	return u.String()
}

// Dictionary is the libavformat option dict for this open. Caller must Free it.
func (o OpenOptions) Dictionary() *astiav.Dictionary {
	d := astiav.NewDictionary()
	flags := astiav.NewDictionaryFlags()
	if o.Kind == model.KindRTSP || strings.HasPrefix(strings.ToLower(o.URL), "rtsp") {
		_ = d.Set("rtsp_transport", o.Transport, flags)
		_ = d.Set("rtsp_flags", "prefer_tcp", flags)
		// 20s socket timeouts. `fflags nobuffer` makes UniFi RTSPS look idle and
		// FFmpeg then tears down TLS with "Unknown error".
		_ = d.Set("stimeout", "20000000", flags)
		_ = d.Set("timeout", "20000000", flags)
		_ = d.Set("rw_timeout", "20000000", flags)
	} else if o.BufferMS <= 0 {
		_ = d.Set("fflags", "nobuffer", flags)
	}
	if o.hlsLive() {
		// live_start_index=-3 is FFmpeg's default and the smoothness profile:
		// two to three segments of compressed preroll. -1 is live-edge (buffer 0).
		_ = d.Set("live_start_index", strconv.Itoa(liveStartIndex(o.BufferMS)), flags)
		_ = d.Set("http_multiple", "1", flags)
		if o.BufferMS <= 0 {
			_ = d.Set("max_delay", "0", flags)
		}
	}
	if !o.TLSVerify && looksTLS(o.URL) {
		_ = d.Set("tls_verify", "0", flags)
	}
	return d
}

func liveStartIndex(bufferMS int) int {
	if bufferMS <= 0 {
		return -1
	}
	segs := bufferMS/2000 + 1
	if segs > 5 {
		segs = 5
	}
	return -segs
}

// shouldPace is true for sources that dump media faster than realtime
// (HLS/YouTube/DASH/files). RTSP packets already arrive on a clock.
func shouldPace(s model.Source) bool {
	switch s.Kind {
	case model.KindYouTube, model.KindHLS, model.KindDASH, model.KindFile:
		return true
	}
	u := strings.ToLower(s.URL)
	return strings.Contains(u, ".m3u8") || strings.Contains(u, "hls_playlist") || strings.Contains(u, ".mpd")
}

// hlsLive is true for YouTube and HLS inputs. Those demuxers deliver a whole
// MPEG-TS segment per read; RTSP must not get these options (UniFi is
// sensitive) and DASH must not get HLS-private keys or open fails.
func (o OpenOptions) hlsLive() bool {
	if o.Kind == model.KindYouTube || o.Kind == model.KindHLS {
		return true
	}
	u := strings.ToLower(o.URL)
	return strings.Contains(u, ".m3u8") || strings.Contains(u, "hls_playlist")
}

func looksTLS(u string) bool {
	l := strings.ToLower(u)
	return strings.HasPrefix(l, "rtsps://") || strings.HasPrefix(l, "https://") || strings.Contains(l, "srtp")
}
