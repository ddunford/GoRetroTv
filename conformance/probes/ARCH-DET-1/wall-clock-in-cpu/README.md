# Introduce wall time into the future CPU package

Adds a production source file using `time.Now` under `internal/cpu`. The checker must enrol
that newly created package automatically and report `wall-clock-source`.
