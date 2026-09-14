//go:build !linux

package capability

func classifyV4L2M2M(nodes []string) []string {
	return []string{}
}
