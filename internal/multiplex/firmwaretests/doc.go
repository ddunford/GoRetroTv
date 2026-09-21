// Package firmwaretests holds the multiplex tests that drive the REAL firmware,
// separated from internal/multiplex so the two do not compete for one timeout.
//
// They were together until 2026-09-21, when internal/multiplex reached
// 1800.280s under the race detector against the Makefile's 30-minute ceiling
// and failed the suite outright -- a failure that reads as a hang rather than
// as "the package outgrew its budget", and which costs a full thirty minutes to
// discover. Every test here restores a real box from the post-acquisition
// snapshot and runs millions of guest instructions, so each one costs tens of
// seconds under -race while the loader and codec tests next door cost
// milliseconds. Keeping them apart means the expensive set is nameable, can be
// given its own budget, and cannot push the cheap set over a cliff.
//
// Everything here skips rather than fails when the private firmware, the
// dictionary or the snapshot is absent, exactly as it did before the split.
package firmwaretests
