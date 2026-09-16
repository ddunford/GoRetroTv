# Add a state field that the device's mutator does not drive

Adds an exported RAM state field. `bustest.CheckSnapshot` must fail because its mutator and
serializer have not been extended to cover the field. The checker must attribute the failure to
`device-contract-failed` rather than mistaking a passing test from another type for coverage.
