# Public demo deployment

The public route is hosted on the `*.demosrv.uk` origin described in the machine's private
`~/.claude/local/traefik.md`. The base compose file remains a local loopback stack. The
`docker-compose.traefik.yml` overlay joins the shared Traefik network, routes
`goretrotv.demosrv.uk` over TLS, and removes the base stack's host port. The browser page and Go
binary are built into the image. The firmware and post-acquisition snapshot stay on this host as
read-only mounts; neither is in the build context or published image.

## Start and verify

From the repository owner account, with the private firmware files and
`snapshots/post-acquisition.snapshot` present:

```sh
./ctl.sh up-public
./ctl.sh health-public
```

`up-public` starts the container under the caller's non-root UID/GID so it can read the private
snapshot without widening its `0600` file mode or its directory's `0700` mode. It sets production
mode, disables pprof, and validates the public `/health` route. The firmware is checked against its
manifest before the listener opens; the restored snapshot must pass the exact post-acquisition
state check before the page is served.

Check `https://goretrotv.demosrv.uk/` in a browser, including the actual `/styles.css`,
`/favicon.svg`, and `/dist/*.js` requests. The server must return 404 for unknown assets, not
index HTML. Press **sky** and observe the firmware's menu. WebSocket uses the same origin at
`wss://goretrotv.demosrv.uk/ws`.

## Stop or roll back

`./ctl.sh down-public` removes the route and container while preserving the private files. To
restore a previous release, check out that release and run `./ctl.sh up-public`; the firmware and
snapshot mounts are unchanged. The image build stamps the source commit in `/health` for checking
which release answers the public URL.
