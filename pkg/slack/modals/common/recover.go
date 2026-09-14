package common

import (
	"runtime/debug"

	"k8s.io/klog"
)

// SafeGo runs fn in a new goroutine, recovering from any panic so that a bug in
// a single modal interaction can never crash the whole bot process. Slack
// interaction handlers spawn background goroutines to do their work and post
// results back to Slack; a panic in one of those goroutines is not recovered by
// net/http and would otherwise take the entire service down for every user.
func SafeGo(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				klog.Errorf("Recovered from panic in modal interaction goroutine: %v\n%s", r, debug.Stack())
			}
		}()
		fn()
	}()
}
