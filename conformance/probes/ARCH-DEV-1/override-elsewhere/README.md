# Put the container override in another configuration file

A second tracked shell script sets `GORETROTV_BIND_ALL_INTERFACES`. The checker must report
`bind-override-elsewhere`; the override is a container exception, not a general setting.
