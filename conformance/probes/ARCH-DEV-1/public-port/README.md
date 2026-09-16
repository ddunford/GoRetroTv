# Publish the container port publicly

Changes the host-side compose mapping to `0.0.0.0`. The checker must report
`container-port-public` against Compose's parsed effective port mapping.
