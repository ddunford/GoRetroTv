# Make the container override configurable

Replaces the literal compose value with an environment expansion. The checker must report
`container-override-not-literal`, because another input could then widen a listener silently.
