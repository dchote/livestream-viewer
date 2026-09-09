package output

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeCard builds a synthetic DRM tree: a /dev/dri/cardN node plus one sysfs
// connector directory per entry in connectors (name -> status).
func writeCard(t *testing.T, root string, index int, connectors map[string]string, modes int) {
	t.Helper()
	dev := filepath.Join(root, "dev", "dri")
	if err := os.MkdirAll(dev, 0o755); err != nil {
		t.Fatal(err)
	}
	node := filepath.Join(dev, "card"+itoa(index))
	if err := os.WriteFile(node, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for name, status := range connectors {
		dir := filepath.Join(root, "sys", "class", "drm", "card"+itoa(index)+"-"+name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "status"), []byte(status+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		lines := make([]string, 0, modes)
		for i := 0; i < modes; i++ {
			lines = append(lines, "1920x1080")
		}
		if err := os.WriteFile(filepath.Join(dir, "modes"), []byte(strings.Join(lines, "\n")), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func itoa(i int) string {
	return string(rune('0' + i))
}

func useRoot(t *testing.T, root string) {
	t.Helper()
	prev := drmRoot
	drmRoot = root
	t.Cleanup(func() { drmRoot = prev })
}

func TestDRMDiagnosticNoDevDri(t *testing.T) {
	useRoot(t, t.TempDir())
	got := DRMDiagnostic(-1)
	if !strings.Contains(got, "/dev/dri is not present") {
		t.Fatalf("got %q", got)
	}
}

// The Raspberry Pi layout: card0 is the render-only v3d node and card1 is vc4
// with the panel. Pinning card0 is why SDL reported no displays.
func TestDRMDiagnosticPinnedWrongCard(t *testing.T) {
	root := t.TempDir()
	useRoot(t, root)
	writeCard(t, root, 0, nil, 0)
	writeCard(t, root, 1, map[string]string{"HDMI-A-1": "connected"}, 3)

	got := DRMDiagnostic(0)
	if !strings.Contains(got, "render-only node") {
		t.Fatalf("expected card0 described as render-only, got %q", got)
	}
	if !strings.Contains(got, "pins card0 but only card1 can drive output") {
		t.Fatalf("expected the pin to be called out, got %q", got)
	}
}

func TestDRMDiagnosticNoPanelAttached(t *testing.T) {
	root := t.TempDir()
	useRoot(t, root)
	writeCard(t, root, 0, map[string]string{"HDMI-A-1": "disconnected"}, 0)

	got := DRMDiagnostic(-1)
	if !strings.Contains(got, "no card has a connected panel") {
		t.Fatalf("got %q", got)
	}
}

// Auto-detect with a usable card means the card selection is not the problem,
// so the diagnostic must point at DRM master instead of blaming the hardware.
func TestDRMDiagnosticUsableCardBlamesDRMMaster(t *testing.T) {
	root := t.TempDir()
	useRoot(t, root)
	writeCard(t, root, 0, nil, 0)
	writeCard(t, root, 1, map[string]string{"HDMI-A-1": "connected"}, 2)

	got := DRMDiagnostic(-1)
	if !strings.Contains(got, "DRM master held by another client") {
		t.Fatalf("got %q", got)
	}
}

func TestDRMCardsReportsConnectorState(t *testing.T) {
	root := t.TempDir()
	useRoot(t, root)
	writeCard(t, root, 0, nil, 0)
	writeCard(t, root, 1, map[string]string{"HDMI-A-1": "connected"}, 2)

	cards := DRMCards()
	if len(cards) != 2 {
		t.Fatalf("cards = %d, want 2", len(cards))
	}
	if cards[0].Connected() {
		t.Fatal("card0 has no connectors and must not report connected")
	}
	if !cards[1].Connected() {
		t.Fatal("card1 has a connected connector with modes")
	}
	if cards[1].Connectors[0].Modes != 2 {
		t.Fatalf("modes = %d, want 2", cards[1].Connectors[0].Modes)
	}
}
