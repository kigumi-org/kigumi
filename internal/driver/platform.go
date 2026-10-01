package driver

import "kigumi/internal/platform"

// `--sys none` asks for no OS services, so it selects the `bare` files like
// a freestanding OS.
func (m *Module) platformFile(name string) (tagged, matches bool) {
	os := m.target.OS
	if m.target.Sys == "none" {
		os = "freestanding"
	}
	return platform.File(name, os, m.target.Arch)
}
