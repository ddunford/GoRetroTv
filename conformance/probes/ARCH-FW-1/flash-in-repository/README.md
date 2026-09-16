# Commit a fake flash-shaped image

Adds a tracked 2 MB file carrying the U202 bootloader's documented JB magic at its documented
offset. The checker must report `firmware-in-repository`. It is generated inside the probe worktree;
no Pace image is copied or committed.
