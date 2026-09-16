# Move the audited config loader away

Moves `internal/config/config.go` aside. The checker must report `bind-subject-missing`, so the
rule cannot pass when the loader it means to exercise vanishes.
