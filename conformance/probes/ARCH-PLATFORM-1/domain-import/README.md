# Domain import from the platform layer

Adds a Go import in `internal/platform/hexfmt` pointing at `internal/device/demux`.
`ARCH-PLATFORM-1` must report `platform-imports-domain`, even while that device package has
not been built. The import edge itself is the prohibited architecture change.
