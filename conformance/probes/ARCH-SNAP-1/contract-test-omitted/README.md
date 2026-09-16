# Add a serializable device without its contract test

Adds a complete method set but no `Test<Type>HoldsTheDeviceContract` call to `bustest.CheckSnapshot`.
The checker must report `device-missing-contract-test`, so a new type cannot inherit a green
round-trip result from tests of older devices.
