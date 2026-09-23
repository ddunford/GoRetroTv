//go:build race

package broadcast_test

// underRaceDetector is true when this package was built with -race. See the firmware fixture in
// fixture_test.go for why the boxes skip when it is.
const underRaceDetector = true
