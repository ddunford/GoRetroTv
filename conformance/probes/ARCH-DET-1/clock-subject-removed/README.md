# Remove the present instruction-time package

Moves `internal/platform/clock` aside. The coverage detector must report
`instruction-subject-missing`, rather than passing over a future CPU/device set that is still
empty.
