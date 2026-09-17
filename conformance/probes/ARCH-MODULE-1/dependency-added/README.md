# Add a Go dependency

Adds a second `require` directive using Go's own modfile editor. The checker must report
`external-module` without needing to download the package. This is the same change a developer
would make with `go get` before importing an unapproved module.
