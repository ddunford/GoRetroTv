# Bake an untracked flash-shaped file into the runtime image

Creates a synthetic file only in the probe worktree and copies it into the runtime image. It is
left untracked so the repository detector stays quiet; the image detector must report
`firmware-in-image` after building and exporting the actual runtime image.
