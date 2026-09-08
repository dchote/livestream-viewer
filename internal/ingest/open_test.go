package ingest

import (
	"testing"

	"github.com/asticode/go-astiav"
	"github.com/dchote/livestream-viewer/internal/model"
)

func TestDictionaryRTSPAndFile(t *testing.T) {
	rtsp := OpenOptions{URL: "rtsps://example.invalid/cam", Kind: model.KindRTSP, Transport: "tcp"}
	d := rtsp.Dictionary()
	if d == nil {
		t.Fatal("rtsp dict")
	}
	if dictValue(d, "live_start_index") != "" {
		t.Fatal("rtsp must not get HLS live options")
	}
	d.Free()

	file := OpenOptions{URL: "/tmp/x.mp4", Kind: model.KindFile}
	d = file.Dictionary()
	if d == nil {
		t.Fatal("file dict")
	}
	if dictValue(d, "fflags") != "nobuffer" {
		t.Fatalf("file fflags = %q", dictValue(d, "fflags"))
	}
	d.Free()
}

func TestDictionaryYouTubeHLSBuffer(t *testing.T) {
	yt := OptionsFromSource(&model.Source{Kind: model.KindYouTube}, "https://manifest.googlevideo.com/api/manifest/hls_playlist/x")
	if yt.BufferMS != model.DefaultBufferMSSegmented {
		t.Fatalf("default buffer %d", yt.BufferMS)
	}
	d := yt.Dictionary()
	defer d.Free()
	if dictValue(d, "live_start_index") != "-3" {
		t.Fatalf("live_start_index = %q", dictValue(d, "live_start_index"))
	}
	if dictValue(d, "http_multiple") != "1" {
		t.Fatalf("http_multiple = %q", dictValue(d, "http_multiple"))
	}
	if dictValue(d, "fflags") != "" {
		t.Fatalf("smooth HLS must not set nobuffer, got %q", dictValue(d, "fflags"))
	}
	if dictValue(d, "max_delay") != "" {
		t.Fatalf("smooth HLS must not set max_delay=0, got %q", dictValue(d, "max_delay"))
	}

	zero := 0
	edge := OptionsFromSource(&model.Source{
		Kind:    model.KindYouTube,
		Options: model.SourceOptions{BufferMS: &zero},
	}, "https://manifest.googlevideo.com/api/manifest/hls_playlist/x")
	d2 := edge.Dictionary()
	defer d2.Free()
	if dictValue(d2, "live_start_index") != "-1" {
		t.Fatalf("live-edge live_start_index = %q", dictValue(d2, "live_start_index"))
	}
	if dictValue(d2, "fflags") != "nobuffer" {
		t.Fatalf("live-edge fflags = %q", dictValue(d2, "fflags"))
	}

	hls := OptionsFromSource(&model.Source{Kind: model.KindHLS}, "https://example.invalid/live.m3u8")
	d3 := hls.Dictionary()
	defer d3.Free()
	if dictValue(d3, "live_start_index") != "-3" {
		t.Fatalf("hls live_start_index = %q", dictValue(d3, "live_start_index"))
	}

	dash := OptionsFromSource(&model.Source{Kind: model.KindDASH}, "https://example.invalid/manifest.mpd")
	d4 := dash.Dictionary()
	defer d4.Free()
	if dictValue(d4, "live_start_index") != "" {
		t.Fatal("dash must not get HLS-private options")
	}
}

func TestLiveStartIndex(t *testing.T) {
	t.Parallel()
	if liveStartIndex(0) != -1 {
		t.Fatal("zero")
	}
	if liveStartIndex(4000) != -3 {
		t.Fatalf("4000 -> %d", liveStartIndex(4000))
	}
	if liveStartIndex(8000) != -5 {
		t.Fatalf("8000 -> %d", liveStartIndex(8000))
	}
	if liveStartIndex(30000) != -5 {
		t.Fatalf("cap -> %d", liveStartIndex(30000))
	}
}

func TestShouldPace(t *testing.T) {
	t.Parallel()
	if !shouldPace(model.Source{Kind: model.KindYouTube}) {
		t.Fatal("youtube")
	}
	if shouldPace(model.Source{Kind: model.KindRTSP, URL: "rtsp://cam"}) {
		t.Fatal("rtsp")
	}
	if !shouldPace(model.Source{Kind: model.KindHTTP, URL: "http://x/live.m3u8"}) {
		t.Fatal("m3u8 url")
	}
}

func dictValue(d *astiav.Dictionary, key string) string {
	e := d.Get(key, nil, astiav.NewDictionaryFlags())
	if e == nil {
		return ""
	}
	return e.Value()
}
