# Move the bus out of the core corpus

Moves `internal/bus` aside. The checker must report `core-subject-missing`, because a green
import scan over the remaining memory files would not prove the bus boundary.
