package ingest

import (
	"testing"

	"github.com/dchote/livestream-viewer/internal/ingest/capability"
	"github.com/dchote/livestream-viewer/internal/model"
	"github.com/dchote/livestream-viewer/internal/source/resolver"
)

func TestShouldUseHW(t *testing.T) {
	hw := true
	sw := false
	vt := capability.Info{VideoToolbox: true, H264HW: true, HWTypeName: "videotoolbox"}
	rtsp := model.Source{Kind: model.KindRTSP, Probe: model.ProbeResult{Codec: "h264"}}
	if ShouldUseHW(rtsp, vt) {
		t.Fatal("unprobed RTSP must not use VideoToolbox")
	}
	rtsp.Probe.HWDecode = &sw
	if ShouldUseHW(rtsp, vt) {
		t.Fatal("hw_decode false must skip hardware")
	}
	rtsp.Probe.HWDecode = &hw
	if !ShouldUseHW(rtsp, vt) {
		t.Fatal("probed hw_decode true should try VideoToolbox")
	}
	file := model.Source{Kind: model.KindFile, Probe: model.ProbeResult{Codec: "h264"}}
	if !ShouldUseHW(file, vt) {
		t.Fatal("file h264 should try VideoToolbox")
	}
	file.Options.ForceSoftware = true
	if ShouldUseHW(file, vt) {
		t.Fatal("force_software must skip hardware")
	}
}

func TestIngestFingerprintChangesWithProbeHW(t *testing.T) {
	t.Parallel()
	a := model.Source{Kind: model.KindRTSP, URL: "rtsp://cam", Probe: model.ProbeResult{Codec: "h264"}}
	b := a
	hw := true
	b.Probe.HWDecode = &hw
	if ingestFingerprint(a) == ingestFingerprint(b) {
		t.Fatal("hw_decode must restart the worker")
	}
}

func TestIngestFingerprintChangesWithBuffer(t *testing.T) {
	t.Parallel()
	a := model.Source{Kind: model.KindYouTube, URL: "https://youtube.com/watch?v=x"}
	b := a
	if ingestFingerprint(a) != ingestFingerprint(b) {
		t.Fatal("same source")
	}
	zero := 0
	b.Options.BufferMS = &zero
	if ingestFingerprint(a) == ingestFingerprint(b) {
		t.Fatal("explicit live-edge must restart a default-buffered YouTube worker")
	}
	b.Options.BufferMS = nil
	b.Options.ForceSoftware = true
	if ingestFingerprint(a) == ingestFingerprint(b) {
		t.Fatal("force_software must restart")
	}
}

func TestYouTubeAuthGenerationRestartsWorkers(t *testing.T) {
	t.Parallel()
	m := NewManager(nil, nil, capability.Info{}, resolver.Tools{}, nil)
	src := model.Source{Kind: model.KindYouTube, URL: "https://youtube.com/watch?v=x"}
	before := m.fingerprint(src)
	m.ReloadYouTube()
	after := m.fingerprint(src)
	if before == after {
		t.Fatal("ReloadYouTube must change the YouTube fingerprint")
	}
	rtsp := model.Source{Kind: model.KindRTSP, URL: "rtsp://cam"}
	if m.fingerprint(rtsp) != ingestFingerprint(rtsp) {
		t.Fatal("RTSP fingerprints must not include the YouTube auth generation")
	}
}

func TestSameSet(t *testing.T) {
	cases := []struct {
		name   string
		sorted []uint
		ids    []uint
		want   bool
	}{
		{"both empty", nil, nil, true},
		{"same order", []uint{1, 2, 3}, []uint{1, 2, 3}, true},
		{"different order", []uint{1, 2, 3}, []uint{3, 1, 2}, true},
		{"extra", []uint{1, 2}, []uint{1, 2, 3}, false},
		{"missing", []uint{1, 2, 3}, []uint{1, 2}, false},
		{"swapped member", []uint{1, 2, 3}, []uint{1, 2, 4}, false},
		{"empty vs one", nil, []uint{1}, false},
	}
	for _, tc := range cases {
		if got := sameSet(tc.sorted, tc.ids); got != tc.want {
			t.Errorf("%s: sameSet(%v, %v) = %v", tc.name, tc.sorted, tc.ids, got)
		}
	}
}

// The render loop calls RequestSync every frame. An unchanged set must not
// wake the manager, or it re-queries SQLite at frame rate forever.
func TestRequestSyncIgnoresUnchangedSet(t *testing.T) {
	m := NewManager(nil, nil, capability.Info{}, resolver.Tools{}, nil)
	m.applied = []uint{1, 2, 3}

	for i := 0; i < 100; i++ {
		m.RequestSync([]uint{3, 2, 1})
	}
	if len(m.needed) != 0 {
		t.Fatal("unchanged set should not queue a sync")
	}

	m.RequestSync([]uint{1, 2})
	if len(m.needed) != 1 {
		t.Fatal("a changed set must queue a sync")
	}
}

// Configuration edits can leave the needed set identical while changing the
// rows behind it, so Resync must always get through.
func TestResyncForcesAPass(t *testing.T) {
	m := NewManager(nil, nil, capability.Info{}, resolver.Tools{}, nil)
	m.applied = []uint{1, 2, 3}

	m.Resync([]uint{1, 2, 3})
	if len(m.needed) != 1 {
		t.Fatal("Resync must queue a sync even for an unchanged set")
	}
	if !m.dirty.Load() {
		t.Fatal("Resync must mark the manager dirty")
	}
	if m.settled([]uint{1, 2, 3}) {
		t.Fatal("a dirty manager is never settled")
	}
}
