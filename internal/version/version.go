// Package version carries the build identity, injected at link time.
package version

// Overridden at build time via -ldflags:
//
//	-X github.com/ddunford/goretrotv/internal/version.Version=...
//	-X github.com/ddunford/goretrotv/internal/version.Commit=...
//	-X github.com/ddunford/goretrotv/internal/version.Date=...
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)
