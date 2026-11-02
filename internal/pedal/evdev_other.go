//go:build !linux

package pedal

import (
	"context"
	"log"
)

// Watch is a no-op outside Linux (e.g. Mac development builds) — evdev is a
// Linux-only kernel interface. The phone's HTTP control works regardless.
func Watch(ctx context.Context, nameSubstr string, c Controller) {
	log.Println("pedal: foot pedal input requires Linux (evdev); disabled on this platform")
}
