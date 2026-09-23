//go:build !race

package broadcast_test

// underRaceDetector is false when this package was built without -race. See race_on_test.go.
const underRaceDetector = false
