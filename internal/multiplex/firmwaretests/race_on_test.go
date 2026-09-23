//go:build race

package firmwaretests_test

// underRaceDetector is true when this package was built with -race.
//
// Go gives a test no way to ask at runtime, so the answer comes from the build tag and its twin in
// race_off_test.go. Two three-line files rather than a condition, because the alternative is a
// package that silently runs the boxes under the detector again the moment somebody deletes a
// guard they do not recognise.
const underRaceDetector = true
