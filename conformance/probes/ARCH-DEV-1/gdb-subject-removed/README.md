# Move the GDB listener away

Moves the audited listener source aside. The checker must report `bind-subject-missing` before
trying to compile the Go helper, proving the guard still covers this listener.
