package output

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// drmRoot prefixes every DRM path so tests can supply a synthetic tree. This
// code only runs when KMSDRM output has already failed, so it has to be
// verifiable somewhere other than the target panel.
var drmRoot = ""

// DRMCard is one /dev/dri/cardN node and the connector state sysfs reports for it.
type DRMCard struct {
	Index      int
	Path       string
	Openable   bool
	OpenErr    string
	Connectors []DRMConnector
}

// DRMConnector is one output on a card, e.g. cardN-HDMI-A-1.
type DRMConnector struct {
	Name   string
	Status string // connected, disconnected, unknown
	Modes  int
}

// Connected reports that at least one connector has a panel with usable modes.
// SDL's KMSDRM device scan requires exactly this, so a card without it can
// never drive output no matter how the driver is configured.
func (c DRMCard) Connected() bool {
	for _, conn := range c.Connectors {
		if conn.Status == "connected" && conn.Modes > 0 {
			return true
		}
	}
	return false
}

// DRMCards enumerates /dev/dri/card* and reads each card's connectors from
// sysfs. It needs no libdrm and takes no DRM master, so it is safe to call
// after SDL has already failed.
func DRMCards() []DRMCard {
	nodes, _ := filepath.Glob(drmRoot + "/dev/dri/card*")
	sort.Strings(nodes)
	cards := make([]DRMCard, 0, len(nodes))
	for _, node := range nodes {
		idx, err := strconv.Atoi(strings.TrimPrefix(filepath.Base(node), "card"))
		if err != nil {
			continue
		}
		card := DRMCard{Index: idx, Path: node}
		if f, err := os.OpenFile(node, os.O_RDWR, 0); err == nil {
			card.Openable = true
			_ = f.Close()
		} else {
			card.OpenErr = err.Error()
		}
		card.Connectors = drmConnectors(idx)
		cards = append(cards, card)
	}
	return cards
}

func drmConnectors(cardIndex int) []DRMConnector {
	prefix := fmt.Sprintf("card%d-", cardIndex)
	entries, _ := filepath.Glob(drmRoot + "/sys/class/drm/" + prefix + "*")
	sort.Strings(entries)
	out := make([]DRMConnector, 0, len(entries))
	for _, dir := range entries {
		conn := DRMConnector{
			Name:   strings.TrimPrefix(filepath.Base(dir), prefix),
			Status: "unknown",
		}
		if b, err := os.ReadFile(filepath.Join(dir, "status")); err == nil {
			conn.Status = strings.TrimSpace(string(b))
		}
		if b, err := os.ReadFile(filepath.Join(dir, "modes")); err == nil {
			for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
				if strings.TrimSpace(line) != "" {
					conn.Modes++
				}
			}
		}
		out = append(out, conn)
	}
	return out
}

// DRMDiagnostic explains why KMSDRM has no display to drive, in the terms an
// operator can act on: no device passed through, no panel attached, or the
// wrong card pinned. SDL reports all three as one opaque string.
func DRMDiagnostic(pinned int) string {
	if _, err := os.Stat(drmRoot + "/dev/dri"); err != nil {
		return "/dev/dri is not present; pass the host GPU through to the container (devices: /dev/dri)"
	}
	cards := DRMCards()
	if len(cards) == 0 {
		return "/dev/dri exists but contains no card* nodes; the kernel has no modesetting driver loaded"
	}

	parts := make([]string, 0, len(cards))
	usable := make([]int, 0, len(cards))
	for _, card := range cards {
		desc := make([]string, 0, len(card.Connectors))
		for _, conn := range card.Connectors {
			desc = append(desc, fmt.Sprintf("%s=%s/%dmodes", conn.Name, conn.Status, conn.Modes))
		}
		detail := strings.Join(desc, " ")
		if detail == "" {
			detail = "no connectors (render-only node)"
		}
		if !card.Openable {
			detail += " [cannot open: " + card.OpenErr + "]"
		}
		parts = append(parts, fmt.Sprintf("card%d: %s", card.Index, detail))
		if card.Connected() && card.Openable {
			usable = append(usable, card.Index)
		}
	}

	summary := strings.Join(parts, "; ")
	switch {
	case len(usable) == 0:
		return summary + "; no card has a connected panel with modes, so KMSDRM has nothing to drive"
	case pinned >= 0 && !containsInt(usable, pinned):
		return fmt.Sprintf("%s; display.device pins card%d but only %s can drive output",
			summary, pinned, cardList(usable))
	default:
		return summary + "; a usable card exists, so the failure is likely DRM master held by another client (console, compositor, or a second instance)"
	}
}

func containsInt(values []int, want int) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func cardList(indexes []int) string {
	names := make([]string, 0, len(indexes))
	for _, i := range indexes {
		names = append(names, "card"+strconv.Itoa(i))
	}
	return strings.Join(names, "/")
}
