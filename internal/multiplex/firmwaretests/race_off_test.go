//go:build !race

package firmwaretests_test

// underRaceDetector is false when this package was built without -race. See race_on_test.go.
const underRaceDetector = false
