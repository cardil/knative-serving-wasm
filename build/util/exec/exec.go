package exec

import (
	"github.com/goyek/goyek/v2"
	"github.com/goyek/x/cmd"
)

// ExecOrDie runs the command and fails the task immediately if it returns an error.
// This is a wrapper around cmd.Exec that calls a.FailNow() on failure.
func ExecOrDie(a *goyek.A, cmdLine string, opts ...cmd.Option) {
	a.Helper()
	if !cmd.Exec(a, cmdLine, opts...) {
		a.FailNow()
	}
}
