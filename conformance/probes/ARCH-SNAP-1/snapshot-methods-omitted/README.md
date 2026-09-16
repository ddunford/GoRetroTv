# Add a device shape without serialization

Adds a production type with Name, Read, Write and Reset but no Snapshot or Restore. The census
must discover it and report `device-missing-snapshot`, even though no bus registration yet
requires the type to implement `bus.Device` at compile time.
