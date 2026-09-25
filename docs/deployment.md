# Public demo deployment

<!-- anchor: docker-compose.traefik.yml -->
<!-- anchor: Dockerfile -->
<!-- fingerprint: sha256:6d079bbc6f0904e0723611675a973c29d9e090bc9199bb86ab9a84d5dace408b @ 2026-09-25 -->

The public route is hosted on the wildcard origin described in the machine's private Traefik
configuration. The base compose file remains a local loopback stack. The
`docker-compose.traefik.yml` overlay joins the configured shared Traefik network, routes the host
named by `GORETROTV_DOMAIN` over TLS, and removes the base stack's host port. The browser page and Go
binary and ffmpeg runtime are built into the image. The firmware, post-acquisition snapshot and
operator media directory stay on this host as read-only mounts; none is in the build context or
published image. Schedule media paths are relative to `/media` in the container.

## Start and verify

<!-- anchor: ctl.sh -->
<!-- fingerprint: sha256:86aee141dd84d885ffca9af74277ce79cb965a7a706d45e08b0094738c5bb1e3 @ 2026-09-25 -->

From the repository owner account, with the private firmware files and
`snapshots/post-acquisition.snapshot` present:

```sh
./ctl.sh up-public
./ctl.sh health-public
./ctl.sh logs-public 100
```

`up-public` starts the container under the caller's non-root UID/GID so it can read the private
snapshot without widening its `0600` file mode or its directory's `0700` mode. It sets production
mode, disables pprof, and validates the public `/health` route. The app itself refuses to start
with pprof enabled outside explicit development mode, including a direct production run that
bypasses this overlay. The firmware is checked against its
manifest before the listener opens; the restored snapshot must pass the exact post-acquisition
state check before the page is served.
`logs-public` reads the published container through the same compose overlay when startup or a
firmware-driven transition needs diagnosing. Public commands require `GORETROTV_DOMAIN` in `.env`.

Check `https://$GORETROTV_DOMAIN/` in a browser, including the actual `/styles.css`,
`/favicon.svg`, and `/dist/*.js` requests. The server must return 404 for unknown assets, not
index HTML. Press **sky** and observe the firmware's menu. WebSocket uses the same origin at
`wss://$GORETROTV_DOMAIN/ws`.

`npm run test:e2e:public` runs the committed browser acceptance against this HTTPS route. It
briefly restarts the public container from the private snapshot first, so the Sky-to-menu test
starts from a known screen; run it when a short interruption is acceptable.

## Check the public developer boundary

<!-- anchor: internal/httpx/handlers/pprof.go -->
<!-- fingerprint: sha256:79f8f30f869a16d87462217306c1a6a4ae939317eae1e8b73131d0df9c9e75ff @ 2026-09-22 -->

Run these checks while the public stack is up. The live `/health` request is the positive control;
each developer route must answer 404. `config-public` prints the merged configuration, including
the fixed `GORETROTV_ENABLE_PPROF=false` value even if an operator exports `true` in the shell.

```sh
./ctl.sh health-public
GORETROTV_ENABLE_PPROF=true ./ctl.sh config-public --format json
for path in /debug/pprof/ /debug/pprof/profile /debug/pprof/cmdline /instruments /metrics /trace; do
  curl -sS -o /dev/null -w "$path %{http_code}\n" "https://${GORETROTV_DOMAIN}$path"
done
```

From this origin, TCP attempts to `$GORETROTV_DOMAIN:23457` (the GDB test port) and `:8099`
(the app port) must fail. Repeat against the host's LAN address to rule out a direct port bypass
around Cloudflare:

```sh
python3 - <<'PY'
import os
import socket
import subprocess

route = subprocess.check_output(['ip', '-4', 'route', 'get', '1.1.1.1'], text=True).split()
lan = route[route.index('src') + 1]
for host in (os.environ['GORETROTV_DOMAIN'], lan):
    for port in (23457, 8099):
        try:
            connection = socket.create_connection((host, port), timeout=3)
        except OSError as error:
            print(f'{host}:{port}: blocked ({type(error).__name__})')
        else:
            connection.close()
            raise SystemExit(f'developer port exposed: {host}:{port}')
PY
```

The public overlay publishes no host port, and the runtime image contains only `goretrotv` and
`oraclecmp`, not the separate `firmwaretrace` executable that can start a GDB stub.

## Stop or roll back

<!-- anchor: ctl.sh -->
<!-- fingerprint: sha256:86aee141dd84d885ffca9af74277ce79cb965a7a706d45e08b0094738c5bb1e3 @ 2026-09-25 -->

`./ctl.sh down-public` removes the route and container while preserving the private files. To
restore a previous release, check out that release and run `./ctl.sh up-public`; the firmware and
snapshot mounts are unchanged. The image build stamps the source commit in `/health` for checking
which release answers the public URL.
