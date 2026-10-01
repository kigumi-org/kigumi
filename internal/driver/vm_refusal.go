package driver

import "fmt"

// std/net's Http gets its own wording here: the VM's rejection is
// deliberate, not a gap, and a
// program always hits `Net.http` (to get an `Http` at all) before it could
// ever reach `Http.post` itself.
func vmRefusalError(why string) error {
	if why == "primitive net.Net.http" || why == "primitive net.Http.post" {
		return fmt.Errorf("net.Http.post is deliberately not supported by the VM: its native, interp and VM behavior would otherwise diverge (cleanup_plan_v1.md §2.3) (run with --engine interp)")
	}
	return fmt.Errorf("%s is not supported by the VM (run with --engine interp)", why)
}
